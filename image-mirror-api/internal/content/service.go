package content

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/linxunxi/image-mirror/internal/storage"
)

type Page struct {
	Key       string    `json:"key"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	IsActive  bool      `json:"isActive"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Asset struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Filename   string    `json:"filename"`
	StorageKey string    `json:"-"`
	URL        string    `json:"url"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Service struct {
	db      *pgxpool.Pool
	storage *storage.Local
}

func NewService(db *pgxpool.Pool, storageSvc *storage.Local) *Service {
	return &Service{db: db, storage: storageSvc}
}

func (s *Service) PublicPage(ctx context.Context, key string) (Page, error) {
	row := s.db.QueryRow(ctx, `
		SELECT key, title, body, is_active, updated_at
		FROM site_content
		WHERE key=$1 AND is_active=true
	`, key)
	return scanPage(row)
}

func (s *Service) AdminPage(ctx context.Context, key string) (Page, error) {
	row := s.db.QueryRow(ctx, `
		SELECT key, title, body, is_active, updated_at
		FROM site_content
		WHERE key=$1
	`, key)
	return scanPage(row)
}

func (s *Service) UpdatePage(ctx context.Context, key string, title string, body string, active bool, adminID string) (Page, error) {
	key = normalizeKey(key)
	if key == "" {
		return Page{}, errors.New("invalid content key")
	}
	row := s.db.QueryRow(ctx, `
		INSERT INTO site_content(key, title, body, is_active, updated_by, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (key)
		DO UPDATE SET title=EXCLUDED.title, body=EXCLUDED.body, is_active=EXCLUDED.is_active, updated_by=EXCLUDED.updated_by, updated_at=now()
		RETURNING key, title, body, is_active, updated_at
	`, key, strings.TrimSpace(title), body, active, nullableString(adminID))
	return scanPage(row)
}

func (s *Service) UploadAsset(ctx context.Context, kind string, userID string, header *multipart.FileHeader) (Asset, error) {
	if header == nil {
		return Asset{}, errors.New("file is required")
	}
	kind = normalizeKey(kind)
	if kind == "" {
		kind = "docs"
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
	default:
		return Asset{}, errors.New("asset must be png, jpg, jpeg, webp, or gif")
	}
	if header.Size > 10<<20 {
		return Asset{}, errors.New("asset must be 10MB or smaller")
	}
	file, err := header.Open()
	if err != nil {
		return Asset{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (10<<20)+1))
	if err != nil {
		return Asset{}, err
	}
	if len(data) == 0 {
		return Asset{}, errors.New("asset is empty")
	}
	if len(data) > 10<<20 {
		return Asset{}, errors.New("asset must be 10MB or smaller")
	}
	id := uuid.NewString()
	key, err := s.storage.SaveContentAsset(ctx, userID, id, header.Filename, data)
	if err != nil {
		return Asset{}, err
	}
	url := "/api/content/assets/" + id
	row := s.db.QueryRow(ctx, `
		INSERT INTO content_assets(id, kind, filename, storage_key, url, uploaded_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, kind, filename, storage_key, url, created_at
	`, id, kind, header.Filename, key, url, nullableString(userID))
	asset, err := scanAsset(row)
	if err != nil {
		_ = s.storage.DeleteImage(ctx, key)
		return Asset{}, err
	}
	return asset, nil
}

func (s *Service) ReadAsset(ctx context.Context, id string) ([]byte, string, error) {
	var key string
	var filename string
	if err := s.db.QueryRow(ctx, `SELECT storage_key, filename FROM content_assets WHERE id=$1`, id).Scan(&key, &filename); err != nil {
		return nil, "", err
	}
	data, err := s.storage.ReadFile(ctx, key)
	if err != nil {
		return nil, "", err
	}
	return data, contentType(filename), nil
}

func scanPage(row pgx.Row) (Page, error) {
	var page Page
	if err := row.Scan(&page.Key, &page.Title, &page.Body, &page.IsActive, &page.UpdatedAt); err != nil {
		return Page{}, err
	}
	return page, nil
}

func scanAsset(row pgx.Row) (Asset, error) {
	var asset Asset
	if err := row.Scan(&asset.ID, &asset.Kind, &asset.Filename, &asset.StorageKey, &asset.URL, &asset.CreatedAt); err != nil {
		return Asset{}, err
	}
	return asset, nil
}

func normalizeKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "docs", "announcement", "terms", "privacy":
		return value
	default:
		return ""
	}
}

func contentType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/png"
	}
}

func nullableString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
