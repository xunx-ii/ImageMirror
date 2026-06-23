package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Local struct {
	root string
}

func NewLocal(root string) *Local {
	return &Local{root: root}
}

func (s *Local) SaveImage(ctx context.Context, userID string, imageID string, data []byte) (string, error) {
	now := time.Now()
	key := filepath.ToSlash(filepath.Join(
		fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", int(now.Month())),
		fmt.Sprintf("%02d", now.Day()),
		userID,
		imageID+".png",
	))
	return s.write(ctx, key, data)
}

func (s *Local) SaveReference(ctx context.Context, userID string, imageID string, index int, filename string, data []byte) (string, error) {
	now := time.Now()
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp":
	default:
		ext = ".png"
	}
	key := filepath.ToSlash(filepath.Join(
		fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", int(now.Month())),
		fmt.Sprintf("%02d", now.Day()),
		userID,
		imageID,
		fmt.Sprintf("reference-%d%s", index, ext),
	))
	return s.write(ctx, key, data)
}

func (s *Local) write(ctx context.Context, key string, data []byte) (string, error) {
	path, err := s.resolve(key)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return key, ctx.Err()
}

func (s *Local) ReadImage(_ context.Context, key string) ([]byte, error) {
	path, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (s *Local) DeleteImage(_ context.Context, key string) error {
	path, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Local) resolve(key string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(key))
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return "", errors.New("invalid storage key")
	}
	root, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, cleaned)
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return "", errors.New("storage key escapes root")
	}
	return abs, nil
}
