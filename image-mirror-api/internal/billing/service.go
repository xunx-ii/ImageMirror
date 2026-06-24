package billing

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Transaction struct {
	ID           string    `json:"id"`
	UserID       string    `json:"userId"`
	Type         string    `json:"type"`
	Amount       int64     `json:"amount"`
	BalanceAfter int64     `json:"balanceAfter"`
	Description  string    `json:"description"`
	RelatedID    *string   `json:"relatedId,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) Deduct(ctx context.Context, userID string, amount int64, description string, relatedID string) error {
	return s.change(ctx, userID, -amount, "CONSUME", description, &relatedID, true)
}

func (s *Service) Refund(ctx context.Context, userID string, amount int64, description string, relatedID string) error {
	return s.change(ctx, userID, amount, "REFUND", description, &relatedID, false)
}

func (s *Service) RefundWithTx(ctx context.Context, tx pgx.Tx, userID string, amount int64, description string, relatedID string) error {
	return s.changeWithTx(ctx, tx, userID, amount, "REFUND", description, &relatedID, false)
}

func (s *Service) Recharge(ctx context.Context, userID string, amount int64, description string) error {
	return s.change(ctx, userID, amount, "RECHARGE", description, nil, false)
}

func (s *Service) AdminAdjust(ctx context.Context, userID string, amount int64, description string) error {
	return s.change(ctx, userID, amount, "ADMIN_ADJUST", description, nil, amount < 0)
}

func (s *Service) Balance(ctx context.Context, userID string) (int64, error) {
	var balance int64
	if err := s.db.QueryRow(ctx, `SELECT balance FROM users WHERE id=$1`, userID).Scan(&balance); err != nil {
		return 0, err
	}
	return balance, nil
}

func (s *Service) ListTransactions(ctx context.Context, userID string, limit int32, offset int32) ([]Transaction, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, type, amount, balance_after, description, related_id, created_at
		FROM credit_transactions
		WHERE user_id=$1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Transaction, 0)
	for rows.Next() {
		var item Transaction
		if err := rows.Scan(&item.ID, &item.UserID, &item.Type, &item.Amount, &item.BalanceAfter, &item.Description, &item.RelatedID, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) change(ctx context.Context, userID string, delta int64, txType string, description string, relatedID *string, enforcePositive bool) error {
	if delta == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.changeWithTx(ctx, tx, userID, delta, txType, description, relatedID, enforcePositive); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) changeWithTx(ctx context.Context, tx pgx.Tx, userID string, delta int64, txType string, description string, relatedID *string, enforcePositive bool) error {
	if delta == 0 {
		return nil
	}
	var balance int64
	if err := tx.QueryRow(ctx, `SELECT balance FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&balance); err != nil {
		return err
	}
	next := balance + delta
	if enforcePositive && next < 0 {
		return ErrInsufficientCredits
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET balance=$1, updated_at=now() WHERE id=$2`, next, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO credit_transactions(user_id, type, amount, balance_after, description, related_id)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, userID, txType, delta, next, description, relatedID); err != nil {
		return err
	}
	return nil
}
