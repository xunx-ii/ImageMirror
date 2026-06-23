package users

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUserNotFound = errors.New("user not found")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, email string, passwordHash string, role string, initialBalance int64) (User, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO users(email, password_hash, role, balance)
		VALUES ($1, $2, $3, $4)
		RETURNING id, email, role, status, balance, last_login_at, created_at, updated_at, password_hash
	`, email, passwordHash, role, initialBalance)
	return scanUser(row)
}

func (r *Repository) FindByEmail(ctx context.Context, email string) (User, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, email, role, status, balance, last_login_at, created_at, updated_at, password_hash
		FROM users
		WHERE email=$1
	`, email)
	return scanUser(row)
}

func (r *Repository) FindByID(ctx context.Context, id string) (User, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, email, role, status, balance, last_login_at, created_at, updated_at, password_hash
		FROM users
		WHERE id=$1
	`, id)
	return scanUser(row)
}

func (r *Repository) List(ctx context.Context, limit int32, offset int32) ([]User, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, email, role, status, balance, last_login_at, created_at, updated_at, password_hash
		FROM users
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, user)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateLoginAt(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET last_login_at=now(), updated_at=now() WHERE id=$1`, id)
	return err
}

func scanUser(row pgx.Row) (User, error) {
	var user User
	var lastLoginAt pgtype.Timestamptz
	if err := row.Scan(&user.ID, &user.Email, &user.Role, &user.Status, &user.Balance, &lastLoginAt, &user.CreatedAt, &user.UpdatedAt, &user.PasswordHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, err
	}
	if lastLoginAt.Valid {
		value := lastLoginAt.Time.In(time.UTC)
		user.LastLoginAt = &value
	}
	return user, nil
}
