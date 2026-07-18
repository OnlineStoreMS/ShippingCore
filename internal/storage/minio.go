package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"shippingcore/internal/config"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOStorage struct {
	client     *minio.Client
	bucket     string
	baseURL    string
	rootPrefix string
}

func NewMinIO(cfg *config.StorageConfig) (*MinIOStorage, error) {
	m := cfg.MinIO
	if m.Endpoint == "" {
		return nil, fmt.Errorf("minio endpoint required")
	}
	if m.AccessKey == "" || m.SecretKey == "" {
		return nil, fmt.Errorf("minio access_key and secret_key required")
	}
	if m.Bucket == "" {
		return nil, fmt.Errorf("minio bucket required")
	}

	client, err := minio.New(m.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(m.AccessKey, m.SecretKey, ""),
		Secure: m.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}

	ctx := context.Background()
	if _, err := client.ListBuckets(ctx); err != nil {
		return nil, fmt.Errorf("minio connect %s: %w", m.Endpoint, err)
	}

	exists, err := client.BucketExists(ctx, m.Bucket)
	if err != nil {
		return nil, fmt.Errorf("minio bucket check: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, m.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("minio make bucket: %w", err)
		}
	}

	if m.PublicRead {
		if err := setBucketPublicRead(ctx, client, m.Bucket); err != nil {
			return nil, fmt.Errorf("minio bucket policy: %w", err)
		}
	}

	baseURL := strings.TrimRight(cfg.PublicBaseURL, "/")
	if baseURL == "" {
		scheme := "http"
		if m.UseSSL {
			scheme = "https"
		}
		baseURL = fmt.Sprintf("%s://%s/%s", scheme, m.Endpoint, m.Bucket)
	}

	prefix := strings.Trim(m.Prefix, "/")
	if prefix == "" {
		prefix = "attachments"
	}

	return &MinIOStorage{
		client:     client,
		bucket:     m.Bucket,
		baseURL:    baseURL,
		rootPrefix: prefix,
	}, nil
}

func setBucketPublicRead(ctx context.Context, client *minio.Client, bucket string) error {
	policy := map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{{
			"Effect":    "Allow",
			"Principal": map[string]any{"AWS": []string{"*"}},
			"Action":    []string{"s3:GetObject"},
			"Resource":  []string{fmt.Sprintf("arn:aws:s3:::%s/*", bucket)},
		}},
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	return client.SetBucketPolicy(ctx, bucket, string(raw))
}

func (s *MinIOStorage) Upload(file *multipart.FileHeader, subdir string) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	ext := filepath.Ext(file.Filename)
	name := fmt.Sprintf("%s_%s%s", time.Now().Format("20060102150405"), uuid.New().String()[:8], ext)
	subdir = strings.Trim(subdir, "/")
	objectKey := s.rootPrefix
	if subdir != "" {
		objectKey += "/" + subdir
	}
	objectKey += "/" + safeFilename(name)

	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	_, err = s.client.PutObject(context.Background(), s.bucket, objectKey, src, file.Size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", err
	}
	return s.baseURL + "/" + objectKey, nil
}

func safeFilename(name string) string {
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, name)
	if name == "" || name == "." {
		return "file.bin"
	}
	return name
}
