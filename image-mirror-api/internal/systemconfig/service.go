package systemconfig

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
	KeyOpenAIAPIKey  = "openai_api_key"
	KeyOpenAIBaseURL = "openai_base_url"
	KeyEPayGateway   = "epay_gateway"
	KeyEPayPID       = "epay_pid"
	KeyEPayKey       = "epay_key"
	KeyEPayName      = "epay_name"
	KeyEPayRate      = "epay_credits_per_yuan"
	KeyEPayEnabled   = "epay_enabled"
)

type OpenAISettings struct {
	OpenAIBaseURL      string `json:"openaiBaseUrl"`
	OpenAIAPIKey       string `json:"openaiApiKey,omitempty"`
	HasOpenAIAPIKey    bool   `json:"hasOpenaiApiKey"`
	UsesEnvironmentKey bool   `json:"usesEnvironmentKey"`
}

type EPaySettings struct {
	Gateway        string `json:"gateway"`
	PID            string `json:"pid"`
	Key            string `json:"key,omitempty"`
	HasKey         bool   `json:"hasKey"`
	Name           string `json:"name"`
	CreditsPerYuan int64  `json:"creditsPerYuan"`
	Enabled        bool   `json:"enabled"`
}

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) GetOpenAI(ctx context.Context, fallbackAPIKey string, fallbackBaseURL string) (string, string, error) {
	apiKey, err := s.value(ctx, KeyOpenAIAPIKey)
	if err != nil {
		return "", "", err
	}
	baseURL, err := s.value(ctx, KeyOpenAIBaseURL)
	if err != nil {
		return "", "", err
	}
	if apiKey == "" {
		apiKey = fallbackAPIKey
	}
	if baseURL == "" {
		baseURL = fallbackBaseURL
	}
	return strings.TrimSpace(apiKey), strings.TrimRight(strings.TrimSpace(baseURL), "/"), nil
}

func (s *Service) PublicOpenAI(ctx context.Context, fallbackAPIKey string, fallbackBaseURL string) (OpenAISettings, error) {
	storedKey, err := s.value(ctx, KeyOpenAIAPIKey)
	if err != nil {
		return OpenAISettings{}, err
	}
	apiKey, baseURL, err := s.GetOpenAI(ctx, fallbackAPIKey, fallbackBaseURL)
	if err != nil {
		return OpenAISettings{}, err
	}
	return OpenAISettings{
		OpenAIBaseURL:      baseURL,
		HasOpenAIAPIKey:    apiKey != "",
		UsesEnvironmentKey: storedKey == "" && fallbackAPIKey != "",
	}, nil
}

func (s *Service) UpdateOpenAI(ctx context.Context, baseURL string, apiKey *string, updatedBy string) (OpenAISettings, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	if err := s.upsert(ctx, KeyOpenAIBaseURL, baseURL, updatedBy); err != nil {
		return OpenAISettings{}, err
	}
	if apiKey != nil {
		trimmed := strings.TrimSpace(*apiKey)
		if trimmed != "" {
			if err := s.upsert(ctx, KeyOpenAIAPIKey, trimmed, updatedBy); err != nil {
				return OpenAISettings{}, err
			}
		}
	}
	return s.PublicOpenAI(ctx, "", baseURL)
}

func (s *Service) PublicEPay(ctx context.Context) (EPaySettings, error) {
	gateway, err := s.value(ctx, KeyEPayGateway)
	if err != nil {
		return EPaySettings{}, err
	}
	pid, err := s.value(ctx, KeyEPayPID)
	if err != nil {
		return EPaySettings{}, err
	}
	key, err := s.value(ctx, KeyEPayKey)
	if err != nil {
		return EPaySettings{}, err
	}
	name, err := s.value(ctx, KeyEPayName)
	if err != nil {
		return EPaySettings{}, err
	}
	rate, err := s.value(ctx, KeyEPayRate)
	if err != nil {
		return EPaySettings{}, err
	}
	enabled, err := s.value(ctx, KeyEPayEnabled)
	if err != nil {
		return EPaySettings{}, err
	}
	settings := EPaySettings{
		Gateway:        strings.TrimRight(strings.TrimSpace(gateway), "/"),
		PID:            strings.TrimSpace(pid),
		Name:           strings.TrimSpace(name),
		CreditsPerYuan: 100,
		Enabled:        enabled == "true",
		HasKey:         strings.TrimSpace(key) != "",
	}
	if settings.Gateway == "" {
		settings.Gateway = "https://pay.example.com"
	}
	if settings.Name == "" {
		settings.Name = "ImageMirror credits"
	}
	if parsed, ok := parsePositiveInt(rate); ok {
		settings.CreditsPerYuan = parsed
	}
	return settings, nil
}

func (s *Service) GetEPay(ctx context.Context) (EPaySettings, string, error) {
	settings, err := s.PublicEPay(ctx)
	if err != nil {
		return EPaySettings{}, "", err
	}
	key, err := s.value(ctx, KeyEPayKey)
	if err != nil {
		return EPaySettings{}, "", err
	}
	return settings, strings.TrimSpace(key), nil
}

func (s *Service) UpdateEPay(ctx context.Context, settings EPaySettings, apiKey *string, updatedBy string) (EPaySettings, error) {
	settings.Gateway = strings.TrimRight(strings.TrimSpace(settings.Gateway), "/")
	settings.PID = strings.TrimSpace(settings.PID)
	settings.Name = strings.TrimSpace(settings.Name)
	if settings.Gateway == "" {
		settings.Gateway = "https://pay.example.com"
	}
	if settings.Name == "" {
		settings.Name = "ImageMirror credits"
	}
	if settings.CreditsPerYuan <= 0 {
		settings.CreditsPerYuan = 100
	}
	values := map[string]string{
		KeyEPayGateway: settings.Gateway,
		KeyEPayPID:     settings.PID,
		KeyEPayName:    settings.Name,
		KeyEPayRate:    strconv.FormatInt(settings.CreditsPerYuan, 10),
		KeyEPayEnabled: strconv.FormatBool(settings.Enabled),
	}
	for key, value := range values {
		if err := s.upsert(ctx, key, value, updatedBy); err != nil {
			return EPaySettings{}, err
		}
	}
	if apiKey != nil {
		trimmed := strings.TrimSpace(*apiKey)
		if trimmed != "" {
			if err := s.upsert(ctx, KeyEPayKey, trimmed, updatedBy); err != nil {
				return EPaySettings{}, err
			}
		}
	}
	return s.PublicEPay(ctx)
}

func (s *Service) value(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRow(ctx, `SELECT value FROM system_config WHERE key=$1`, key).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (s *Service) upsert(ctx context.Context, key string, value string, updatedBy string) error {
	var updatedByPtr *string
	if updatedBy != "" {
		updatedByPtr = &updatedBy
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO system_config(key, value, updated_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (key)
		DO UPDATE SET value=EXCLUDED.value, updated_by=EXCLUDED.updated_by, updated_at=$4
	`, key, value, updatedByPtr, time.Now())
	return err
}

func parsePositiveInt(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed, err == nil && parsed > 0
}
