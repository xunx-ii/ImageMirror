package images

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	imagedraw "image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"mime/multipart"
	"os"
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
	"github.com/linxunxi/image-mirror/internal/systemconfig"
	"github.com/linxunxi/image-mirror/internal/usage"
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

type FileRef struct {
	StorageKey    *string
	ReferenceKeys []string
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
	configs *systemconfig.Service
	usage   *usage.Service
}

func NewService(cfg config.Config, db *pgxpool.Pool, pricingSvc *pricing.Service, billingSvc *billing.Service, storageSvc *storage.Local, openAIClient *openai.Client, configSvc *systemconfig.Service) *Service {
	return &Service{cfg: cfg, db: db, pricing: pricingSvc, billing: billingSvc, storage: storageSvc, openai: openAIClient, configs: configSvc}
}

func (s *Service) SetUsageService(usageSvc *usage.Service) {
	s.usage = usageSvc
}

const (
	maxReferenceImages     = 4
	maxReferenceImageBytes = 20 << 20
	DefaultPreviewMaxEdge  = 512
)

var previewMaxEdges = []int{256, 512, 768}

func (s *Service) CreatePending(ctx context.Context, req CreateRequest) (Generation, error) {
	req = normalize(req, s.cfg.DefaultImageModel)
	if err := validate(req); err != nil {
		return Generation{}, err
	}
	if err := s.validateResolutionLimit(ctx, req.Size); err != nil {
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
	if s.usage != nil {
		_ = s.usage.MarkProcessingByImageID(ctx, gen.ID)
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
		_ = s.storage.DeleteImage(ctx, key)
		return err
	}
	if completedTag.RowsAffected() == 0 {
		_ = s.storage.DeleteImage(ctx, key)
		s.deleteReferenceImages(ctx, gen.ReferenceKeys)
		return nil
	}
	if s.usage != nil {
		statusCode := 200
		_ = s.usage.CompleteByImageID(ctx, usage.CompleteInput{
			ImageGenerationID: gen.ID,
			Status:            "COMPLETED",
			Success:           true,
			StatusCode:        &statusCode,
		})
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
	if s.usage != nil {
		statusCode := 500
		_ = s.usage.CompleteByImageID(ctx, usage.CompleteInput{
			ImageGenerationID: gen.ID,
			Status:            "FAILED",
			Success:           false,
			StatusCode:        &statusCode,
			ErrorMessage:      &reason,
		})
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

func (s *Service) ReadPreview(ctx context.Context, userID string, imageID string, maxEdge int) ([]byte, Generation, error) {
	maxEdge = normalizePreviewMaxEdge(maxEdge)
	gen, err := s.FindForUser(ctx, userID, imageID)
	if err != nil {
		return nil, Generation{}, err
	}
	if gen.Status != "COMPLETED" || gen.StorageKey == nil || gen.DeletedAt != nil {
		return nil, Generation{}, errors.New("image preview is not available")
	}
	if data, _, err := s.storage.ReadImagePreview(ctx, *gen.StorageKey, maxEdge); err == nil {
		return data, gen, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, Generation{}, err
	}

	original, err := s.storage.ReadImage(ctx, *gen.StorageKey)
	if err != nil {
		return nil, Generation{}, err
	}
	preview, err := createPreview(original, maxEdge)
	if err != nil {
		return nil, Generation{}, err
	}
	if _, err := s.storage.SaveImagePreview(ctx, *gen.StorageKey, maxEdge, preview); err != nil {
		if cached, _, readErr := s.storage.ReadImagePreview(ctx, *gen.StorageKey, maxEdge); readErr == nil {
			return cached, gen, nil
		}
		return nil, Generation{}, err
	}
	return preview, gen, nil
}

func (s *Service) DeleteForUser(ctx context.Context, userID string, imageID string) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row := tx.QueryRow(ctx, `
		SELECT id, user_id, api_key_id, model, prompt, size, quality, status, storage_key, storage_url, reference_keys, credits_cost, error_message, expires_at, deleted_at, created_at, updated_at
		FROM image_generations
		WHERE id=$1 AND user_id=$2
		FOR UPDATE
	`, imageID, userID)
	var gen Generation
	if err := scanGeneration(row, &gen); err != nil {
		return err
	}
	if gen.DeletedAt != nil {
		return nil
	}
	row = tx.QueryRow(ctx, `
		UPDATE image_generations
		SET status='EXPIRED', deleted_at=now(), updated_at=now()
		WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL
		RETURNING id, user_id, api_key_id, model, prompt, size, quality, status, storage_key, storage_url, reference_keys, credits_cost, error_message, expires_at, deleted_at, created_at, updated_at
	`, imageID, userID)
	var deleted Generation
	if err := scanGeneration(row, &deleted); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if gen.Status == "PENDING" {
		if err := s.billing.RefundWithTx(ctx, tx, gen.UserID, gen.CreditsCost, "image deleted before processing", gen.ID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if gen.StorageKey != nil {
		s.deleteImageFiles(ctx, *gen.StorageKey)
	}
	s.deleteReferenceImages(ctx, gen.ReferenceKeys)
	return nil
}

func (s *Service) DeleteManyForUser(ctx context.Context, userID string, imageIDs []string) (int, error) {
	count := 0
	seen := make(map[string]struct{}, len(imageIDs))
	for _, imageID := range imageIDs {
		imageID = strings.TrimSpace(imageID)
		if imageID == "" {
			continue
		}
		if _, ok := seen[imageID]; ok {
			continue
		}
		seen[imageID] = struct{}{}
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
	seen := make(map[string]struct{}, len(imageIDs))
	for _, imageID := range imageIDs {
		imageID = strings.TrimSpace(imageID)
		if imageID == "" {
			continue
		}
		if _, ok := seen[imageID]; ok {
			continue
		}
		seen[imageID] = struct{}{}
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

func (s *Service) DeleteFilesForUser(ctx context.Context, userID string) error {
	refs, err := s.FileRefsForUser(ctx, userID)
	if err != nil {
		return err
	}
	return s.DeleteFileRefs(ctx, refs)
}

func (s *Service) FileRefsForUser(ctx context.Context, userID string) ([]FileRef, error) {
	rows, err := s.db.Query(ctx, `
		SELECT storage_key, reference_keys
		FROM image_generations
		WHERE user_id=$1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]FileRef, 0)
	for rows.Next() {
		var item FileRef
		var refs []byte
		if err := rows.Scan(&item.StorageKey, &refs); err != nil {
			return nil, err
		}
		if len(refs) > 0 {
			if err := json.Unmarshal(refs, &item.ReferenceKeys); err != nil {
				return nil, err
			}
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) DeleteFileRefs(ctx context.Context, refs []FileRef) error {
	for _, item := range refs {
		if item.StorageKey != nil {
			s.deleteImageFiles(ctx, *item.StorageKey)
		}
		s.deleteReferenceImages(ctx, item.ReferenceKeys)
	}
	return nil
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
	expired := 0
	for _, it := range items {
		tag, err := s.db.Exec(ctx, `
			UPDATE image_generations
			SET status='EXPIRED', deleted_at=now(), updated_at=now()
			WHERE id=$1 AND status='COMPLETED' AND deleted_at IS NULL
		`, it.id)
		if err != nil {
			return expired, err
		}
		if tag.RowsAffected() == 0 {
			continue
		}
		if it.key != nil {
			s.deleteImageFiles(ctx, *it.key)
		}
		s.deleteReferenceImages(ctx, it.refs)
		expired++
	}
	return expired, nil
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

func (s *Service) validateResolutionLimit(ctx context.Context, size string) error {
	if s.configs == nil {
		return nil
	}
	maxBucket, err := s.configs.MaxResolutionBucket(ctx)
	if err != nil {
		return err
	}
	if maxBucket != "2k" {
		return nil
	}
	bucket, err := pricing.ResolutionBucket(size)
	if err != nil {
		return err
	}
	if bucket == "4k" {
		return errors.New("4K image generation is disabled")
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

func normalizePreviewMaxEdge(maxEdge int) int {
	if maxEdge <= 0 {
		return DefaultPreviewMaxEdge
	}
	for _, allowed := range previewMaxEdges {
		if maxEdge <= allowed {
			return allowed
		}
	}
	return previewMaxEdges[len(previewMaxEdges)-1]
}

func createPreview(data []byte, maxEdge int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, errors.New("image has invalid dimensions")
	}
	scale := math.Min(1, float64(maxEdge)/float64(max(width, height)))
	nextWidth := max(1, int(math.Round(float64(width)*scale)))
	nextHeight := max(1, int(math.Round(float64(height)*scale)))
	dst := resizeToOpaqueRGBA(src, nextWidth, nextHeight)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 64}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func resizeToOpaqueRGBA(src image.Image, width int, height int) *image.RGBA {
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()
	if width == srcWidth && height == srcHeight {
		imagedraw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, imagedraw.Src)
		imagedraw.Draw(dst, dst.Bounds(), src, bounds.Min, imagedraw.Over)
		return dst
	}

	xRatio := float64(srcWidth) / float64(width)
	yRatio := float64(srcHeight) / float64(height)
	for y := 0; y < height; y++ {
		srcY := (float64(y)+0.5)*yRatio - 0.5
		y0 := int(math.Floor(srcY))
		if y0 < 0 {
			y0 = 0
		}
		y1 := min(y0+1, srcHeight-1)
		wy := srcY - float64(y0)
		if wy < 0 {
			wy = 0
		}
		for x := 0; x < width; x++ {
			srcX := (float64(x)+0.5)*xRatio - 0.5
			x0 := int(math.Floor(srcX))
			if x0 < 0 {
				x0 = 0
			}
			x1 := min(x0+1, srcWidth-1)
			wx := srcX - float64(x0)
			if wx < 0 {
				wx = 0
			}
			dst.SetRGBA(x, y, bilinearOpaque(src, bounds.Min.X+x0, bounds.Min.Y+y0, bounds.Min.X+x1, bounds.Min.Y+y1, wx, wy))
		}
	}
	return dst
}

func bilinearOpaque(src image.Image, x0 int, y0 int, x1 int, y1 int, wx float64, wy float64) color.RGBA {
	c00 := compositeOnWhite(src.At(x0, y0))
	c10 := compositeOnWhite(src.At(x1, y0))
	c01 := compositeOnWhite(src.At(x0, y1))
	c11 := compositeOnWhite(src.At(x1, y1))
	return color.RGBA{
		R: lerpByte(lerpByteFloat(c00.R, c10.R, wx), lerpByteFloat(c01.R, c11.R, wx), wy),
		G: lerpByte(lerpByteFloat(c00.G, c10.G, wx), lerpByteFloat(c01.G, c11.G, wx), wy),
		B: lerpByte(lerpByteFloat(c00.B, c10.B, wx), lerpByteFloat(c01.B, c11.B, wx), wy),
		A: 255,
	}
}

func compositeOnWhite(c color.Color) color.RGBA {
	r, g, b, a := c.RGBA()
	alpha := float64(a) / 65535
	return color.RGBA{
		R: uint8(math.Round(float64(r)/257 + 255*(1-alpha))),
		G: uint8(math.Round(float64(g)/257 + 255*(1-alpha))),
		B: uint8(math.Round(float64(b)/257 + 255*(1-alpha))),
		A: 255,
	}
}

func lerpByteFloat(a uint8, b uint8, t float64) float64 {
	return float64(a)*(1-t) + float64(b)*t
}

func lerpByte(a float64, b float64, t float64) uint8 {
	return uint8(math.Round(a*(1-t) + b*t))
}

func (s *Service) deleteImageFiles(ctx context.Context, key string) {
	_ = s.storage.DeleteImage(ctx, key)
	for _, maxEdge := range previewMaxEdges {
		_ = s.storage.DeleteImagePreview(ctx, key, maxEdge)
	}
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
		if s.usage != nil {
			statusCode := 502
			_ = s.usage.CompleteByImageID(ctx, usage.CompleteInput{
				ImageGenerationID: gen.ID,
				Status:            "FAILED",
				Success:           false,
				StatusCode:        &statusCode,
				ErrorMessage:      &message,
			})
		}
		_ = s.billing.Refund(ctx, gen.UserID, gen.CreditsCost, refundDescription, gen.ID)
		s.deleteReferenceImages(ctx, gen.ReferenceKeys)
	}
}
