package checkins

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/linxunxi/image-mirror/internal/systemconfig"
)

var ErrAlreadyCheckedIn = errors.New("already checked in today")

var checkinLocation = func() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return location
}()

type Status struct {
	Enabled     bool       `json:"enabled"`
	Credits     int64      `json:"credits"`
	CheckedIn   bool       `json:"checkedIn"`
	LastCheckin *time.Time `json:"lastCheckin,omitempty"`
}

type Result struct {
	Status
	Balance int64 `json:"balance"`
}

type Service struct {
	db      *pgxpool.Pool
	configs *systemconfig.Service
}

func NewService(db *pgxpool.Pool, configSvc *systemconfig.Service) *Service {
	return &Service{db: db, configs: configSvc}
}

func (s *Service) Status(ctx context.Context, userID string) (Status, error) {
	settings, err := s.configs.CheckinSettings(ctx)
	if err != nil {
		return Status{}, err
	}
	status := Status{
		Enabled: settings.Enabled,
		Credits: settings.Credits,
	}
	checkinDate := todayCheckinDate()
	var createdAt time.Time
	err = s.db.QueryRow(ctx, `
		SELECT created_at
		FROM daily_checkins
		WHERE user_id=$1 AND checkin_date=$2
	`, userID, checkinDate).Scan(&createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return status, nil
	}
	if err != nil {
		return Status{}, err
	}
	status.CheckedIn = true
	status.LastCheckin = &createdAt
	return status, nil
}

func (s *Service) Checkin(ctx context.Context, userID string) (Result, error) {
	settings, err := s.configs.CheckinSettings(ctx)
	if err != nil {
		return Result{}, err
	}
	if !settings.Enabled {
		return Result{}, errors.New("daily check-in is disabled")
	}
	if settings.Credits <= 0 {
		return Result{}, errors.New("daily check-in credits must be positive")
	}
	checkinDate := todayCheckinDate()

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var checkinCreatedAt time.Time
	if err := tx.QueryRow(ctx, `
		INSERT INTO daily_checkins(user_id, checkin_date, credits)
		VALUES ($1, $2, $3)
		RETURNING created_at
	`, userID, checkinDate, settings.Credits).Scan(&checkinCreatedAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Result{}, ErrAlreadyCheckedIn
		}
		return Result{}, err
	}

	var balance int64
	if err := tx.QueryRow(ctx, `SELECT balance FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&balance); err != nil {
		return Result{}, err
	}
	next := balance + settings.Credits
	if _, err := tx.Exec(ctx, `UPDATE users SET balance=$1, updated_at=now() WHERE id=$2`, next, userID); err != nil {
		return Result{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO credit_transactions(user_id, type, amount, balance_after, description)
		VALUES ($1, 'RECHARGE', $2, $3, $4)
	`, userID, settings.Credits, next, "每日签到奖励"); err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}

	return Result{
		Status: Status{
			Enabled:     true,
			Credits:     settings.Credits,
			CheckedIn:   true,
			LastCheckin: &checkinCreatedAt,
		},
		Balance: next,
	}, nil
}

func todayCheckinDate() string {
	return time.Now().In(checkinLocation).Format(time.DateOnly)
}
