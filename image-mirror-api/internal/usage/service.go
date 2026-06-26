package usage

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	RetentionConfigKey = "usage_log_retention_days"
	DefaultRetention   = 90
	MaxRetention       = 3650
)

type Service struct {
	db *pgxpool.Pool
}

type RecordInput struct {
	UserID            string
	APIKeyID          *string
	ImageGenerationID *string
	Source            string
	Method            string
	Path              string
	IPAddress         string
	UserAgent         string
	Model             string
	Prompt            string
	Size              string
	Quality           string
	ReferenceCount    int
	CreditsCost       int64
	Status            string
	Success           bool
	StatusCode        *int
	DurationMS        *int64
	ErrorMessage      *string
}

type CompleteInput struct {
	ImageGenerationID string
	Status            string
	Success           bool
	StatusCode        *int
	ErrorMessage      *string
}

type ListFilter struct {
	UserQuery  string
	Success    string
	Source     string
	Limit      int32
	Offset     int32
	Before     *time.Time
	After      *time.Time
	PromptLike string
}

type Log struct {
	ID                string     `json:"id"`
	UserID            *string    `json:"userId,omitempty"`
	UserEmail         string     `json:"userEmail"`
	APIKeyID          *string    `json:"apiKeyId,omitempty"`
	APIKeyName        *string    `json:"apiKeyName,omitempty"`
	APIKeyPrefix      *string    `json:"apiKeyPrefix,omitempty"`
	ImageGenerationID *string    `json:"imageGenerationId,omitempty"`
	Source            string     `json:"source"`
	Method            string     `json:"method"`
	Path              string     `json:"path"`
	IPAddress         string     `json:"ipAddress"`
	UserAgent         string     `json:"userAgent"`
	Model             string     `json:"model"`
	Prompt            string     `json:"prompt"`
	Size              string     `json:"size"`
	Quality           string     `json:"quality"`
	ReferenceCount    int        `json:"referenceCount"`
	CreditsCost       int64      `json:"creditsCost"`
	Status            string     `json:"status"`
	Success           bool       `json:"success"`
	StatusCode        *int       `json:"statusCode,omitempty"`
	DurationMS        *int64     `json:"durationMs,omitempty"`
	ErrorMessage      *string    `json:"errorMessage,omitempty"`
	CompletedAt       *time.Time `json:"completedAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

type ListResult struct {
	Data   []Log `json:"data"`
	Total  int64 `json:"total"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

type RetentionSettings struct {
	Days int `json:"days"`
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) Record(ctx context.Context, input RecordInput) (Log, error) {
	normalized := normalizeRecord(input)
	var log Log
	row := s.db.QueryRow(ctx, `
		INSERT INTO usage_logs(
			user_id, user_email, api_key_id, api_key_name, api_key_prefix, image_generation_id,
			source, method, path, ip_address, user_agent, model, prompt, size, quality,
			reference_count, credits_cost, status, success, status_code, duration_ms,
			error_message, completed_at
		)
		SELECT
			NULLIF($1, '')::uuid,
			COALESCE(u.email, ''),
			NULLIF($2::text, '')::uuid,
			ak.name,
			ak.key_prefix,
			NULLIF($3::text, '')::uuid,
			$4, $5, $6, $7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18, $19,
			CASE WHEN $15 <> 'PENDING' THEN now() ELSE NULL END
		FROM (SELECT 1) seed
		LEFT JOIN users u ON u.id = NULLIF($1, '')::uuid
		LEFT JOIN api_keys ak ON ak.id = NULLIF($2::text, '')::uuid
		RETURNING id, user_id, user_email, api_key_id, api_key_name, api_key_prefix,
			image_generation_id, source, method, path, ip_address, user_agent, model, prompt,
			size, quality, reference_count, credits_cost, status, success, status_code,
			duration_ms, error_message, completed_at, created_at, updated_at
	`, normalized.UserID, nullableUUID(normalized.APIKeyID), nullableUUID(normalized.ImageGenerationID),
		normalized.Source, normalized.Method, normalized.Path, normalized.IPAddress, normalized.UserAgent,
		normalized.Model, normalized.Prompt, normalized.Size, normalized.Quality, normalized.ReferenceCount,
		normalized.CreditsCost, normalized.Status, normalized.Success, normalized.StatusCode,
		normalized.DurationMS, normalized.ErrorMessage)
	if err := scanLog(row, &log); err != nil {
		return Log{}, err
	}
	return log, nil
}

func (s *Service) CompleteByImageID(ctx context.Context, input CompleteInput) error {
	imageID := strings.TrimSpace(input.ImageGenerationID)
	if imageID == "" {
		return errors.New("image generation id is required")
	}
	status := normalizeStatus(input.Status, input.Success)
	_, err := s.db.Exec(ctx, `
		UPDATE usage_logs
		SET status=$2,
			success=$3,
			status_code=$4,
			duration_ms=GREATEST(0, (EXTRACT(EPOCH FROM (now() - created_at)) * 1000)::bigint),
			error_message=$5,
			completed_at=now(),
			updated_at=now()
		WHERE image_generation_id=$1 AND status <> 'TIMEOUT'
	`, imageID, status, input.Success, input.StatusCode, input.ErrorMessage)
	return err
}

func (s *Service) MarkProcessingByImageID(ctx context.Context, imageGenerationID string) error {
	imageGenerationID = strings.TrimSpace(imageGenerationID)
	if imageGenerationID == "" {
		return errors.New("image generation id is required")
	}
	_, err := s.db.Exec(ctx, `
		UPDATE usage_logs
		SET status='PROCESSING',
			updated_at=now()
		WHERE image_generation_id=$1 AND status='PENDING'
	`, imageGenerationID)
	return err
}

func (s *Service) SetReferenceCount(ctx context.Context, imageGenerationID string, count int) error {
	imageGenerationID = strings.TrimSpace(imageGenerationID)
	if imageGenerationID == "" {
		return nil
	}
	if count < 0 {
		count = 0
	}
	_, err := s.db.Exec(ctx, `
		UPDATE usage_logs
		SET reference_count=$2, updated_at=now()
		WHERE image_generation_id=$1
	`, imageGenerationID, count)
	return err
}

func (s *Service) List(ctx context.Context, filter ListFilter) (ListResult, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	userQuery := strings.TrimSpace(filter.UserQuery)
	success := strings.ToLower(strings.TrimSpace(filter.Success))
	source := strings.ToUpper(strings.TrimSpace(filter.Source))
	promptLike := strings.TrimSpace(filter.PromptLike)
	if success != "true" && success != "false" {
		success = ""
	}
	if source != "WEB" && source != "API" {
		source = ""
	}

	rows, err := s.db.Query(ctx, `
		WITH filtered AS (
			SELECT *
			FROM usage_logs
			WHERE ($1 = '' OR user_email ILIKE '%' || $1 || '%' OR user_id::text = $1)
				AND ($2 = '' OR success = ($2 = 'true'))
				AND ($3 = '' OR source = $3)
				AND ($4::timestamptz IS NULL OR created_at >= $4)
				AND ($5::timestamptz IS NULL OR created_at < $5)
				AND ($6 = '' OR prompt ILIKE '%' || $6 || '%')
		)
		SELECT id, user_id, user_email, api_key_id, api_key_name, api_key_prefix,
			image_generation_id, source, method, path, ip_address, user_agent, model, prompt,
			size, quality, reference_count, credits_cost, status, success, status_code,
			duration_ms, error_message, completed_at, created_at, updated_at,
			count(*) OVER()
		FROM filtered
		ORDER BY created_at DESC
		LIMIT $7 OFFSET $8
	`, userQuery, success, source, filter.After, filter.Before, promptLike, filter.Limit, filter.Offset)
	if err != nil {
		return ListResult{}, err
	}
	defer rows.Close()

	items := make([]Log, 0)
	var total int64
	for rows.Next() {
		var item Log
		if err := scanLogWithTotal(rows, &item, &total); err != nil {
			return ListResult{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, err
	}
	return ListResult{Data: items, Total: total, Limit: filter.Limit, Offset: filter.Offset}, nil
}

func (s *Service) Retention(ctx context.Context) (RetentionSettings, error) {
	var value string
	err := s.db.QueryRow(ctx, `SELECT value FROM system_config WHERE key=$1`, RetentionConfigKey).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return RetentionSettings{Days: DefaultRetention}, nil
	}
	if err != nil {
		return RetentionSettings{}, err
	}
	return RetentionSettings{Days: normalizeRetention(value)}, nil
}

func (s *Service) SetRetention(ctx context.Context, days int, updatedBy string) (RetentionSettings, error) {
	days = clampRetention(days)
	var updatedByPtr *string
	if strings.TrimSpace(updatedBy) != "" {
		updatedByPtr = &updatedBy
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO system_config(key, value, updated_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (key)
		DO UPDATE SET value=EXCLUDED.value, updated_by=EXCLUDED.updated_by, updated_at=now()
	`, RetentionConfigKey, strconv.Itoa(days), updatedByPtr)
	if err != nil {
		return RetentionSettings{}, err
	}
	return RetentionSettings{Days: days}, nil
}

func (s *Service) DeleteBefore(ctx context.Context, before time.Time, limit int32) (int64, error) {
	if limit <= 0 {
		tag, err := s.db.Exec(ctx, `DELETE FROM usage_logs WHERE created_at < $1`, before)
		if err != nil {
			return 0, err
		}
		return tag.RowsAffected(), nil
	}
	if limit > 10000 {
		limit = 10000
	}
	tag, err := s.db.Exec(ctx, `
		WITH doomed AS (
			SELECT id
			FROM usage_logs
			WHERE created_at < $1
			ORDER BY created_at ASC
			LIMIT $2
		)
		DELETE FROM usage_logs
		WHERE id IN (SELECT id FROM doomed)
	`, before, limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Service) CleanupExpired(ctx context.Context, limit int32) (int64, error) {
	settings, err := s.Retention(ctx)
	if err != nil {
		return 0, err
	}
	if settings.Days <= 0 {
		return 0, nil
	}
	return s.DeleteBefore(ctx, time.Now().AddDate(0, 0, -settings.Days), limit)
}

func normalizeRecord(input RecordInput) RecordInput {
	input.UserID = strings.TrimSpace(input.UserID)
	input.Source = strings.ToUpper(strings.TrimSpace(input.Source))
	if input.Source == "" {
		input.Source = "WEB"
	}
	input.Method = strings.ToUpper(strings.TrimSpace(input.Method))
	input.Path = trimLimit(input.Path, 500)
	input.IPAddress = trimLimit(input.IPAddress, 200)
	input.UserAgent = trimLimit(input.UserAgent, 1000)
	input.Model = trimLimit(input.Model, 200)
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.Size = trimLimit(input.Size, 50)
	input.Quality = trimLimit(input.Quality, 50)
	if input.ReferenceCount < 0 {
		input.ReferenceCount = 0
	}
	if input.CreditsCost < 0 {
		input.CreditsCost = 0
	}
	input.Status = normalizeStatus(input.Status, input.Success)
	return input
}

func normalizeStatus(status string, success bool) string {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status == "" {
		if success {
			return "COMPLETED"
		}
		return "FAILED"
	}
	return trimLimit(status, 50)
}

func trimLimit(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func nullableUUID(value *string) any {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func normalizeRetention(value string) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return DefaultRetention
	}
	return clampRetention(parsed)
}

func clampRetention(days int) int {
	if days < 0 {
		return 0
	}
	if days > MaxRetention {
		return MaxRetention
	}
	return days
}

func scanLog(row pgx.Row, item *Log) error {
	return row.Scan(
		&item.ID,
		&item.UserID,
		&item.UserEmail,
		&item.APIKeyID,
		&item.APIKeyName,
		&item.APIKeyPrefix,
		&item.ImageGenerationID,
		&item.Source,
		&item.Method,
		&item.Path,
		&item.IPAddress,
		&item.UserAgent,
		&item.Model,
		&item.Prompt,
		&item.Size,
		&item.Quality,
		&item.ReferenceCount,
		&item.CreditsCost,
		&item.Status,
		&item.Success,
		&item.StatusCode,
		&item.DurationMS,
		&item.ErrorMessage,
		&item.CompletedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
}

func scanLogWithTotal(row pgx.Row, item *Log, total *int64) error {
	return row.Scan(
		&item.ID,
		&item.UserID,
		&item.UserEmail,
		&item.APIKeyID,
		&item.APIKeyName,
		&item.APIKeyPrefix,
		&item.ImageGenerationID,
		&item.Source,
		&item.Method,
		&item.Path,
		&item.IPAddress,
		&item.UserAgent,
		&item.Model,
		&item.Prompt,
		&item.Size,
		&item.Quality,
		&item.ReferenceCount,
		&item.CreditsCost,
		&item.Status,
		&item.Success,
		&item.StatusCode,
		&item.DurationMS,
		&item.ErrorMessage,
		&item.CompletedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
		total,
	)
}
