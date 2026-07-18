package storage

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"shippingcore/internal/config"

	"github.com/google/uuid"
)

type Storage interface {
	Upload(file *multipart.FileHeader, subdir string) (string, error)
}

type LocalStorage struct {
	baseDir string
	baseURL string
	prefix  string
}

func NewLocal(cfg *config.StorageConfig) (*LocalStorage, error) {
	base := filepath.Join(cfg.LocalPath, cfg.Prefix)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, err
	}
	return &LocalStorage{
		baseDir: base,
		baseURL: strings.TrimRight(cfg.PublicBaseURL, "/"),
		prefix:  cfg.Prefix,
	}, nil
}

func (s *LocalStorage) Upload(file *multipart.FileHeader, subdir string) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	ext := filepath.Ext(file.Filename)
	name := fmt.Sprintf("%s_%s%s", time.Now().Format("20060102150405"), uuid.New().String()[:8], ext)
	rel := filepath.Join(subdir, name)
	destPath := filepath.Join(s.baseDir, rel)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", err
	}
	dst, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	urlPath := strings.ReplaceAll(filepath.Join(s.prefix, rel), "\\", "/")
	return s.baseURL + "/" + urlPath, nil
}

func New(cfg *config.StorageConfig) (Storage, error) {
	switch cfg.Driver {
	case "minio":
		return NewMinIO(cfg)
	case "local", "":
		return NewLocal(cfg)
	default:
		return nil, fmt.Errorf("unsupported storage driver: %s", cfg.Driver)
	}
}
