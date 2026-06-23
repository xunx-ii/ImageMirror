package redemptions

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Code struct {
	ID             string     `json:"id"`
	Code           string     `json:"code"`
	Credits        int64      `json:"credits"`
	Status         string     `json:"status"`
	ExpiresAt      *time.Time `json:"expiresAt,omitempty"`
	UsedBy         *string    `json:"usedBy,omitempty"`
	UsedByEmail    *string    `json:"usedByEmail,omitempty"`
	UsedAt         *time.Time `json:"usedAt,omitempty"`
	CreatedBy      *string    `json:"createdBy,omitempty"`
	CreatedByEmail *string    `json:"createdByEmail,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type HistoryItem struct {
	Code       string    `json:"code"`
	Credits    int64     `json:"credits"`
	RedeemedAt time.Time `json:"redeemedAt"`
}

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) Generate(ctx context.Context, credits int64, count int, expiresAt *time.Time, adminID string) ([]Code, error) {
	if credits <= 0 {
		return nil, errors.New("credits must be positive")
	}
	if count <= 0 || count > 500 {
		return nil, errors.New("count must be between 1 and 500")
	}
	out := make([]Code, 0, count)
	for len(out) < count {
		code, err := randomCode()
		if err != nil {
			return nil, err
		}
		row := s.db.QueryRow(ctx, `
			INSERT INTO redemption_codes(code, credits, expires_at, created_by)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (code) DO NOTHING
			RETURNING id, code, credits, status, expires_at, used_by, used_at, created_by, created_at, updated_at
		`, code, credits, expiresAt, nullableString(adminID))
		item, err := scanCode(row)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Service) List(ctx context.Context, limit int32, offset int32) ([]Code, error) {
	rows, err := s.db.Query(ctx, `
		SELECT rc.id, rc.code, rc.credits, rc.status, rc.expires_at, rc.used_by, used_user.email, rc.used_at, rc.created_by, created_user.email, rc.created_at, rc.updated_at
		FROM redemption_codes rc
		LEFT JOIN users used_user ON used_user.id=rc.used_by
		LEFT JOIN users created_user ON created_user.id=rc.created_by
		ORDER BY rc.created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Code, 0)
	for rows.Next() {
		item, err := scanCodeWithUsers(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) Disable(ctx context.Context, ids []string) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE redemption_codes
		SET status='DISABLED', updated_at=now()
		WHERE id=ANY($1::uuid[]) AND status='ACTIVE' AND used_at IS NULL
	`, ids)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Service) Delete(ctx context.Context, ids []string) (int64, error) {
	tag, err := s.db.Exec(ctx, `DELETE FROM redemption_codes WHERE id=ANY($1::uuid[])`, ids)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Service) Redeem(ctx context.Context, userID string, rawCode string) (Code, error) {
	codeValue := strings.ToUpper(strings.TrimSpace(rawCode))
	if codeValue == "" {
		return Code{}, errors.New("code is required")
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Code{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx, `
		SELECT id, code, credits, status, expires_at, used_by, used_at, created_by, created_at, updated_at
		FROM redemption_codes
		WHERE code=$1
		FOR UPDATE
	`, codeValue)
	item, err := scanCode(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Code{}, errors.New("redemption code not found")
	}
	if err != nil {
		return Code{}, err
	}
	if item.Status != "ACTIVE" {
		return Code{}, errors.New("redemption code is not active")
	}
	if item.UsedAt != nil {
		return Code{}, errors.New("redemption code has been used")
	}
	if item.ExpiresAt != nil && time.Now().After(*item.ExpiresAt) {
		_, _ = tx.Exec(ctx, `UPDATE redemption_codes SET status='EXPIRED', updated_at=now() WHERE id=$1`, item.ID)
		return Code{}, errors.New("redemption code has expired")
	}

	var balance int64
	if err := tx.QueryRow(ctx, `SELECT balance FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&balance); err != nil {
		return Code{}, err
	}
	next := balance + item.Credits
	row = tx.QueryRow(ctx, `
		UPDATE redemption_codes
		SET status='USED', used_by=$2, used_at=now(), updated_at=now()
		WHERE id=$1
		RETURNING id, code, credits, status, expires_at, used_by, used_at, created_by, created_at, updated_at
	`, item.ID, userID)
	item, err = scanCode(row)
	if err != nil {
		return Code{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET balance=$1, updated_at=now() WHERE id=$2`, next, userID); err != nil {
		return Code{}, err
	}
	relatedID := item.ID
	if _, err := tx.Exec(ctx, `
		INSERT INTO credit_transactions(user_id, type, amount, balance_after, description, related_id)
		VALUES ($1, 'RECHARGE', $2, $3, $4, $5)
	`, userID, item.Credits, next, "redeem code "+item.Code, &relatedID); err != nil {
		return Code{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Code{}, err
	}
	return item, nil
}

func (s *Service) History(ctx context.Context, userID string, limit int32, offset int32) ([]HistoryItem, error) {
	rows, err := s.db.Query(ctx, `
		SELECT code, credits, used_at
		FROM redemption_codes
		WHERE used_by=$1 AND used_at IS NOT NULL
		ORDER BY used_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]HistoryItem, 0)
	for rows.Next() {
		var item HistoryItem
		if err := rows.Scan(&item.Code, &item.Credits, &item.RedeemedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanCode(row pgx.Row) (Code, error) {
	var item Code
	var expiresAt pgtype.Timestamptz
	var usedAt pgtype.Timestamptz
	if err := row.Scan(&item.ID, &item.Code, &item.Credits, &item.Status, &expiresAt, &item.UsedBy, &usedAt, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return Code{}, err
	}
	if expiresAt.Valid {
		item.ExpiresAt = &expiresAt.Time
	}
	if usedAt.Valid {
		item.UsedAt = &usedAt.Time
	}
	return item, nil
}

func scanCodeWithUsers(row pgx.Row) (Code, error) {
	var item Code
	var expiresAt pgtype.Timestamptz
	var usedAt pgtype.Timestamptz
	if err := row.Scan(&item.ID, &item.Code, &item.Credits, &item.Status, &expiresAt, &item.UsedBy, &item.UsedByEmail, &usedAt, &item.CreatedBy, &item.CreatedByEmail, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return Code{}, err
	}
	if expiresAt.Valid {
		item.ExpiresAt = &expiresAt.Time
	}
	if usedAt.Valid {
		item.UsedAt = &usedAt.Time
	}
	return item, nil
}

func randomCode() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	value := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	return "IM-" + value[:4] + "-" + value[4:8] + "-" + value[8:12] + "-" + value[12:16], nil
}

func nullableString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
