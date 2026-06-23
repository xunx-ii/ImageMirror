package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv              string
	AppRole             string
	HTTPAddr            string
	PublicBaseURL       string
	DatabaseURL         string
	RedisAddr           string
	RedisPassword       string
	RedisDB             int
	JWTSecret           string
	RefreshTokenTTL     time.Duration
	AccessTokenTTL      time.Duration
	OpenAIAPIKey        string
	OpenAIBaseURL       string
	OpenAITimeout       time.Duration
	DefaultImageModel   string
	StorageRoot         string
	ImageRetention      time.Duration
	DeveloperAPITimeout time.Duration
	RateLimitPerMinute  int
	CORSOrigins         []string
	AdminEmail          string
	AdminPassword       string
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:              getenv("APP_ENV", "development"),
		AppRole:             getenv("APP_ROLE", "api"),
		HTTPAddr:            getenv("HTTP_ADDR", ":8080"),
		PublicBaseURL:       strings.TrimRight(getenv("PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
		DatabaseURL:         getenv("DATABASE_URL", "postgres://imagemirror:imagemirror@localhost:5432/imagemirror?sslmode=disable"),
		RedisAddr:           getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:       getenv("REDIS_PASSWORD", ""),
		RedisDB:             getenvInt("REDIS_DB", 0),
		JWTSecret:           getenv("JWT_SECRET", "dev-secret-change-me"),
		RefreshTokenTTL:     getenvDuration("REFRESH_TOKEN_TTL", 7*24*time.Hour),
		AccessTokenTTL:      getenvDuration("ACCESS_TOKEN_TTL", 2*time.Hour),
		OpenAIAPIKey:        getenv("OPENAI_API_KEY", ""),
		OpenAIBaseURL:       strings.TrimRight(getenv("OPENAI_BASE_URL", "https://api.openai.com"), "/"),
		OpenAITimeout:       getenvDuration("OPENAI_TIMEOUT", 5*time.Minute),
		DefaultImageModel:   getenv("DEFAULT_IMAGE_MODEL", "gpt-image-2"),
		StorageRoot:         getenv("STORAGE_ROOT", "./data/images"),
		ImageRetention:      getenvDuration("IMAGE_RETENTION", 24*time.Hour),
		DeveloperAPITimeout: getenvDuration("DEVELOPER_API_TIMEOUT", 300*time.Second),
		RateLimitPerMinute:  getenvInt("RATE_LIMIT_PER_MINUTE", 120),
		CORSOrigins:         splitCSV(getenv("CORS_ORIGINS", "http://localhost:5173,http://localhost:3000")),
		AdminEmail:          getenv("ADMIN_EMAIL", "admin@example.com"),
		AdminPassword:       getenv("ADMIN_PASSWORD", "admin123456"),
	}
	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required")
	}
	if len(cfg.JWTSecret) < 16 {
		return cfg, fmt.Errorf("JWT_SECRET must be at least 16 characters")
	}
	return cfg, nil
}

func getenv(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	if value, err := time.ParseDuration(raw); err == nil {
		return value
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
