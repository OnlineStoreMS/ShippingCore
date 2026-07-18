package admin

import (
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"shippingcore/internal/pkg/authcontext"
	"shippingcore/internal/pkg/response"
	"shippingcore/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	photoSessionTTL   = 10 * time.Minute
	photoSessionClean = 2 * time.Minute
)

type photoSession struct {
	Token    string
	Subdir   string
	TenantID uint64
	URL      string
	Status   string // pending | done
	ExpireAt time.Time
}

type PhotoUploadHandler struct {
	store    storage.Storage
	mu       sync.Mutex
	sessions map[string]*photoSession
}

func NewPhotoUploadHandler(store storage.Storage) *PhotoUploadHandler {
	h := &PhotoUploadHandler{
		store:    store,
		sessions: make(map[string]*photoSession),
	}
	go h.cleanupLoop()
	return h
}

func (h *PhotoUploadHandler) cleanupLoop() {
	ticker := time.NewTicker(photoSessionClean)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		h.mu.Lock()
		for token, s := range h.sessions {
			if now.After(s.ExpireAt) {
				delete(h.sessions, token)
			}
		}
		h.mu.Unlock()
	}
}

func (h *PhotoUploadHandler) get(token string) (*photoSession, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[token]
	if !ok {
		return nil, false
	}
	if time.Now().After(s.ExpireAt) {
		delete(h.sessions, token)
		return nil, false
	}
	cp := *s
	return &cp, true
}

func (h *PhotoUploadHandler) CreateSession(c *gin.Context) {
	var body struct {
		Subdir string `json:"subdir"`
	}
	_ = c.ShouldBindJSON(&body)
	subdir := strings.Trim(body.Subdir, "/")
	if subdir == "" {
		subdir = "products"
	}
	token := strings.ReplaceAll(uuid.NewString(), "-", "")
	s := &photoSession{
		Token:    token,
		Subdir:   subdir,
		TenantID: authcontext.TenantID(c),
		Status:   "pending",
		ExpireAt: time.Now().Add(photoSessionTTL),
	}
	h.mu.Lock()
	h.sessions[token] = s
	h.mu.Unlock()
	response.OK(c, gin.H{
		"token":    token,
		"expireAt": s.ExpireAt.UTC().Format(time.RFC3339),
		"status":   s.Status,
	})
}

func (h *PhotoUploadHandler) GetSession(c *gin.Context) {
	token := c.Param("token")
	s, ok := h.get(token)
	if !ok {
		response.Fail(c, http.StatusNotFound, "扫码会话已过期或不存在")
		return
	}
	response.OK(c, gin.H{
		"token":    s.Token,
		"status":   s.Status,
		"url":      s.URL,
		"expireAt": s.ExpireAt.UTC().Format(time.RFC3339),
	})
}

func (h *PhotoUploadHandler) MobileGet(c *gin.Context) {
	h.GetSession(c)
}

func (h *PhotoUploadHandler) MobileUpload(c *gin.Context) {
	token := c.Param("token")
	h.mu.Lock()
	s, ok := h.sessions[token]
	if !ok || time.Now().After(s.ExpireAt) {
		if ok {
			delete(h.sessions, token)
		}
		h.mu.Unlock()
		response.Fail(c, http.StatusNotFound, "扫码会话已过期或不存在")
		return
	}
	if s.Status == "done" && s.URL != "" {
		h.mu.Unlock()
		response.OK(c, gin.H{"url": s.URL, "status": "done"})
		return
	}
	subdir := s.Subdir
	h.mu.Unlock()

	file, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "file required")
		return
	}
	if file.Size > maxImageSize {
		response.Fail(c, http.StatusBadRequest, "image too large (max 10MB)")
		return
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
	default:
		ct := file.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "image/") {
			response.Fail(c, http.StatusBadRequest, "unsupported image type")
			return
		}
	}

	url, err := h.store.Upload(file, subdir)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.mu.Lock()
	if cur, ok := h.sessions[token]; ok && time.Now().Before(cur.ExpireAt) {
		cur.URL = url
		cur.Status = "done"
		cur.ExpireAt = time.Now().Add(2 * time.Minute)
	}
	h.mu.Unlock()

	response.OK(c, gin.H{"url": url, "status": "done"})
}
