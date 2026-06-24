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
	KeyMaxResolution = "max_resolution_bucket"
	KeySiteTitle     = "site_title"
	KeySiteSubtitle  = "site_subtitle"

	KeyImageGenerationConcurrency = "image_generation_concurrency"

	DefaultImageGenerationConcurrency = 10
	MaxImageGenerationConcurrency     = 100
)

type OpenAISettings struct {
	OpenAIBaseURL      string           `json:"openaiBaseUrl"`
	OpenAIAPIKey       string           `json:"openaiApiKey,omitempty"`
	HasOpenAIAPIKey    bool             `json:"hasOpenaiApiKey"`
	UsesEnvironmentKey bool             `json:"usesEnvironmentKey"`
	Endpoints          []OpenAIEndpoint `json:"endpoints"`
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

type PlatformSettings struct {
	MaxResolutionBucket string `json:"maxResolutionBucket"`
	Allow4K             bool   `json:"allow4k"`
	SiteTitle           string `json:"siteTitle"`
	SiteSubtitle        string `json:"siteSubtitle"`
}

type GenerationSettings struct {
	ImageGenerationConcurrency int `json:"imageGenerationConcurrency"`
}

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) PublicPlatform(ctx context.Context) (PlatformSettings, error) {
	bucket, err := s.value(ctx, KeyMaxResolution)
	if err != nil {
		return PlatformSettings{}, err
	}
	title, err := s.value(ctx, KeySiteTitle)
	if err != nil {
		return PlatformSettings{}, err
	}
	subtitle, err := s.value(ctx, KeySiteSubtitle)
	if err != nil {
		return PlatformSettings{}, err
	}
	return normalizePlatform(bucket, title, subtitle), nil
}

func (s *Service) UpdatePlatform(ctx context.Context, maxResolutionBucket string, siteTitle *string, siteSubtitle *string, updatedBy string) (PlatformSettings, error) {
	current, err := s.PublicPlatform(ctx)
	if err != nil {
		return PlatformSettings{}, err
	}
	if strings.TrimSpace(maxResolutionBucket) == "" {
		maxResolutionBucket = current.MaxResolutionBucket
	}
	title := current.SiteTitle
	if siteTitle != nil {
		title = *siteTitle
	}
	subtitle := current.SiteSubtitle
	if siteSubtitle != nil {
		subtitle = *siteSubtitle
	}
	settings := normalizePlatform(maxResolutionBucket, title, subtitle)
	values := map[string]string{
		KeyMaxResolution: settings.MaxResolutionBucket,
		KeySiteTitle:     settings.SiteTitle,
		KeySiteSubtitle:  settings.SiteSubtitle,
	}
	for key, value := range values {
		if err := s.upsert(ctx, key, value, updatedBy); err != nil {
			return PlatformSettings{}, err
		}
	}
	return settings, nil
}

func (s *Service) MaxResolutionBucket(ctx context.Context) (string, error) {
	settings, err := s.PublicPlatform(ctx)
	if err != nil {
		return "", err
	}
	return settings.MaxResolutionBucket, nil
}

func (s *Service) GenerationSettings(ctx context.Context) (GenerationSettings, error) {
	value, err := s.value(ctx, KeyImageGenerationConcurrency)
	if err != nil {
		return GenerationSettings{}, err
	}
	concurrency := DefaultImageGenerationConcurrency
	if parsed, ok := parsePositiveInt(value); ok {
		concurrency = normalizeImageGenerationConcurrency(int(parsed))
	}
	return GenerationSettings{ImageGenerationConcurrency: concurrency}, nil
}

func (s *Service) UpdateGenerationSettings(ctx context.Context, imageGenerationConcurrency int, updatedBy string) (GenerationSettings, error) {
	settings := GenerationSettings{
		ImageGenerationConcurrency: normalizeImageGenerationConcurrency(imageGenerationConcurrency),
	}
	if err := s.upsert(ctx, KeyImageGenerationConcurrency, strconv.Itoa(settings.ImageGenerationConcurrency), updatedBy); err != nil {
		return GenerationSettings{}, err
	}
	return settings, nil
}

func (s *Service) GetOpenAI(ctx context.Context, fallbackAPIKey string, fallbackBaseURL string) (string, string, error) {
	candidates, err := s.OpenAIEndpointCandidates(ctx, fallbackAPIKey, fallbackBaseURL)
	if err != nil {
		return "", "", err
	}
	if len(candidates) > 0 {
		return candidates[0].APIKey, candidates[0].BaseURL, nil
	}
	return "", "", nil
}

func (s *Service) PublicOpenAI(ctx context.Context, fallbackAPIKey string, fallbackBaseURL string) (OpenAISettings, error) {
	endpoints, err := s.ListOpenAIEndpoints(ctx)
	if err != nil {
		return OpenAISettings{}, err
	}
	settings := OpenAISettings{
		OpenAIBaseURL:      normalizeOpenAIBaseURL(fallbackBaseURL),
		UsesEnvironmentKey: len(endpoints) == 0 && strings.TrimSpace(fallbackAPIKey) != "",
		Endpoints:          endpoints,
	}
	if len(endpoints) > 0 {
		settings.OpenAIBaseURL = endpoints[0].BaseURL
		settings.HasOpenAIAPIKey = false
		for _, endpoint := range endpoints {
			if endpoint.HasAPIKey {
				settings.HasOpenAIAPIKey = true
				break
			}
		}
		return settings, nil
	}
	settings.HasOpenAIAPIKey = strings.TrimSpace(fallbackAPIKey) != ""
	return settings, nil
}

func (s *Service) UpdateOpenAI(ctx context.Context, baseURL string, apiKey *string, updatedBy string) (OpenAISettings, error) {
	baseURL = normalizeOpenAIBaseURL(baseURL)
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	endpoints, err := s.ListOpenAIEndpoints(ctx)
	if err != nil {
		return OpenAISettings{}, err
	}
	input := OpenAIEndpointInput{
		Name:        "默认节点",
		BaseURL:     baseURL,
		APIKey:      apiKey,
		Enabled:     true,
		Schedulable: true,
		Priority:    100,
	}
	if len(endpoints) == 0 {
		if apiKey == nil || strings.TrimSpace(*apiKey) == "" {
			return OpenAISettings{}, errors.New("api key is required")
		}
		if _, err := s.CreateOpenAIEndpoint(ctx, input, updatedBy); err != nil {
			return OpenAISettings{}, err
		}
		return s.PublicOpenAI(ctx, "", baseURL)
	}
	first := endpoints[0]
	input.Name = first.Name
	input.Enabled = first.Enabled
	input.Schedulable = first.Schedulable
	input.Priority = first.Priority
	if _, err := s.UpdateOpenAIEndpoint(ctx, first.ID, input, updatedBy); err != nil {
		return OpenAISettings{}, err
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
	if settings.CreditsPerYuan > 1_000_000 {
		return EPaySettings{}, errors.New("creditsPerYuan is too large")
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

func normalizeImageGenerationConcurrency(value int) int {
	if value <= 0 {
		return DefaultImageGenerationConcurrency
	}
	if value > MaxImageGenerationConcurrency {
		return MaxImageGenerationConcurrency
	}
	return value
}

func normalizePlatform(maxResolutionBucket string, siteTitle string, siteSubtitle string) PlatformSettings {
	bucket := strings.ToLower(strings.TrimSpace(maxResolutionBucket))
	if bucket != "2k" {
		bucket = "4k"
	}
	title := strings.TrimSpace(siteTitle)
	if title == "" {
		title = "IM"
	}
	subtitle := strings.TrimSpace(siteSubtitle)
	if subtitle == "" {
		subtitle = "AI图像生成平台"
	}
	return PlatformSettings{
		MaxResolutionBucket: bucket,
		Allow4K:             bucket == "4k",
		SiteTitle:           title,
		SiteSubtitle:        subtitle,
	}
}
