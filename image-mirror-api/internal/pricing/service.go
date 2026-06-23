package pricing

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	minImagePixels = 655360
	maxImageEdge   = 3840
	maxImagePixels = 3840 * 2160
)

var customSizePattern = regexp.MustCompile(`^(\d+)x(\d+)$`)

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
	bucket, err := ResolutionBucket(size)
	if err != nil {
		return 0, err
	}
	cacheKey := fmt.Sprintf("pricing:%s:%s:%s", model, bucket, quality)
	cached, err := s.redis.Get(ctx, cacheKey).Int64()
	if err == nil && cached > 0 {
		return cached, nil
	}
	var credits int64
	err = s.db.QueryRow(ctx, `
		SELECT credits FROM pricing_rules
		WHERE model=$1 AND size=$2 AND quality=$3 AND is_active=true
	`, model, bucket, quality).Scan(&credits)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, errors.New("pricing rule not found")
	}
	if err != nil {
		return 0, err
	}
	_ = s.redis.Set(ctx, cacheKey, credits, 10*time.Minute).Err()
	return credits, nil
}

func ResolutionBucket(size string) (string, error) {
	width, height, err := ParseCustomSize(size)
	if err != nil {
		return "", err
	}
	longest := width
	if height > longest {
		longest = height
	}
	switch {
	case longest <= 1024:
		return "1k", nil
	case longest <= 2048:
		return "2k", nil
	default:
		return "4k", nil
	}
}

func ParseCustomSize(size string) (int, int, error) {
	size = strings.ToLower(strings.TrimSpace(size))
	matches := customSizePattern.FindStringSubmatch(size)
	if matches == nil {
		return 0, 0, errors.New("size must use WIDTHxHEIGHT format")
	}
	width, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, 0, errors.New("invalid width")
	}
	height, err := strconv.Atoi(matches[2])
	if err != nil {
		return 0, 0, errors.New("invalid height")
	}
	if width <= 0 || height <= 0 {
		return 0, 0, errors.New("width and height must be positive")
	}
	if width%16 != 0 || height%16 != 0 {
		return 0, 0, errors.New("width and height must be multiples of 16")
	}
	if width > maxImageEdge || height > maxImageEdge {
		return 0, 0, errors.New("longest edge must not exceed 3840")
	}
	if width*height < minImagePixels {
		return 0, 0, errors.New("total pixels must be at least 655360")
	}
	if width*height > maxImagePixels {
		return 0, 0, errors.New("total pixels must not exceed 3840x2160")
	}
	if width*3 < height || height*3 < width {
		return 0, 0, errors.New("aspect ratio must be between 1:3 and 3:1")
	}
	return width, height, nil
}

func NormalizeBucket(size string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(size)) {
	case "1k":
		return "1k", nil
	case "2k":
		return "2k", nil
	case "4k":
		return "4k", nil
	default:
		return "", errors.New("pricing size must be 1k, 2k, or 4k")
	}
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
	size, err := NormalizeBucket(size)
	if err != nil {
		return Rule{}, err
	}
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
	size, err := NormalizeBucket(size)
	if err != nil {
		return Rule{}, err
	}
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
