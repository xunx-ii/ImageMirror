package systemconfig

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	openAIEndpointFailureThreshold = 3
	openAIEndpointCircuitDuration  = 5 * time.Minute
)

type OpenAIEndpoint struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	BaseURL          string     `json:"baseUrl"`
	APIKey           string     `json:"apiKey,omitempty"`
	HasAPIKey        bool       `json:"hasApiKey"`
	Enabled          bool       `json:"enabled"`
	Schedulable      bool       `json:"schedulable"`
	Priority         int        `json:"priority"`
	FailureCount     int        `json:"failureCount"`
	CircuitOpenUntil *time.Time `json:"circuitOpenUntil,omitempty"`
	LastError        *string    `json:"lastError,omitempty"`
	LastUsedAt       *time.Time `json:"lastUsedAt,omitempty"`
	LastSuccessAt    *time.Time `json:"lastSuccessAt,omitempty"`
	LastFailureAt    *time.Time `json:"lastFailureAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type OpenAIEndpointInput struct {
	Name        string
	BaseURL     string
	APIKey      *string
	Enabled     bool
	Schedulable bool
	Priority    int
}

type OpenAIEndpointCredential struct {
	ID      string
	Name    string
	BaseURL string
	APIKey  string
}

func (s *Service) ListOpenAIEndpoints(ctx context.Context) ([]OpenAIEndpoint, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, base_url, api_key <> '', enabled, schedulable, priority, failure_count,
			circuit_open_until, last_error, last_used_at, last_success_at, last_failure_at, created_at, updated_at
		FROM openai_endpoints
		ORDER BY priority ASC, created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]OpenAIEndpoint, 0)
	for rows.Next() {
		item, err := scanOpenAIEndpoint(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) CreateOpenAIEndpoint(ctx context.Context, input OpenAIEndpointInput, updatedBy string) (OpenAIEndpoint, error) {
	normalized, err := normalizeOpenAIEndpointInput(input, true)
	if err != nil {
		return OpenAIEndpoint{}, err
	}
	var updatedByPtr *string
	if updatedBy != "" {
		updatedByPtr = &updatedBy
	}
	row := s.db.QueryRow(ctx, `
		INSERT INTO openai_endpoints(name, base_url, api_key, enabled, schedulable, priority, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, name, base_url, api_key <> '', enabled, schedulable, priority, failure_count,
			circuit_open_until, last_error, last_used_at, last_success_at, last_failure_at, created_at, updated_at
	`, normalized.Name, normalized.BaseURL, strings.TrimSpace(*normalized.APIKey), normalized.Enabled, normalized.Schedulable, normalized.Priority, updatedByPtr)
	return scanOpenAIEndpoint(row)
}

func (s *Service) UpdateOpenAIEndpoint(ctx context.Context, id string, input OpenAIEndpointInput, updatedBy string) (OpenAIEndpoint, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return OpenAIEndpoint{}, errors.New("endpoint id is required")
	}
	normalized, err := normalizeOpenAIEndpointInput(input, false)
	if err != nil {
		return OpenAIEndpoint{}, err
	}
	var updatedByPtr *string
	if updatedBy != "" {
		updatedByPtr = &updatedBy
	}
	apiKey := ""
	if normalized.APIKey != nil {
		apiKey = strings.TrimSpace(*normalized.APIKey)
	}
	row := s.db.QueryRow(ctx, `
		UPDATE openai_endpoints
		SET name=$2,
			base_url=$3,
			api_key=CASE WHEN $4 <> '' THEN $4 ELSE api_key END,
			enabled=$5,
			schedulable=$6,
			priority=$7,
			updated_by=$8,
			updated_at=now()
		WHERE id=$1
		RETURNING id, name, base_url, api_key <> '', enabled, schedulable, priority, failure_count,
			circuit_open_until, last_error, last_used_at, last_success_at, last_failure_at, created_at, updated_at
	`, id, normalized.Name, normalized.BaseURL, apiKey, normalized.Enabled, normalized.Schedulable, normalized.Priority, updatedByPtr)
	return scanOpenAIEndpoint(row)
}

func (s *Service) DeleteOpenAIEndpoint(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM openai_endpoints WHERE id=$1`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Service) ResetOpenAIEndpointCircuit(ctx context.Context, id string) (OpenAIEndpoint, error) {
	row := s.db.QueryRow(ctx, `
		UPDATE openai_endpoints
		SET failure_count=0, circuit_open_until=NULL, last_error=NULL, updated_at=now()
		WHERE id=$1
		RETURNING id, name, base_url, api_key <> '', enabled, schedulable, priority, failure_count,
			circuit_open_until, last_error, last_used_at, last_success_at, last_failure_at, created_at, updated_at
	`, strings.TrimSpace(id))
	return scanOpenAIEndpoint(row)
}

func (s *Service) OpenAIEndpointCandidates(ctx context.Context, fallbackAPIKey string, fallbackBaseURL string) ([]OpenAIEndpointCredential, error) {
	var total int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM openai_endpoints`).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, name, base_url, api_key
		FROM openai_endpoints
		WHERE enabled=true
			AND schedulable=true
			AND api_key <> ''
			AND (circuit_open_until IS NULL OR circuit_open_until <= now())
		ORDER BY priority ASC, last_used_at ASC NULLS FIRST, created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]OpenAIEndpointCredential, 0)
	for rows.Next() {
		var item OpenAIEndpointCredential
		if err := rows.Scan(&item.ID, &item.Name, &item.BaseURL, &item.APIKey); err != nil {
			return nil, err
		}
		item.BaseURL = normalizeOpenAIBaseURL(item.BaseURL)
		item.APIKey = strings.TrimSpace(item.APIKey)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) > 0 {
		return items, nil
	}
	if total > 0 {
		return nil, errors.New("no schedulable openai endpoint is available")
	}
	fallbackAPIKey = strings.TrimSpace(fallbackAPIKey)
	if fallbackAPIKey == "" {
		return nil, nil
	}
	return []OpenAIEndpointCredential{{
		Name:    "环境变量",
		BaseURL: normalizeOpenAIBaseURL(fallbackBaseURL),
		APIKey:  fallbackAPIKey,
	}}, nil
}

func (s *Service) MarkOpenAIEndpointAttempt(ctx context.Context, id string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	_, _ = s.db.Exec(ctx, `UPDATE openai_endpoints SET last_used_at=now(), updated_at=now() WHERE id=$1`, id)
}

func (s *Service) MarkOpenAIEndpointSuccess(ctx context.Context, id string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	_, _ = s.db.Exec(ctx, `
		UPDATE openai_endpoints
		SET failure_count=0, circuit_open_until=NULL, last_error=NULL, last_success_at=now(), updated_at=now()
		WHERE id=$1
	`, id)
}

func (s *Service) MarkOpenAIEndpointFailure(ctx context.Context, id string, message string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	message = strings.TrimSpace(message)
	if len(message) > 1000 {
		message = message[:1000]
	}
	_, _ = s.db.Exec(ctx, `
		UPDATE openai_endpoints
		SET failure_count=failure_count + 1,
			circuit_open_until=CASE
				WHEN failure_count + 1 >= $2 THEN now() + $3::interval
				ELSE circuit_open_until
			END,
			last_error=$4,
			last_failure_at=now(),
			updated_at=now()
		WHERE id=$1
	`, id, openAIEndpointFailureThreshold, fmt.Sprintf("%f seconds", openAIEndpointCircuitDuration.Seconds()), message)
}

func normalizeOpenAIEndpointInput(input OpenAIEndpointInput, requireKey bool) (OpenAIEndpointInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.BaseURL = normalizeOpenAIBaseURL(input.BaseURL)
	if input.BaseURL == "" {
		input.BaseURL = "https://api.openai.com"
	}
	if input.Name == "" {
		input.Name = input.BaseURL
	}
	if input.Priority <= 0 {
		input.Priority = 100
	}
	if input.Priority > 10000 {
		input.Priority = 10000
	}
	if requireKey && (input.APIKey == nil || strings.TrimSpace(*input.APIKey) == "") {
		return input, errors.New("api key is required")
	}
	return input, nil
}

func normalizeOpenAIBaseURL(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	return strings.TrimSuffix(value, "/v1")
}

func scanOpenAIEndpoint(row pgx.Row) (OpenAIEndpoint, error) {
	var item OpenAIEndpoint
	if err := row.Scan(
		&item.ID,
		&item.Name,
		&item.BaseURL,
		&item.HasAPIKey,
		&item.Enabled,
		&item.Schedulable,
		&item.Priority,
		&item.FailureCount,
		&item.CircuitOpenUntil,
		&item.LastError,
		&item.LastUsedAt,
		&item.LastSuccessAt,
		&item.LastFailureAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return OpenAIEndpoint{}, err
	}
	return item, nil
}
