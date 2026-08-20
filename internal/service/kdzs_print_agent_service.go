package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"shippingcore/internal/model"
	"shippingcore/internal/repo"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	kdzsPairTTL          = 10 * time.Minute
	kdzsDeviceOnlineSkew = 45 * time.Second
)

var (
	ErrPairCodeInvalid = errors.New("配对码无效或已过期")
	ErrDeviceAuth      = errors.New("设备鉴权失败")
	ErrDeviceOffline   = errors.New("打单插件不在线")
	ErrDeviceNotFound  = errors.New("设备不存在")
	ErrNoTask          = errors.New("暂无待领任务")
)

type KdzsPrintAgentService struct {
	repos    *repo.Repos
	tenantID uint64
}

func NewKdzsPrintAgentService(repos *repo.Repos) *KdzsPrintAgentService {
	return &KdzsPrintAgentService{repos: repos}
}

func (s *KdzsPrintAgentService) ForTenant(tenantID uint64) *KdzsPrintAgentService {
	cp := *s
	cp.tenantID = tenantID
	return &cp
}

func (s *KdzsPrintAgentService) db() *gorm.DB {
	return s.repos.ForTenant(s.tenantID)
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func randomPairCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

type KdzsPrintDeviceDTO struct {
	ID         uint64  `json:"id"`
	DeviceKey  string  `json:"deviceKey"`
	Name       string  `json:"name"`
	Online     bool    `json:"online"`
	LastSeenAt *string `json:"lastSeenAt,omitempty"`
	Enabled    bool    `json:"enabled"`
	CreatedAt  string  `json:"createdAt"`
}

func deviceOnline(last *time.Time) bool {
	if last == nil {
		return false
	}
	return time.Since(*last) <= kdzsDeviceOnlineSkew
}

func toDeviceDTO(d model.KdzsPrintDevice) KdzsPrintDeviceDTO {
	out := KdzsPrintDeviceDTO{
		ID:        d.ID,
		DeviceKey: d.DeviceKey,
		Name:      d.Name,
		Online:    deviceOnline(d.LastSeenAt),
		Enabled:   d.Enabled,
		CreatedAt: d.CreatedAt.Format(time.RFC3339),
	}
	if d.LastSeenAt != nil {
		s := d.LastSeenAt.Format(time.RFC3339)
		out.LastSeenAt = &s
	}
	return out
}

// CreatePairSession 手机端创建配对码。
func (s *KdzsPrintAgentService) CreatePairSession(userID uint64) (pairCode string, expireAt time.Time, err error) {
	if userID == 0 {
		return "", time.Time{}, ErrBadRequest
	}
	expireAt = time.Now().Add(kdzsPairTTL)
	var code string
	for i := 0; i < 8; i++ {
		code, err = randomPairCode()
		if err != nil {
			return "", time.Time{}, err
		}
		row := model.KdzsPrintPairSession{
			TenantID: s.tenantID,
			UserID:   userID,
			PairCode: code,
			ExpireAt: expireAt,
		}
		if err = s.repos.DB.Create(&row).Error; err == nil {
			return code, expireAt, nil
		}
		// unique pair_code 冲突则重试
	}
	return "", time.Time{}, fmt.Errorf("生成配对码失败，请重试")
}

type CompletePairResult struct {
	DeviceID    uint64 `json:"deviceId"`
	DeviceKey   string `json:"deviceKey"`
	DeviceSecret string `json:"deviceSecret"`
	Name        string `json:"name"`
	TenantID    uint64 `json:"tenantId"`
}

// CompletePair 扩展输入配对码完成绑定（公开接口，凭配对码）。
func (s *KdzsPrintAgentService) CompletePair(pairCode, deviceName string) (*CompletePairResult, error) {
	code := strings.TrimSpace(pairCode)
	if len(code) < 4 {
		return nil, ErrPairCodeInvalid
	}
	name := strings.TrimSpace(deviceName)
	if name == "" {
		name = "打单电脑"
	}
	if len(name) > 64 {
		name = name[:64]
	}

	var sess model.KdzsPrintPairSession
	err := s.repos.DB.Where("pair_code = ? AND consumed = false", code).First(&sess).Error
	if err != nil {
		return nil, ErrPairCodeInvalid
	}
	if time.Now().After(sess.ExpireAt) {
		return nil, ErrPairCodeInvalid
	}

	deviceKey, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	secret, err := randomHex(24)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	dev := model.KdzsPrintDevice{
		TenantID:   sess.TenantID,
		UserID:     sess.UserID,
		DeviceKey:  deviceKey,
		SecretHash: hashSecret(secret),
		Name:       name,
		LastSeenAt: &now,
		Enabled:    true,
	}

	err = s.repos.DB.Transaction(func(tx *gorm.DB) error {
		var locked model.KdzsPrintPairSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND consumed = false", sess.ID).First(&locked).Error; err != nil {
			return ErrPairCodeInvalid
		}
		if time.Now().After(locked.ExpireAt) {
			return ErrPairCodeInvalid
		}
		if err := tx.Create(&dev).Error; err != nil {
			return err
		}
		return tx.Model(&locked).Updates(map[string]any{
			"consumed":  true,
			"device_id": dev.ID,
		}).Error
	})
	if err != nil {
		if errors.Is(err, ErrPairCodeInvalid) {
			return nil, err
		}
		return nil, err
	}

	return &CompletePairResult{
		DeviceID:     dev.ID,
		DeviceKey:    deviceKey,
		DeviceSecret: secret,
		Name:         name,
		TenantID:     sess.TenantID,
	}, nil
}

func (s *KdzsPrintAgentService) AuthenticateDevice(deviceKey, secret string) (*model.KdzsPrintDevice, error) {
	key := strings.TrimSpace(deviceKey)
	sec := strings.TrimSpace(secret)
	if key == "" || sec == "" {
		return nil, ErrDeviceAuth
	}
	var d model.KdzsPrintDevice
	if err := s.repos.DB.Where("device_key = ? AND enabled = true", key).First(&d).Error; err != nil {
		return nil, ErrDeviceAuth
	}
	if d.SecretHash != hashSecret(sec) {
		return nil, ErrDeviceAuth
	}
	return &d, nil
}

func (s *KdzsPrintAgentService) Heartbeat(deviceKey, secret string) (*KdzsPrintDeviceDTO, error) {
	d, err := s.AuthenticateDevice(deviceKey, secret)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if err := s.repos.DB.Model(d).Update("last_seen_at", now).Error; err != nil {
		return nil, err
	}
	d.LastSeenAt = &now
	dto := toDeviceDTO(*d)
	return &dto, nil
}

func (s *KdzsPrintAgentService) ListDevices() ([]KdzsPrintDeviceDTO, error) {
	var rows []model.KdzsPrintDevice
	if err := s.db().Where("enabled = true").Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]KdzsPrintDeviceDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDeviceDTO(r))
	}
	return out, nil
}

func (s *KdzsPrintAgentService) RenameDevice(id uint64, name string) (*KdzsPrintDeviceDTO, error) {
	name = strings.TrimSpace(name)
	if id == 0 || name == "" {
		return nil, ErrBadRequest
	}
	if len(name) > 64 {
		name = name[:64]
	}
	res := s.db().Model(&model.KdzsPrintDevice{}).Where("id = ?", id).Update("name", name)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrDeviceNotFound
	}
	var d model.KdzsPrintDevice
	if err := s.db().First(&d, id).Error; err != nil {
		return nil, err
	}
	dto := toDeviceDTO(d)
	return &dto, nil
}

func (s *KdzsPrintAgentService) UnbindDevice(id uint64) error {
	if id == 0 {
		return ErrBadRequest
	}
	res := s.db().Model(&model.KdzsPrintDevice{}).Where("id = ?", id).Update("enabled", false)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

type CreatePrintTaskInput struct {
	DeviceID uint64          `json:"deviceId"`
	Payload  json.RawMessage `json:"payload"`
}

type KdzsPrintTaskDTO struct {
	ID           uint64          `json:"id"`
	DeviceID     uint64          `json:"deviceId"`
	Status       string          `json:"status"`
	Payload      json.RawMessage `json:"payload"`
	ErrorMessage string          `json:"errorMessage,omitempty"`
	CreatedAt    string          `json:"createdAt"`
	ClaimedAt    *string         `json:"claimedAt,omitempty"`
	FinishedAt   *string         `json:"finishedAt,omitempty"`
}

func toTaskDTO(t model.KdzsPrintTask) KdzsPrintTaskDTO {
	out := KdzsPrintTaskDTO{
		ID:           t.ID,
		DeviceID:     t.DeviceID,
		Status:       t.Status,
		Payload:      json.RawMessage(t.Payload),
		ErrorMessage: t.ErrorMessage,
		CreatedAt:    t.CreatedAt.Format(time.RFC3339),
	}
	if t.ClaimedAt != nil {
		s := t.ClaimedAt.Format(time.RFC3339)
		out.ClaimedAt = &s
	}
	if t.FinishedAt != nil {
		s := t.FinishedAt.Format(time.RFC3339)
		out.FinishedAt = &s
	}
	return out
}

func (s *KdzsPrintAgentService) CreateTask(userID uint64, in *CreatePrintTaskInput) (*KdzsPrintTaskDTO, error) {
	if in == nil || in.DeviceID == 0 || len(in.Payload) == 0 {
		return nil, ErrBadRequest
	}
	var probe map[string]any
	if err := json.Unmarshal(in.Payload, &probe); err != nil {
		return nil, fmt.Errorf("%w: payload 须为 JSON 对象", ErrBadRequest)
	}
	var d model.KdzsPrintDevice
	if err := s.db().Where("id = ? AND enabled = true", in.DeviceID).First(&d).Error; err != nil {
		return nil, ErrDeviceNotFound
	}
	if !deviceOnline(d.LastSeenAt) {
		return nil, ErrDeviceOffline
	}
	task := model.KdzsPrintTask{
		TenantID:  s.tenantID,
		DeviceID:  d.ID,
		Status:    model.KdzsPrintTaskPending,
		Payload:   string(in.Payload),
		CreatedBy: userID,
	}
	if err := s.repos.DB.Create(&task).Error; err != nil {
		return nil, err
	}
	dto := toTaskDTO(task)
	return &dto, nil
}

func (s *KdzsPrintAgentService) ListRecentTasks(limit int) ([]KdzsPrintTaskDTO, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var rows []model.KdzsPrintTask
	if err := s.db().Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]KdzsPrintTaskDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toTaskDTO(r))
	}
	return out, nil
}

// ClaimNext 扩展领取下一待办（同设备串行）。
func (s *KdzsPrintAgentService) ClaimNext(deviceKey, secret string) (*KdzsPrintTaskDTO, error) {
	d, err := s.AuthenticateDevice(deviceKey, secret)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	_ = s.repos.DB.Model(d).Update("last_seen_at", now)

	var task model.KdzsPrintTask
	err = s.repos.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("tenant_id = ? AND device_id = ? AND status = ?", d.TenantID, d.ID, model.KdzsPrintTaskPending).
			Order("id ASC").
			First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNoTask
			}
			return err
		}
		return tx.Model(&task).Updates(map[string]any{
			"status":     model.KdzsPrintTaskClaimed,
			"claimed_at": now,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	task.Status = model.KdzsPrintTaskClaimed
	task.ClaimedAt = &now
	dto := toTaskDTO(task)
	return &dto, nil
}

type ReportTaskInput struct {
	Status       string `json:"status"` // done | failed
	ErrorMessage string `json:"errorMessage"`
}

func (s *KdzsPrintAgentService) ReportTask(deviceKey, secret string, taskID uint64, in *ReportTaskInput) (*KdzsPrintTaskDTO, error) {
	d, err := s.AuthenticateDevice(deviceKey, secret)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, ErrBadRequest
	}
	st := strings.TrimSpace(in.Status)
	if st != model.KdzsPrintTaskDone && st != model.KdzsPrintTaskFailed {
		return nil, fmt.Errorf("%w: status 须为 done 或 failed", ErrBadRequest)
	}
	var task model.KdzsPrintTask
	if err := s.repos.DB.Where("id = ? AND tenant_id = ? AND device_id = ?", taskID, d.TenantID, d.ID).
		First(&task).Error; err != nil {
		return nil, ErrNotFound
	}
	if task.Status != model.KdzsPrintTaskClaimed && task.Status != model.KdzsPrintTaskPending {
		return nil, fmt.Errorf("%w: 任务状态不可更新", ErrBadRequest)
	}
	now := time.Now()
	updates := map[string]any{
		"status":      st,
		"finished_at": now,
	}
	if st == model.KdzsPrintTaskFailed {
		msg := strings.TrimSpace(in.ErrorMessage)
		if len(msg) > 1000 {
			msg = msg[:1000]
		}
		updates["error_message"] = msg
	}
	if err := s.repos.DB.Model(&task).Updates(updates).Error; err != nil {
		return nil, err
	}
	_ = s.repos.DB.First(&task, task.ID)
	dto := toTaskDTO(task)
	return &dto, nil
}
