package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"shippingcore/internal/integrations/agentscenter"
	"shippingcore/internal/model"
	"shippingcore/internal/repo"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	kdzsDeviceOnlineSkew = 90 * time.Second // WA 心跳约 12s；略放宽避免抖动标离线
	kdzsDeviceStaleAfter = 3 * 24 * time.Hour
	kdzsPrintSkillID     = "kdzs.remote.print"
)

var (
	ErrEnrollTokenInvalid = errors.New("注册令牌无效")
	ErrDeviceAuth         = errors.New("设备鉴权失败")
	ErrDeviceOffline      = errors.New("打单电脑不在线")
	ErrDeviceNotFound     = errors.New("设备不存在")
	ErrNoTask             = errors.New("暂无待领任务")
	ErrAgentsUnavailable  = errors.New("Agents 中心不可用")
)

type KdzsPrintAgentService struct {
	repos    *repo.Repos
	agents   *agentscenter.Client
	tenantID uint64
}

func NewKdzsPrintAgentService(repos *repo.Repos, agents *agentscenter.Client) *KdzsPrintAgentService {
	return &KdzsPrintAgentService{repos: repos, agents: agents}
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

type KdzsPrintDeviceDTO struct {
	ID         uint64  `json:"id"`
	MachineID  string  `json:"machineId"`
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
		MachineID: d.MachineID,
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

type RegisterPrintMachineInput struct {
	EnrollToken string `json:"enrollToken"`
	MachineID   string `json:"machineId"`
	Name        string `json:"name"`
}

type RegisterPrintMachineResult struct {
	DeviceID     uint64 `json:"deviceId"`
	DeviceKey    string `json:"deviceKey"`
	DeviceSecret string `json:"deviceSecret"`
	MachineID    string `json:"machineId"`
	Name         string `json:"name"`
	TenantID     uint64 `json:"tenantId"`
}

// RegisterMachine WA 用租户注册令牌自助登记打单机；同 machineId 重复注册则轮换密钥并上线。
func (s *KdzsPrintAgentService) RegisterMachine(in *RegisterPrintMachineInput) (*RegisterPrintMachineResult, error) {
	if in == nil {
		return nil, ErrBadRequest
	}
	token := strings.TrimSpace(in.EnrollToken)
	machineID := strings.TrimSpace(in.MachineID)
	name := strings.TrimSpace(in.Name)
	if token == "" || machineID == "" {
		return nil, fmt.Errorf("%w: enrollToken/machineId 必填", ErrBadRequest)
	}
	if name == "" {
		name = machineID
	}
	if len(name) > 64 {
		name = name[:64]
	}
	if len(machineID) > 128 {
		machineID = machineID[:128]
	}

	var st model.KdzsSetting
	if err := s.repos.DB.Where("print_enroll_token = ?", token).First(&st).Error; err != nil {
		return nil, ErrEnrollTokenInvalid
	}
	tenantID := st.TenantID

	deviceKey, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	secret, err := randomHex(24)
	if err != nil {
		return nil, err
	}
	now := time.Now()

	var dev model.KdzsPrintDevice
	err = s.repos.DB.Transaction(func(tx *gorm.DB) error {
		q := tx.Where("tenant_id = ? AND machine_id = ?", tenantID, machineID).First(&dev)
		if q.Error == nil {
			return tx.Model(&dev).Updates(map[string]any{
				"device_key":   deviceKey,
				"secret_hash": hashSecret(secret),
				"name":        name,
				"enabled":     true,
				"last_seen_at": now,
			}).Error
		}
		if !errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return q.Error
		}
		dev = model.KdzsPrintDevice{
			TenantID:   tenantID,
			MachineID:  machineID,
			DeviceKey:  deviceKey,
			SecretHash: hashSecret(secret),
			Name:       name,
			LastSeenAt: &now,
			Enabled:    true,
		}
		return tx.Create(&dev).Error
	})
	if err != nil {
		return nil, err
	}
	// 重新读 id
	if err := s.repos.DB.Where("tenant_id = ? AND machine_id = ?", tenantID, machineID).First(&dev).Error; err != nil {
		return nil, err
	}
	return &RegisterPrintMachineResult{
		DeviceID:     dev.ID,
		DeviceKey:    deviceKey,
		DeviceSecret: secret,
		MachineID:    machineID,
		Name:         name,
		TenantID:     tenantID,
	}, nil
}

func (s *KdzsPrintAgentService) EnsurePrintEnrollToken() (string, error) {
	if s.tenantID == 0 {
		return "", ErrBadRequest
	}
	var st model.KdzsSetting
	err := s.db().Where("tenant_id = ?", s.tenantID).First(&st).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		tok, e := randomHex(16)
		if e != nil {
			return "", e
		}
		st = model.KdzsSetting{
			TenantID:         s.tenantID,
			AutoSyncFromSSA:  true,
			PrintEnrollToken: tok,
		}
		if e := s.repos.DB.Create(&st).Error; e != nil {
			return "", e
		}
		return tok, nil
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(st.PrintEnrollToken) == "" {
		tok, e := randomHex(16)
		if e != nil {
			return "", e
		}
		st.PrintEnrollToken = tok
		if e := s.repos.DB.Model(&st).Update("print_enroll_token", tok).Error; e != nil {
			return "", e
		}
		return tok, nil
	}
	return st.PrintEnrollToken, nil
}

func (s *KdzsPrintAgentService) RotatePrintEnrollToken() (string, error) {
	if s.tenantID == 0 {
		return "", ErrBadRequest
	}
	tok, err := randomHex(16)
	if err != nil {
		return "", err
	}
	var st model.KdzsSetting
	err = s.db().Where("tenant_id = ?", s.tenantID).First(&st).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		st = model.KdzsSetting{
			TenantID:         s.tenantID,
			AutoSyncFromSSA:  true,
			PrintEnrollToken: tok,
		}
		if e := s.repos.DB.Create(&st).Error; e != nil {
			return "", e
		}
		return tok, nil
	}
	if err != nil {
		return "", err
	}
	if err := s.repos.DB.Model(&st).Update("print_enroll_token", tok).Error; err != nil {
		return "", err
	}
	return tok, nil
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
	if err := s.repos.DB.First(d, d.ID).Error; err != nil {
		return nil, err
	}
	d.LastSeenAt = &now
	dto := toDeviceDTO(*d)
	return &dto, nil
}

func (s *KdzsPrintAgentService) ListDevices() ([]KdzsPrintDeviceDTO, error) {
	// 打单机 = Agents 中心在线且具备 kdzs.remote.print 的 WA；无需配对/注册令牌。
	if s.agents == nil {
		return nil, ErrAgentsUnavailable
	}
	agents, err := s.agents.ListAgents(s.tenantID, false, kdzsPrintSkillID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAgentsUnavailable, err)
	}
	out := make([]KdzsPrintDeviceDTO, 0, len(agents))
	for _, a := range agents {
		online := strings.EqualFold(a.Status, "online")
		dto := KdzsPrintDeviceDTO{
			ID:        a.ID,
			MachineID: a.MachineID,
			DeviceKey: a.MachineID,
			Name:      a.Name,
			Online:    online,
			Enabled:   true,
			CreatedAt: a.CreatedAt,
		}
		if a.LastHeartbeat != nil {
			dto.LastSeenAt = a.LastHeartbeat
		}
		out = append(out, dto)
	}
	return out, nil
}

// pruneStaleDevices 关闭长时间无心跳的机器，避免打单页堆满离线项。
func (s *KdzsPrintAgentService) pruneStaleDevices() {
	cutoff := time.Now().Add(-kdzsDeviceStaleAfter)
	_ = s.db().Model(&model.KdzsPrintDevice{}).
		Where("enabled = true AND (last_seen_at IS NULL OR last_seen_at < ?)", cutoff).
		Update("enabled", false).Error
}

func (s *KdzsPrintAgentService) RenameDevice(id uint64, name string) (*KdzsPrintDeviceDTO, error) {
	return nil, fmt.Errorf("%w: 打单机随 Agents 中心在线状态出现，请在 Agents 中心改名", ErrBadRequest)
}

func (s *KdzsPrintAgentService) UnbindDevice(id uint64) error {
	return fmt.Errorf("%w: 打单机无需解绑；关闭 WindowsAgent 或从 Agents 中心下线即可", ErrBadRequest)
}

type CreatePrintTaskInput struct {
	DeviceID    uint64          `json:"deviceId"`
	AccountCode string          `json:"accountCode,omitempty"` // 可选；空则用当前活跃快递助手账号
	Payload     json.RawMessage `json:"payload"`
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

// toPublicTaskDTO 给管理端/手机端：去掉密码字段，避免泄露。
func toPublicTaskDTO(t model.KdzsPrintTask) KdzsPrintTaskDTO {
	dto := toTaskDTO(t)
	dto.Payload = redactPrintPayload(t.Payload)
	return dto
}

func redactPrintPayload(raw string) json.RawMessage {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil || m == nil {
		return json.RawMessage(raw)
	}
	if _, ok := m["kdzsPassword"]; ok {
		m["kdzsPasswordSet"] = true
		delete(m, "kdzsPassword")
	}
	b, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage(raw)
	}
	return b
}

func (s *KdzsPrintAgentService) resolvePrintLogin(accountCode string) (mobile, password, code, name string, err error) {
	code = strings.TrimSpace(accountCode)
	if code == "" {
		var st model.KdzsSetting
		if e := s.db().Where("tenant_id = ?", s.tenantID).First(&st).Error; e == nil {
			code = strings.TrimSpace(st.ActiveAccountCode)
			if code == "" {
				code = strings.TrimSpace(st.DefaultAccountCode)
			}
		}
	}
	if code == "" {
		var first model.KdzsAccount
		if e := s.db().Where("enabled = true").Order("sort_order ASC, id ASC").First(&first).Error; e == nil {
			code = first.Code
		}
	}
	if code == "" {
		return "", "", "", "", fmt.Errorf("%w: 请先在发货中心配置快递助手账号", ErrBadRequest)
	}
	var rec model.KdzsAccount
	if e := s.db().Where("code = ? AND enabled = true", code).First(&rec).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return "", "", "", "", fmt.Errorf("%w: 快递助手账号不存在或未启用", ErrBadRequest)
		}
		return "", "", "", "", e
	}
	mobile = strings.TrimSpace(rec.Mobile)
	password = rec.Password
	if mobile == "" || password == "" {
		return "", "", "", "", fmt.Errorf("%w: 快递助手账号「%s」缺少手机号或密码", ErrBadRequest, code)
	}
	name = strings.TrimSpace(rec.Name)
	if name == "" {
		name = mobile
	}
	return mobile, password, rec.Code, name, nil
}

func (s *KdzsPrintAgentService) CreateTask(userID uint64, in *CreatePrintTaskInput) (*KdzsPrintTaskDTO, error) {
	if in == nil || in.DeviceID == 0 || len(in.Payload) == 0 {
		return nil, ErrBadRequest
	}
	if s.agents == nil {
		return nil, ErrAgentsUnavailable
	}
	var probe map[string]any
	if err := json.Unmarshal(in.Payload, &probe); err != nil {
		return nil, fmt.Errorf("%w: payload 须为 JSON 对象", ErrBadRequest)
	}
	mobile, password, code, name, err := s.resolvePrintLogin(in.AccountCode)
	if err != nil {
		return nil, err
	}
	// 由发货中心注入登录态；不依赖 WindowsAgent 本地手填账号。
	probe["kdzsMobile"] = mobile
	probe["kdzsPassword"] = password
	probe["kdzsAccountCode"] = code
	probe["kdzsAccountName"] = name
	if probe["autoPrint"] == nil {
		probe["autoPrint"] = true
	}
	enriched, err := json.Marshal(probe)
	if err != nil {
		return nil, err
	}

	// DeviceID = AgentsCenter agentId；在线校验由 CreateTargetedJob 完成。
	acJob, err := s.agents.CreateTargetedJob(s.tenantID, in.DeviceID, kdzsPrintSkillID, string(enriched), "shipping")
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "离线") {
			return nil, ErrDeviceOffline
		}
		if strings.Contains(msg, "不存在") {
			return nil, ErrDeviceNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrAgentsUnavailable, err)
	}

	task := model.KdzsPrintTask{
		TenantID:  s.tenantID,
		DeviceID:  in.DeviceID,
		Status:    model.KdzsPrintTaskPending,
		Payload:   string(enriched),
		CreatedBy: userID,
	}
	if acJob != nil && acJob.ID > 0 {
		task.ErrorMessage = fmt.Sprintf("agentsJobId=%d", acJob.ID)
	}
	if err := s.repos.DB.Create(&task).Error; err != nil {
		return nil, err
	}
	dto := toPublicTaskDTO(task)
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
		out = append(out, toPublicTaskDTO(r))
	}
	return out, nil
}

// ClaimNext 打单端（WindowsAgent）领取下一待办（同设备串行）。
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
