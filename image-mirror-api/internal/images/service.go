package images

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/linxunxi/image-mirror/internal/billing"
	"github.com/linxunxi/image-mirror/internal/config"
	"github.com/linxunxi/image-mirror/internal/openai"
	"github.com/linxunxi/image-mirror/internal/pricing"
	"github.com/linxunxi/image-mirror/internal/storage"
)

type Generation struct {
	ID             string     `json:"id"`
	UserID         string     `json:"userId"`
	APIKeyID       *string    `json:"apiKeyId,omitempty"`
	Model          string     `json:"model"`
	Prompt         string     `json:"prompt"`
	Size           string     `json:"size"`
	Quality        string     `json:"quality"`
	Status         string     `json:"status"`
	StorageKey     *string    `json:"-"`
	StorageURL     *string    `json:"storageUrl,omitempty"`
	ReferenceKeys  []string   `json:"-"`
	ReferenceCount int        `json:"referenceCount"`
	CreditsCost    int64      `json:"creditsCost"`
	ErrorMessage   *string    `json:"errorMessage,omitempty"`
	ExpiresAt      time.Time  `json:"expiresAt"`
	DeletedAt      *time.Time `json:"deletedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type CreateRequest struct {
	UserID        string
	APIKeyID      *string
	Model         string
	Prompt        string
	Size          string
	Quality       string
	ReferenceKeys []string
}

type Service struct {
	cfg     config.Config
	db      *pgxpool.Pool
	pricing *pricing.Service
	billing *billing.Service
	storage *storage.Local
	openai  *openai.Client
}

func NewService(cfg config.Config, db *pgxpool.Pool, pricingSvc *pricing.Service, billingSvc *billing.Service, storageSvc *storage.Local, openAIClient *openai.Client) *Service {
	return &Service{cfg: cfg, db: db, pricing: pricingSvc, billing: billingSvc, storage: storageSvc, openai: openAIClient}
}

const (
	maxReferenceImages     = 4
	maxReferenceImageBytes = 20 << 20
)

func (s *Service) CreatePending(ctx context.Context, req CreateRequest) (Generation, error) {
	req = normalize(req, s.cfg.DefaultImageModel)
	if err := validate(req); err != nil {
		return Generation{}, err
	}
	if req.ReferenceKeys == nil {
		req.ReferenceKeys = []string{}
	}
	referencePayload, err := json.Marshal(req.ReferenceKeys)
	if err != nil {
		return Generation{}, err
	}
	cost, err := s.pricing.GetCost(ctx, req.Model, req.Size, req.Quality)
	if err != nil {
		return Generation{}, err
	}
	var gen Generation
	row := s.db.QueryRow(ctx, `
		INSERT INTO image_generations(user_id, api_key_id, model, prompt, size, quality, reference_keys, credits_cost, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, now() + $9::interval)
		RETURNING id, user_id, api_key_id, model, prompt, size, quality, status, storage_key, storage_url, reference_keys, credits_cost, error_message, expires_at, deleted_at, created_at, updated_at
	`, req.UserID, req.APIKeyID, req.Model, req.Prompt, req.Size, req.Quality, string(referencePayload), cost, fmt.Sprintf("%f seconds", s.cfg.ImageRetention.Seconds()))
	if err := scanGeneration(row, &gen); err != nil {
		return Generation{}, err
	}
	if err := s.billing.Deduct(ctx, req.UserID, cost, "image generation", gen.ID); err != nil {
		_, _ = s.db.Exec(ctx, `DELETE FROM image_generations WHERE id=$1`, gen.ID)
		return Generation{}, err
	}
	return gen, nil
}

func (s *Service) SaveReferenceFiles(ctx context.Context, userID string, imageID string, headers []*multipart.FileHeader) ([]string, error) {
	if len(headers) == 0 {
		return []string{}, nil
	}
	if len(headers) > maxReferenceImages {
		return nil, fmt.Errorf("at most %d reference images are supported", maxReferenceImages)
	}
	keys := make([]string, 0, len(headers))
	cleanup := func() {
		for _, key := range keys {
			_ = s.storage.DeleteImage(ctx, key)
		}
	}
	for index, header := range headers {
		ext := strings.ToLower(filepath.Ext(header.Filename))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".webp":
		default:
			cleanup()
			return nil, errors.New("reference images must be png, jpg, jpeg, or webp")
		}
		if header.Size > maxReferenceImageBytes {
			cleanup()
			return nil, fmt.Errorf("reference image %s is larger than 20MB", header.Filename)
		}
		file, err := header.Open()
		if err != nil {
			cleanup()
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxReferenceImageBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			cleanup()
			return nil, readErr
		}
		if closeErr != nil {
			cleanup()
			return nil, closeErr
		}
		if len(data) == 0 {
			cleanup()
			return nil, errors.New("reference image is empty")
		}
		if len(data) > maxReferenceImageBytes {
			cleanup()
			return nil, fmt.Errorf("reference image %s is larger than 20MB", header.Filename)
		}
		key, err := s.storage.SaveReference(ctx, userID, imageID, index+1, header.Filename, data)
		if err != nil {
			cleanup()
			return nil, err
		}
		keys = append(keys, key)
	}
	payload, err := json.Marshal(keys)
	if err != nil {
		cleanup()
		return nil, err
	}
	updateTag, err := s.db.Exec(ctx, `
		UPDATE image_generations
		SET reference_keys=$2::jsonb, updated_at=now()
		WHERE id=$1 AND user_id=$3 AND status='PENDING'
	`, imageID, string(payload), userID)
	if err != nil {
		cleanup()
		return nil, err
	}
	if updateTag.RowsAffected() == 0 {
		cleanup()
		return nil, errors.New("image generation is not available for reference images")
	}
	return keys, nil
}

func (s *Service) Process(ctx context.Context, imageID string) error {
	gen, err := s.FindByID(ctx, imageID)
	if err != nil {
		return err
	}
	if gen.Status != "PENDING" {
		return nil
	}
	processingTag, err := s.db.Exec(ctx, `UPDATE image_generations SET status='PROCESSING', updated_at=now(), error_message=NULL WHERE id=$1 AND status='PENDING'`, imageID)
	if err != nil {
		return err
	}
	if processingTag.RowsAffected() == 0 {
		return nil
	}

	references := make([]openai.ReferenceImage, 0, len(gen.ReferenceKeys))
	for _, key := range gen.ReferenceKeys {
		data, err := s.storage.ReadImage(ctx, key)
		if err != nil {
			s.failGeneration(gen, err.Error(), "reference image read failed")
			return nil
		}
		references = append(references, openai.ReferenceImage{
			Filename: filepath.Base(key),
			Data:     data,
		})
	}

	bytes, err := s.openai.GenerateImage(ctx, openai.ImageRequest{
		Model:   gen.Model,
		Prompt:  gen.Prompt,
		Size:    gen.Size,
		Quality: gen.Quality,
		N:       1,
	}, references)
	if err != nil {
		s.failGeneration(gen, err.Error(), "image generation failed")
		return nil
	}

	key, err := s.storage.SaveImage(ctx, gen.UserID, gen.ID, bytes)
	if err != nil {
		s.failGeneration(gen, err.Error(), "image storage failed")
		return nil
	}
	storageURL := "/api/images/" + gen.ID + "/file"
	if gen.APIKeyID != nil {
		storageURL = "/v1/images/" + gen.ID + "/file"
	}
	completedTag, err := s.db.Exec(ctx, `
		UPDATE image_generations
		SET status='COMPLETED', storage_key=$2, storage_url=$3, updated_at=now()
		WHERE id=$1 AND status='PROCESSING'
	`, imageID, key, storageURL)
	if err != nil {
		return err
	}
	if completedTag.RowsAffected() == 0 {
		_ = s.storage.DeleteImage(ctx, key)
		s.deleteReferenceImages(ctx, gen.ReferenceKeys)
		return nil
	}
	s.deleteReferenceImages(ctx, gen.ReferenceKeys)
	return nil
}

func (s *Service) RecoverStaleProcessing(ctx context.Context, limit int32) (int, error) {
	timeout := s.cfg.OpenAITimeout + time.Minute
	if timeout <= time.Minute {
		timeout = 11 * time.Minute
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, api_key_id, model, prompt, size, quality, status, storage_key, storage_url, reference_keys, credits_cost, error_message, expires_at, deleted_at, created_at, updated_at
		FROM image_generations
		WHERE status='PROCESSING' AND updated_at < $1
		ORDER BY updated_at ASC
		LIMIT $2
	`, time.Now().Add(-timeout), limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	items := make([]Generation, 0)
	for rows.Next() {
		var gen Generation
		if err := scanGeneration(rows, &gen); err != nil {
			return 0, err
		}
		items = append(items, gen)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, gen := range items {
		s.failGeneration(gen, "image generation interrupted or timed out", "image generation interrupted")
	}
	return len(items), nil
}

func (s *Service) CancelPending(ctx context.Context, imageID string, reason string) error {
	gen, err := s.FindByID(ctx, imageID)
	if err != nil {
		return err
	}
	if gen.Status != "PENDING" {
		return nil
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE image_generations
		SET status='FAILED', error_message=$2, updated_at=now()
		WHERE id=$1 AND status='PENDING'
	`, imageID, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	s.deleteReferenceImages(ctx, gen.ReferenceKeys)
	return s.billing.Refund(ctx, gen.UserID, gen.CreditsCost, reason, gen.ID)
}

func (s *Service) FindForUser(ctx context.Context, userID string, imageID string) (Generation, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, user_id, api_key_id, model, prompt, size, quality, status, storage_key, storage_url, reference_keys, credits_cost, error_message, expires_at, deleted_at, created_at, updated_at
		FROM image_generations
		WHERE id=$1 AND user_id=$2
	`, imageID, userID)
	var gen Generation
	if err := scanGeneration(row, &gen); err != nil {
		return Generation{}, err
	}
	return gen, nil
}

func (s *Service) FindByID(ctx context.Context, imageID string) (Generation, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, user_id, api_key_id, model, prompt, size, quality, status, storage_key, storage_url, reference_keys, credits_cost, error_message, expires_at, deleted_at, created_at, updated_at
		FROM image_generations
		WHERE id=$1
	`, imageID)
	var gen Generation
	if err := scanGeneration(row, &gen); err != nil {
		return Generation{}, err
	}
	return gen, nil
}

func (s *Service) ListForUser(ctx context.Context, userID string, limit int32, offset int32) ([]Generation, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, api_key_id, model, prompt, size, quality, status, storage_key, storage_url, reference_keys, credits_cost, error_message, expires_at, deleted_at, created_at, updated_at
		FROM image_generations
		WHERE user_id=$1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Generation, 0)
	for rows.Next() {
		var gen Generation
		if err := scanGeneration(rows, &gen); err != nil {
			return nil, err
		}
		out = append(out, gen)
	}
	return out, rows.Err()
}

func (s *Service) ReadFile(ctx context.Context, userID string, imageID string) ([]byte, Generation, error) {
	gen, err := s.FindForUser(ctx, userID, imageID)
	if err != nil {
		return nil, Generation{}, err
	}
	if gen.Status != "COMPLETED" || gen.StorageKey == nil || gen.DeletedAt != nil {
		return nil, Generation{}, errors.New("image file is not available")
	}
	bytes, err := s.storage.ReadImage(ctx, *gen.StorageKey)
	return bytes, gen, err
}

func (s *Service) DeleteForUser(ctx context.Context, userID string, imageID string) error {
	gen, err := s.FindForUser(ctx, userID, imageID)
	if err != nil {
		return err
	}
	if gen.DeletedAt != nil {
		return nil
	}
	if gen.StorageKey != nil {
		_ = s.storage.DeleteImage(ctx, *gen.StorageKey)
	}
	s.deleteReferenceImages(ctx, gen.ReferenceKeys)
	_, err = s.db.Exec(ctx, `
		UPDATE image_generations
		SET status='EXPIRED', deleted_at=now(), updated_at=now()
		WHERE id=$1 AND user_id=$2
	`, imageID, userID)
	return err
}

func (s *Service) DeleteManyForUser(ctx context.Context, userID string, imageIDs []string) (int, error) {
	count := 0
	for _, imageID := range imageIDs {
		imageID = strings.TrimSpace(imageID)
		if imageID == "" {
			continue
		}
		if err := s.DeleteForUser(ctx, userID, imageID); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *Service) ZipFilesForUser(ctx context.Context, userID string, imageIDs []string) ([]byte, error) {
	buffer := &bytes.Buffer{}
	writer := zip.NewWriter(buffer)
	added := 0
	for _, imageID := range imageIDs {
		imageID = strings.TrimSpace(imageID)
		if imageID == "" {
			continue
		}
		data, gen, err := s.ReadFile(ctx, userID, imageID)
		if err != nil {
			return nil, err
		}
		file, err := writer.Create(fmt.Sprintf("%s.png", gen.ID))
		if err != nil {
			return nil, err
		}
		if _, err := file.Write(data); err != nil {
			return nil, err
		}
		added++
	}
	if added == 0 {
		_ = writer.Close()
		return nil, errors.New("no image files selected")
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (s *Service) ExpireOld(ctx context.Context, limit int32) (int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, storage_key, reference_keys
		FROM image_generations
		WHERE status='COMPLETED' AND expires_at < now() AND deleted_at IS NULL
		ORDER BY expires_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type item struct {
		id   string
		key  *string
		refs []string
	}
	items := make([]item, 0)
	for rows.Next() {
		var it item
		var refs []byte
		if err := rows.Scan(&it.id, &it.key, &refs); err != nil {
			return 0, err
		}
		if len(refs) > 0 {
			if err := json.Unmarshal(refs, &it.refs); err != nil {
				return 0, err
			}
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, it := range items {
		if it.key != nil {
			_ = s.storage.DeleteImage(ctx, *it.key)
		}
		s.deleteReferenceImages(ctx, it.refs)
		_, _ = s.db.Exec(ctx, `UPDATE image_generations SET status='EXPIRED', deleted_at=now(), updated_at=now() WHERE id=$1`, it.id)
	}
	return len(items), nil
}

func normalize(req CreateRequest, defaultModel string) CreateRequest {
	if req.Model == "" {
		req.Model = defaultModel
	}
	if req.Size == "" {
		req.Size = "1024x1024"
	}
	if req.Quality == "" {
		req.Quality = "medium"
	}
	return req
}

func validate(req CreateRequest) error {
	if req.UserID == "" || req.Prompt == "" {
		return errors.New("user and prompt are required")
	}
	if req.Model != "gpt-image-2" {
		return errors.New("only gpt-image-2 is enabled")
	}
	if _, _, err := pricing.ParseCustomSize(req.Size); err != nil {
		return err
	}
	switch req.Quality {
	case "low", "medium", "high", "auto":
	default:
		return errors.New("unsupported quality")
	}
	return nil
}

func scanGeneration(row pgx.Row, gen *Generation) error {
	var referenceKeys []byte
	if err := row.Scan(&gen.ID, &gen.UserID, &gen.APIKeyID, &gen.Model, &gen.Prompt, &gen.Size, &gen.Quality, &gen.Status, &gen.StorageKey, &gen.StorageURL, &referenceKeys, &gen.CreditsCost, &gen.ErrorMessage, &gen.ExpiresAt, &gen.DeletedAt, &gen.CreatedAt, &gen.UpdatedAt); err != nil {
		return err
	}
	if len(referenceKeys) > 0 {
		if err := json.Unmarshal(referenceKeys, &gen.ReferenceKeys); err != nil {
			return err
		}
	}
	if gen.ReferenceKeys == nil {
		gen.ReferenceKeys = []string{}
	}
	gen.ReferenceCount = len(gen.ReferenceKeys)
	return nil
}

func (s *Service) deleteReferenceImages(ctx context.Context, keys []string) {
	for _, key := range keys {
		_ = s.storage.DeleteImage(ctx, key)
	}
}

func (s *Service) failGeneration(gen Generation, message string, refundDescription string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tag, _ := s.db.Exec(ctx, `
		UPDATE image_generations
		SET status='FAILED', error_message=$2, updated_at=now()
		WHERE id=$1 AND status IN ('PENDING', 'PROCESSING')
	`, gen.ID, message)
	if tag.RowsAffected() > 0 {
		_ = s.billing.Refund(ctx, gen.UserID, gen.CreditsCost, refundDescription, gen.ID)
		s.deleteReferenceImages(ctx, gen.ReferenceKeys)
	}
}
