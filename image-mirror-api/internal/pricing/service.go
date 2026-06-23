package pricing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Rule struct {
	ID        string    `json:"id"`
	Model     string    `json:"model"`
	Size      string    `json:"size"`
	Quality   string    `json:"quality"`
	Credits   int64     `json:"credits"`
	IsActive  bool      `json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Service struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewService(db *pgxpool.Pool, redisClient *redis.Client) *Service {
	return &Service{db: db, redis: redisClient}
}

func (s *Service) GetCost(ctx context.Context, model string, size string, quality string) (int64, error) {
	cacheKey := fmt.Sprintf("pricing:%s:%s:%s", model, size, quality)
	cached, err := s.redis.Get(ctx, cacheKey).Int64()
	if err == nil && cached > 0 {
		return cached, nil
	}
	var credits int64
	err = s.db.QueryRow(ctx, `
		SELECT credits FROM pricing_rules
		WHERE model=$1 AND size=$2 AND quality=$3 AND is_active=true
	`, model, size, quality).Scan(&credits)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, errors.New("pricing rule not found")
	}
	if err != nil {
		return 0, err
	}
	_ = s.redis.Set(ctx, cacheKey, credits, 10*time.Minute).Err()
	return credits, nil
}

func (s *Service) List(ctx context.Context) ([]Rule, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, model, size, quality, credits, is_active, created_at, updated_at
		FROM pricing_rules
		WHERE is_active=true
		ORDER BY model, size, quality
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := make([]Rule, 0)
	for rows.Next() {
		var rule Rule
		if err := rows.Scan(&rule.ID, &rule.Model, &rule.Size, &rule.Quality, &rule.Credits, &rule.IsActive, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (s *Service) Upsert(ctx context.Context, model string, size string, quality string, credits int64, active bool) (Rule, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO pricing_rules(model, size, quality, credits, is_active)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (model, size, quality)
		DO UPDATE SET credits=EXCLUDED.credits, is_active=EXCLUDED.is_active, updated_at=now()
		RETURNING id, model, size, quality, credits, is_active, created_at, updated_at
	`, model, size, quality, credits, active)
	var rule Rule
	if err := row.Scan(&rule.ID, &rule.Model, &rule.Size, &rule.Quality, &rule.Credits, &rule.IsActive, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
		return Rule{}, err
	}
	_ = s.redis.Del(ctx, fmt.Sprintf("pricing:%s:%s:%s", model, size, quality)).Err()
	return rule, nil
}

func (s *Service) Update(ctx context.Context, id string, model string, size string, quality string, credits int64, active bool) (Rule, error) {
	var oldModel, oldSize, oldQuality string
	if err := s.db.QueryRow(ctx, `
		SELECT model, size, quality FROM pricing_rules WHERE id=$1
	`, id).Scan(&oldModel, &oldSize, &oldQuality); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Rule{}, errors.New("pricing rule not found")
		}
		return Rule{}, err
	}
	row := s.db.QueryRow(ctx, `
		UPDATE pricing_rules
		SET model=$2, size=$3, quality=$4, credits=$5, is_active=$6, updated_at=now()
		WHERE id=$1
		RETURNING id, model, size, quality, credits, is_active, created_at, updated_at
	`, id, model, size, quality, credits, active)
	var rule Rule
	if err := row.Scan(&rule.ID, &rule.Model, &rule.Size, &rule.Quality, &rule.Credits, &rule.IsActive, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Rule{}, errors.New("pricing rule not found")
		}
		return Rule{}, err
	}
	_ = s.redis.Del(ctx, fmt.Sprintf("pricing:%s:%s:%s", rule.Model, rule.Size, rule.Quality)).Err()
	_ = s.redis.Del(ctx, fmt.Sprintf("pricing:%s:%s:%s", oldModel, oldSize, oldQuality)).Err()
	return rule, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	var model, size, quality string
	err := s.db.QueryRow(ctx, `
		UPDATE pricing_rules
		SET is_active=false, updated_at=now()
		WHERE id=$1
		RETURNING model, size, quality
	`, id).Scan(&model, &size, &quality)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("pricing rule not found")
	}
	if err != nil {
		return err
	}
	_ = s.redis.Del(ctx, fmt.Sprintf("pricing:%s:%s:%s", model, size, quality)).Err()
	return nil
}
