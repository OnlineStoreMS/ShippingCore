package storage

import (
	"net/url"
	"strings"

	"shippingcore/internal/config"
)

// PublicURLResolver 将库内已存 URL 解析为当前 public_base_url（换 IP/域名后仍可用）
type PublicURLResolver struct {
	baseURL   string
	keyPrefix string
}

func NewPublicURLResolver(cfg *config.StorageConfig) *PublicURLResolver {
	base := strings.TrimRight(cfg.PublicBaseURL, "/")
	if base == "" {
		m := cfg.MinIO
		scheme := "http"
		if m.UseSSL {
			scheme = "https"
		}
		if m.Endpoint != "" && m.Bucket != "" {
			base = scheme + "://" + m.Endpoint + "/" + m.Bucket
		}
	}
	prefix := strings.Trim(cfg.Prefix, "/")
	if prefix == "" {
		prefix = strings.Trim(cfg.MinIO.Prefix, "/")
	}
	if prefix == "" {
		prefix = "attachments"
	}
	return &PublicURLResolver{
		baseURL:   base,
		keyPrefix: prefix,
	}
}

func (r *PublicURLResolver) Resolve(stored string) string {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return stored
	}
	if !strings.HasPrefix(stored, "http://") && !strings.HasPrefix(stored, "https://") {
		return stored
	}
	key := r.extractObjectKey(stored)
	if key == "" {
		return stored
	}
	return r.baseURL + "/" + key
}

func (r *PublicURLResolver) extractObjectKey(stored string) string {
	u, err := url.Parse(stored)
	if err != nil {
		return ""
	}
	path := strings.TrimPrefix(u.Path, "/")
	marker := r.keyPrefix + "/"
	if idx := strings.Index(path, marker); idx >= 0 {
		return path[idx:]
	}
	return ""
}
