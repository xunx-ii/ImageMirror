package admin

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/linxunxi/image-mirror/internal/billing"
	"github.com/linxunxi/image-mirror/internal/users"
)

type Service struct {
	db      *pgxpool.Pool
	users   *users.Repository
	billing *billing.Service
}

type Overview struct {
	Users           int64 `json:"users"`
	Images          int64 `json:"images"`
	Completed       int64 `json:"completed"`
	CreditsConsumed int64 `json:"creditsConsumed"`
}

func NewService(db *pgxpool.Pool, usersRepo *users.Repository, billingSvc *billing.Service) *Service {
	return &Service{db: db, users: usersRepo, billing: billingSvc}
}

func (s *Service) ListUsers(ctx context.Context, limit int32, offset int32) ([]users.User, error) {
	return s.users.List(ctx, limit, offset)
}

func (s *Service) AdjustBalance(ctx context.Context, userID string, amount int64, description string) error {
	if description == "" {
		description = "admin adjustment"
	}
	return s.billing.AdminAdjust(ctx, userID, amount, description)
}

func (s *Service) SetUserStatus(ctx context.Context, actorID string, targetID string, status string) (users.User, error) {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "ACTIVE" && status != "SUSPENDED" {
		return users.User{}, errors.New("unsupported user status")
	}
	if actorID == targetID && status == "SUSPENDED" {
		return users.User{}, errors.New("cannot suspend your own account")
	}
	target, err := s.users.FindByID(ctx, targetID)
	if err != nil {
		return users.User{}, err
	}
	if target.Role == "ADMIN" && status == "SUSPENDED" {
		if err := s.ensureAnotherActiveAdmin(ctx, target.ID); err != nil {
			return users.User{}, err
		}
	}
	return s.users.UpdateStatus(ctx, targetID, status)
}

func (s *Service) DeleteUser(ctx context.Context, actorID string, targetID string) error {
	if actorID == targetID {
		return errors.New("cannot delete your own account")
	}
	target, err := s.users.FindByID(ctx, targetID)
	if err != nil {
		return err
	}
	if target.Role == "ADMIN" {
		if err := s.ensureAnotherActiveAdmin(ctx, target.ID); err != nil {
			return err
		}
	}
	return s.users.Delete(ctx, targetID)
}

func (s *Service) ensureAnotherActiveAdmin(ctx context.Context, targetID string) error {
	count, err := s.users.CountActiveAdminsExcept(ctx, targetID)
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("at least one active admin is required")
	}
	return nil
}

func (s *Service) Overview(ctx context.Context) (Overview, error) {
	var out Overview
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&out.Users); err != nil {
		return out, err
	}
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM image_generations`).Scan(&out.Images); err != nil {
		return out, err
	}
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM image_generations WHERE status='COMPLETED'`).Scan(&out.Completed); err != nil {
		return out, err
	}
	if err := s.db.QueryRow(ctx, `SELECT COALESCE(abs(sum(amount)),0) FROM credit_transactions WHERE type='CONSUME'`).Scan(&out.CreditsConsumed); err != nil {
		return out, err
	}
	return out, nil
}
