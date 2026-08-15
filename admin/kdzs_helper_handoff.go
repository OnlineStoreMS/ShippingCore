package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"shippingcore/internal/pkg/authcontext"
	"shippingcore/internal/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	kdzsHelperHandoffTTL   = 30 * time.Minute
	kdzsHelperHandoffClean = 2 * time.Minute
	kdzsHelperHandoffMaxB  = 512 * 1024
)

type kdzsHelperHandoffSession struct {
	Token    string
	TenantID uint64
	UserID   uint64
	Payload  json.RawMessage
	ExpireAt time.Time
}

// KdzsHelperHandoffHandler 发货中心 → 快递助手扩展的短时任务会话（内存）。
type KdzsHelperHandoffHandler struct {
	mu       sync.Mutex
	sessions map[string]*kdzsHelperHandoffSession
}

func NewKdzsHelperHandoffHandler() *KdzsHelperHandoffHandler {
	h := &KdzsHelperHandoffHandler{
		sessions: make(map[string]*kdzsHelperHandoffSession),
	}
	go h.cleanupLoop()
	return h
}

func (h *KdzsHelperHandoffHandler) cleanupLoop() {
	ticker := time.NewTicker(kdzsHelperHandoffClean)
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

func (h *KdzsHelperHandoffHandler) get(token string) (*kdzsHelperHandoffSession, bool) {
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
	if s.Payload != nil {
		cp.Payload = append(json.RawMessage(nil), s.Payload...)
	}
	return &cp, true
}

/** 取出并删除：每个 token 只能被成功拉取一次，避免多机/多标签误用同一链接 */
func (h *KdzsHelperHandoffHandler) take(token string) (*kdzsHelperHandoffSession, bool) {
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
	delete(h.sessions, token)
	cp := *s
	if s.Payload != nil {
		cp.Payload = append(json.RawMessage(nil), s.Payload...)
	}
	return &cp, true
}

// CreateSession POST /api/v1/admin/kdzs/helper-handoff-sessions
func (h *KdzsHelperHandoffHandler) CreateSession(c *gin.Context) {
	var body struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Payload) == 0 {
		response.Fail(c, http.StatusBadRequest, "payload required")
		return
	}
	if len(body.Payload) > kdzsHelperHandoffMaxB {
		response.Fail(c, http.StatusBadRequest, "payload too large")
		return
	}
	// 粗校验 JSON 对象
	var probe map[string]any
	if err := json.Unmarshal(body.Payload, &probe); err != nil {
		response.Fail(c, http.StatusBadRequest, "payload must be json object")
		return
	}

	token := strings.ReplaceAll(uuid.NewString(), "-", "")
	s := &kdzsHelperHandoffSession{
		Token:    token,
		TenantID: authcontext.TenantID(c),
		UserID:   authcontext.UserID(c),
		Payload:  append(json.RawMessage(nil), body.Payload...),
		ExpireAt: time.Now().Add(kdzsHelperHandoffTTL),
	}
	h.mu.Lock()
	h.sessions[token] = s
	h.mu.Unlock()

	response.OK(c, gin.H{
		"token":    token,
		"expireAt": s.ExpireAt.UTC().Format(time.RFC3339),
	})
}

// MobileGet GET /api/v1/mobile/kdzs-helper-handoff/:token （扩展拉取，无需登录；一次性消费）
func (h *KdzsHelperHandoffHandler) MobileGet(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" || len(token) > 64 {
		response.Fail(c, http.StatusBadRequest, "invalid token")
		return
	}
	s, ok := h.take(token)
	if !ok {
		response.Fail(c, http.StatusNotFound, "任务已过期、已领取或不存在")
		return
	}
	var payload any
	if err := json.Unmarshal(s.Payload, &payload); err != nil {
		response.Fail(c, http.StatusInternalServerError, "payload corrupt")
		return
	}
	response.OK(c, gin.H{
		"token":    s.Token,
		"expireAt": s.ExpireAt.UTC().Format(time.RFC3339),
		"payload":  payload,
	})
}
