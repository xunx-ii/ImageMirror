package apikeys

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type APIKey struct {
	ID         string     `json:"id"`
	UserID     string     `json:"userId"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"keyPrefix"`
	Status     string     `json:"status"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}

type CreatedKey struct {
	APIKey
	Plaintext string `json:"plaintext"`
}

type Service struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewService(db *pgxpool.Pool, redisClient *redis.Client) *Service {
	return &Service{db: db, redis: redisClient}
}

func (s *Service) Create(ctx context.Context, userID string, name string) (CreatedKey, error) {
	plain, err := randomKey()
	if err != nil {
		return CreatedKey{}, err
	}
	hash := Hash(plain)
	prefix := plain[:14]
	row := s.db.QueryRow(ctx, `
		INSERT INTO api_keys(user_id, name, key_prefix, key_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, name, key_prefix, status, last_used_at, created_at, revoked_at
	`, userID, strings.TrimSpace(name), prefix, hash)
	key, err := scanKey(row)
	if err != nil {
		return CreatedKey{}, err
	}
	return CreatedKey{APIKey: key, Plaintext: plain}, nil
}

func (s *Service) List(ctx context.Context, userID string) ([]APIKey, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, name, key_prefix, status, last_used_at, created_at, revoked_at
		FROM api_keys
		WHERE user_id=$1 AND status='ACTIVE'
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]APIKey, 0)
	for rows.Next() {
		key, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, rows.Err()
}

func (s *Service) Revoke(ctx context.Context, userID string, keyID string) error {
	var hash string
	err := s.db.QueryRow(ctx, `
		DELETE FROM api_keys
		WHERE id=$1 AND user_id=$2
		RETURNING key_hash
	`, keyID, userID).Scan(&hash)
	if err != nil {
		return err
	}
	_ = s.redis.Del(ctx, "apikey:"+hash).Err()
	return nil
}

func (s *Service) Lookup(ctx context.Context, rawKey string) (string, string, error) {
	hash := Hash(rawKey)
	cacheKey := "apikey:" + hash
	cached, err := s.redis.HGetAll(ctx, cacheKey).Result()
	if err == nil && cached["user_id"] != "" && cached["key_id"] != "" {
		return cached["user_id"], cached["key_id"], nil
	}
	var userID string
	var keyID string
	err = s.db.QueryRow(ctx, `
		SELECT ak.user_id, ak.id
		FROM api_keys ak
		JOIN users u ON u.id = ak.user_id
		WHERE ak.key_hash=$1 AND ak.status='ACTIVE' AND u.status='ACTIVE'
	`, hash).Scan(&userID, &keyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", errors.New("invalid api key")
	}
	if err != nil {
		return "", "", err
	}
	_, _ = s.db.Exec(ctx, `UPDATE api_keys SET last_used_at=now() WHERE id=$1`, keyID)
	_ = s.redis.HSet(ctx, cacheKey, map[string]any{"user_id": userID, "key_id": keyID}).Err()
	_ = s.redis.Expire(ctx, cacheKey, 5*time.Minute).Err()
	return userID, keyID, nil
}

func Hash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func randomKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "imk_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func scanKey(row pgx.Row) (APIKey, error) {
	var key APIKey
	if err := row.Scan(&key.ID, &key.UserID, &key.Name, &key.KeyPrefix, &key.Status, &key.LastUsedAt, &key.CreatedAt, &key.RevokedAt); err != nil {
		return APIKey{}, err
	}
	return key, nil
}
