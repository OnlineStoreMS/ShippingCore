package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"shippingcore/internal/dto"
	"shippingcore/internal/integrations/agentscenter"
	"shippingcore/internal/model"
	jwtmgr "shippingcore/internal/pkg/jwt"
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
	repos     *repo.Repos
	agents    *agentscenter.Client
	shipments *ShipmentService
	jwt       *jwtmgr.Manager
	tenantID  uint64
}

func NewKdzsPrintAgentService(repos *repo.Repos, agents *agentscenter.Client, shipments *ShipmentService, jwt *jwtmgr.Manager) *KdzsPrintAgentService {
	return &KdzsPrintAgentService{repos: repos, agents: agents, shipments: shipments, jwt: jwt}
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
	ID              uint64          `json:"id"`
	DeviceID        uint64          `json:"deviceId"`
	Status          string          `json:"status"`
	Payload         json.RawMessage `json:"payload"`
	ErrorMessage    string          `json:"errorMessage,omitempty"`
	MailNo          string          `json:"mailNo,omitempty"`
	ShipConfirmedAt *string         `json:"shipConfirmedAt,omitempty"`
	CreatedAt       string          `json:"createdAt"`
	ClaimedAt       *string         `json:"claimedAt,omitempty"`
	FinishedAt      *string         `json:"finishedAt,omitempty"`
	// Merged 本次下发被并入已有 pending 批量任务（非新建）。
	Merged bool `json:"merged,omitempty"`
	// OrderCount payload.orders 笔数（合并后便于前端提示）。
	OrderCount int `json:"orderCount,omitempty"`
}

func toTaskDTO(t model.KdzsPrintTask) KdzsPrintTaskDTO {
	out := KdzsPrintTaskDTO{
		ID:           t.ID,
		DeviceID:     t.DeviceID,
		Status:       t.Status,
		Payload:      json.RawMessage(t.Payload),
		ErrorMessage: t.ErrorMessage,
		MailNo:       t.MailNo,
		CreatedAt:    t.CreatedAt.Format(time.RFC3339),
	}
	if t.ShipConfirmedAt != nil {
		s := t.ShipConfirmedAt.Format(time.RFC3339)
		out.ShipConfirmedAt = &s
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

// DefaultPrintLogin 租户默认/活跃快递助手账号（供 AgentsCenter → WA 常驻登录）。
func (s *KdzsPrintAgentService) DefaultPrintLogin(tenantID uint64) (mobile, password, code, name string, err error) {
	if tenantID == 0 {
		return "", "", "", "", fmt.Errorf("%w: tenantId 无效", ErrBadRequest)
	}
	return s.ForTenant(tenantID).resolvePrintLogin("")
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

func payloadString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == 0 {
			return ""
		}
		return strconv.FormatInt(int64(t), 10)
	case json.Number:
		return strings.TrimSpace(t.String())
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

// printMergeKey 同类可合并：同平台 + 同模板 + 同快递助手账号（打印机不参与键，合并时取非空）。
func printMergeKey(m map[string]any) (string, bool) {
	platform := strings.ToUpper(payloadString(m, "platform"))
	tplID := payloadString(m, "templateId")
	tplName := payloadString(m, "templateName")
	if platform == "" || (tplID == "" && tplName == "") {
		return "", false
	}
	account := payloadString(m, "kdzsAccountCode")
	return platform + "\x00" + tplID + "\x00" + tplName + "\x00" + account, true
}

func orderDedupeKey(o map[string]any) string {
	for _, k := range []string{"orderId", "platformSysTid", "sysTid", "platformOrderId", "tid", "orderNo"} {
		if v := payloadString(o, k); v != "" {
			return k + "=" + v
		}
	}
	raw, _ := json.Marshal(o)
	return string(raw)
}

func extractOrders(m map[string]any) []map[string]any {
	raw, ok := m["orders"]
	if !ok || raw == nil {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		om, ok := item.(map[string]any)
		if !ok || om == nil {
			continue
		}
		out = append(out, om)
	}
	return out
}

func mergePrintPayload(base, incoming map[string]any) (map[string]any, int) {
	merged := map[string]any{}
	for k, v := range base {
		merged[k] = v
	}
	// 凭证 / 模板以已有任务为准；打印机优先非空新值。
	for _, k := range []string{"kdzsMobile", "kdzsPassword", "kdzsAccountCode", "kdzsAccountName", "templateId", "templateName", "platform"} {
		if v := payloadString(base, k); v != "" {
			merged[k] = base[k]
		} else if _, ok := incoming[k]; ok {
			merged[k] = incoming[k]
		}
	}
	if p := payloadString(incoming, "printerName"); p != "" {
		merged["printerName"] = p
	} else if p := payloadString(base, "printerName"); p != "" {
		merged["printerName"] = p
	}
	if merged["autoPrint"] == nil {
		if incoming["autoPrint"] != nil {
			merged["autoPrint"] = incoming["autoPrint"]
		} else {
			merged["autoPrint"] = true
		}
	}

	seen := map[string]struct{}{}
	orders := make([]any, 0, 8)
	for _, src := range []map[string]any{base, incoming} {
		for _, o := range extractOrders(src) {
			key := orderDedupeKey(o)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			orders = append(orders, o)
		}
	}
	merged["orders"] = orders

	// 多单批量：去掉单票字段，避免 Agent 走单票确认发货。
	if len(orders) > 1 {
		delete(merged, "orderId")
		delete(merged, "order")
		delete(merged, "autoConfirmShip")
	} else if len(orders) == 1 {
		if om, ok := orders[0].(map[string]any); ok {
			if id := payloadString(om, "orderId"); id != "" {
				if n, err := strconv.ParseUint(id, 10, 64); err == nil && n > 0 {
					merged["orderId"] = n
				}
			}
		}
	}

	// 下单时间窗取并集，方便快递助手筛选。
	fromA, toA := payloadString(base, "orderTimeFrom"), payloadString(base, "orderTimeTo")
	fromB, toB := payloadString(incoming, "orderTimeFrom"), payloadString(incoming, "orderTimeTo")
	from, to := fromA, toA
	if fromB != "" && (from == "" || fromB < from) {
		from = fromB
	}
	if toB != "" && (to == "" || toB > to) {
		to = toB
	}
	if from != "" {
		merged["orderTimeFrom"] = from
	}
	if to != "" {
		merged["orderTimeTo"] = to
	}
	merged["createdAt"] = time.Now().UnixMilli()
	merged["v"] = 1
	return merged, len(orders)
}

// tryMergePendingPrintTask 将本次下发合并进同设备、同平台模板的 pending 任务。
// 上一批若正在跑，新单会堆进下一条 pending；同批多次单发会合成一条批量。
func (s *KdzsPrintAgentService) tryMergePendingPrintTask(userID, deviceID uint64, incoming map[string]any) (*KdzsPrintTaskDTO, error) {
	key, ok := printMergeKey(incoming)
	if !ok || s.agents == nil {
		return nil, nil
	}

	type absorbRef struct {
		id         uint64
		agentsJobID uint64
	}
	var survivorID, survivorJobID uint64
	var absorbList []absorbRef
	var mergedRaw []byte
	var orderCount int

	// 1) 锁定并算出合并结果（不调用外部 HTTP）。
	err := s.db().Transaction(func(tx *gorm.DB) error {
		var pending []model.KdzsPrintTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("tenant_id = ? AND device_id = ? AND status = ?", s.tenantID, deviceID, model.KdzsPrintTaskPending).
			Order("id ASC").
			Find(&pending).Error; err != nil {
			return err
		}
		var survivor model.KdzsPrintTask
		var absorbTasks []model.KdzsPrintTask
		for i := range pending {
			var probe map[string]any
			if err := json.Unmarshal([]byte(pending[i].Payload), &probe); err != nil {
				continue
			}
			k, ok := printMergeKey(probe)
			if !ok || k != key {
				continue
			}
			if survivor.ID == 0 {
				survivor = pending[i]
			} else {
				absorbTasks = append(absorbTasks, pending[i])
			}
		}
		if survivor.ID == 0 || survivor.AgentsJobID == 0 {
			return nil
		}
		var base map[string]any
		if err := json.Unmarshal([]byte(survivor.Payload), &base); err != nil {
			return err
		}
		merged := base
		for _, abs := range absorbTasks {
			var other map[string]any
			if err := json.Unmarshal([]byte(abs.Payload), &other); err != nil {
				continue
			}
			merged, orderCount = mergePrintPayload(merged, other)
		}
		merged, orderCount = mergePrintPayload(merged, incoming)
		raw, err := json.Marshal(merged)
		if err != nil {
			return err
		}
		survivorID = survivor.ID
		survivorJobID = survivor.AgentsJobID
		mergedRaw = raw
		for _, abs := range absorbTasks {
			absorbList = append(absorbList, absorbRef{id: abs.ID, agentsJobID: abs.AgentsJobID})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if survivorID == 0 || len(mergedRaw) == 0 {
		return nil, nil
	}

	// 2) 先更新 Agents；若已领取则放弃合并，走新建。
	if err := s.agents.UpdatePendingJobParams(s.tenantID, survivorJobID, string(mergedRaw)); err != nil {
		log.Printf("[kdzs-print] merge skipped device=%d job=%d: %v", deviceID, survivorJobID, err)
		return nil, nil
	}

	// 3) 回写发货中心任务，并取消被吞并的重复 pending。
	now := time.Now()
	var survivor model.KdzsPrintTask
	err = s.db().Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND tenant_id = ? AND status = ?", survivorID, s.tenantID, model.KdzsPrintTaskPending).
			First(&survivor).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if err := tx.Model(&survivor).Updates(map[string]any{
			"payload":    string(mergedRaw),
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		survivor.Payload = string(mergedRaw)
		survivor.UpdatedAt = now
		for _, abs := range absorbList {
			reason := fmt.Sprintf("已合并到批量任务 #%d", survivorID)
			_ = tx.Model(&model.KdzsPrintTask{}).
				Where("id = ? AND tenant_id = ? AND status = ?", abs.id, s.tenantID, model.KdzsPrintTaskPending).
				Updates(map[string]any{
					"status":        model.KdzsPrintTaskCancelled,
					"error_message": reason,
					"finished_at":   now,
					"updated_at":    now,
				}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if survivor.ID == 0 {
		// Agents 已合并；本地任务可能刚被同步状态，仍视为成功合并。
		survivor = model.KdzsPrintTask{
			ID:          survivorID,
			TenantID:    s.tenantID,
			DeviceID:    deviceID,
			Status:      model.KdzsPrintTaskPending,
			Payload:     string(mergedRaw),
			AgentsJobID: survivorJobID,
			CreatedBy:   userID,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
	}
	for _, abs := range absorbList {
		if abs.agentsJobID > 0 {
			reason := fmt.Sprintf("已合并到批量任务 #%d", survivorID)
			if cerr := s.agents.CancelPendingJob(s.tenantID, abs.agentsJobID, reason); cerr != nil {
				log.Printf("[kdzs-print] cancel absorbed job #%d: %v", abs.agentsJobID, cerr)
			}
		}
	}
	log.Printf("[kdzs-print] merged into task #%d device=%d orders=%d absorbed=%d by=%d",
		survivorID, deviceID, orderCount, len(absorbList), userID)
	dto := toPublicTaskDTO(survivor)
	dto.Merged = true
	dto.OrderCount = orderCount
	return &dto, nil
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

	// 同类多次单发：并入同设备 pending 批量（上一批执行中则堆到下一条 pending）。
	if merged, merr := s.tryMergePendingPrintTask(userID, in.DeviceID, probe); merr != nil {
		return nil, merr
	} else if merged != nil {
		return merged, nil
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
		task.AgentsJobID = acJob.ID
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
	s.syncTasksFromAgents(&rows)
	out := make([]KdzsPrintTaskDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toPublicTaskDTO(r))
	}
	return out, nil
}

// syncTasksFromAgents 把未完结的本地打单任务状态与 AgentsCenter 执行单对齐。
func (s *KdzsPrintAgentService) syncTasksFromAgents(rows *[]model.KdzsPrintTask) {
	if rows == nil || len(*rows) == 0 || s.agents == nil {
		return
	}
	need := make([]uint64, 0, len(*rows))
	for i := range *rows {
		r := &(*rows)[i]
		if r.AgentsJobID == 0 {
			if id := parseAgentsJobID(r.ErrorMessage); id > 0 {
				r.AgentsJobID = id
				_ = s.repos.DB.Model(r).Update("agents_job_id", id).Error
				if strings.HasPrefix(strings.TrimSpace(r.ErrorMessage), "agentsJobId=") {
					r.ErrorMessage = ""
					_ = s.repos.DB.Model(r).Update("error_message", "").Error
				}
			}
		}
		if r.AgentsJobID == 0 {
			continue
		}
		// 未完结，或已完成但尚未自动确认发货：都需要拉 ResultJSON
		if !isPrintTaskTerminal(r.Status) || (r.Status == model.KdzsPrintTaskDone && r.ShipConfirmedAt == nil) {
			need = append(need, r.AgentsJobID)
		}
	}
	if len(need) == 0 {
		return
	}
	jobs, err := s.agents.GetJobs(s.tenantID, need)
	if err != nil || len(jobs) == 0 {
		return
	}
	byID := make(map[uint64]agentscenter.JobStatus, len(jobs))
	for _, j := range jobs {
		byID[j.ID] = j
	}
	now := time.Now()
	for i := range *rows {
		r := &(*rows)[i]
		j, ok := byID[r.AgentsJobID]
		if !ok {
			continue
		}
		st, errMsg := mapAgentsJobStatus(j)
		if st == "" || st == r.Status {
			// 状态未变时仍可刷新失败文案 / 尝试自动确认
			if st == model.KdzsPrintTaskFailed && errMsg != "" && r.ErrorMessage != errMsg {
				r.ErrorMessage = errMsg
				_ = s.repos.DB.Model(r).Update("error_message", errMsg).Error
			}
			if r.Status == model.KdzsPrintTaskDone && r.ShipConfirmedAt == nil {
				s.maybeAutoConfirmShip(r, j.ResultJSON)
			}
			continue
		}
		updates := map[string]any{
			"status":     st,
			"updated_at": now,
		}
		if st == model.KdzsPrintTaskClaimed && r.ClaimedAt == nil {
			t := now
			if j.StartedAt != nil {
				if parsed, e := time.Parse(time.RFC3339, *j.StartedAt); e == nil {
					t = parsed
				}
			}
			r.ClaimedAt = &t
			updates["claimed_at"] = t
		}
		if isPrintTaskTerminal(st) {
			t := now
			if j.FinishedAt != nil {
				if parsed, e := time.Parse(time.RFC3339, *j.FinishedAt); e == nil {
					t = parsed
				}
			}
			r.FinishedAt = &t
			updates["finished_at"] = t
			if st == model.KdzsPrintTaskFailed {
				updates["error_message"] = errMsg
				r.ErrorMessage = errMsg
			} else if st == model.KdzsPrintTaskDone {
				updates["error_message"] = ""
				r.ErrorMessage = ""
			}
		}
		r.Status = st
		_ = s.repos.DB.Model(r).Updates(updates).Error
		if st == model.KdzsPrintTaskDone && r.ShipConfirmedAt == nil {
			s.maybeAutoConfirmShip(r, j.ResultJSON)
		}
	}
}

// StartBackgroundSync 轮询未完结/待自动确认的打单任务（不依赖前端刷列表）。
func (s *KdzsPrintAgentService) StartBackgroundSync(ctx context.Context) {
	if s == nil || s.agents == nil {
		return
	}
	go func() {
		t := time.NewTicker(12 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.pollOpenTasks()
			}
		}
	}()
}

func (s *KdzsPrintAgentService) pollOpenTasks() {
	var rows []model.KdzsPrintTask
	err := s.repos.DB.
		Where("agents_job_id > 0 AND (status IN ? OR (status = ? AND ship_confirmed_at IS NULL))",
			[]string{model.KdzsPrintTaskPending, model.KdzsPrintTaskClaimed},
			model.KdzsPrintTaskDone).
		Order("id DESC").
		Limit(80).
		Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return
	}
	// 按租户分组同步（GetJobs 带 tenantId）
	byTenant := map[uint64][]model.KdzsPrintTask{}
	for _, r := range rows {
		byTenant[r.TenantID] = append(byTenant[r.TenantID], r)
	}
	for tid, list := range byTenant {
		cp := list
		s.ForTenant(tid).syncTasksFromAgents(&cp)
	}
}

func parsePayloadUint64(v any) uint64 {
	switch x := v.(type) {
	case float64:
		if x > 0 {
			return uint64(x)
		}
	case json.Number:
		n, _ := x.Int64()
		if n > 0 {
			return uint64(n)
		}
	case string:
		n, _ := strconv.ParseUint(strings.TrimSpace(x), 10, 64)
		return n
	case int:
		if x > 0 {
			return uint64(x)
		}
	case int64:
		if x > 0 {
			return uint64(x)
		}
	case uint64:
		return x
	}
	return 0
}

func parseMailNoFromResult(resultJSON string) string {
	raw := strings.TrimSpace(resultJSON)
	if raw == "" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err == nil && m != nil {
		for _, key := range []string{"mailNo", "expressNo", "waybillNo", "trackingNo"} {
			if s, ok := m[key].(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					return s
				}
			}
		}
		// 旧版 Agent 只把运单号写在文案里：已打印并发货，运单号 7903…
		if msg, ok := m["message"].(string); ok {
			if n := extractMailNoFromText(msg); n != "" {
				return n
			}
		}
	}
	return extractMailNoFromText(raw)
}

var mailNoInTextRe = regexp.MustCompile(`(?i)(?:运单号|快递单号|面单号|mail\s*no|tracking)\s*[:：]?\s*([A-Za-z0-9]{8,32})`)

func extractMailNoFromText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if m := mailNoInTextRe.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func payloadBool(m map[string]any, key string) (val bool, present bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return false, false
	}
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		if s == "true" || s == "1" || s == "yes" {
			return true, true
		}
		if s == "false" || s == "0" || s == "no" {
			return false, true
		}
	case float64:
		return x != 0, true
	}
	return false, true
}

// maybeAutoConfirmShip Agent 打单成功并带回运单号后，自动 ConfirmKdzsShip 回写订单中心。
func (s *KdzsPrintAgentService) maybeAutoConfirmShip(task *model.KdzsPrintTask, resultJSON string) {
	if task == nil || task.ShipConfirmedAt != nil {
		return
	}
	if s.shipments == nil || s.jwt == nil {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(task.Payload), &payload); err != nil || payload == nil {
		return
	}
	auto, hasAuto := payloadBool(payload, "autoConfirmShip")
	orderID := parsePayloadUint64(payload["orderId"])
	if orderID == 0 {
		if orders, ok := payload["orders"].([]any); ok && len(orders) == 1 {
			if om, ok := orders[0].(map[string]any); ok {
				orderID = parsePayloadUint64(om["orderId"])
			}
		}
	}
	if orderID == 0 {
		return
	}
	// 显式 false 关闭；未传时有 orderId 则默认开启
	if hasAuto && !auto {
		return
	}
	if !hasAuto {
		auto = true
	}
	if !auto {
		return
	}
	// 批量多单一票：无法安全映射，跳过
	if orders, ok := payload["orders"].([]any); ok && len(orders) > 1 {
		return
	}

	mailNo := strings.TrimSpace(task.MailNo)
	if mailNo == "" {
		mailNo = parseMailNoFromResult(resultJSON)
	}
	if mailNo == "" {
		log.Printf("[kdzs-print] task=%d skip auto-confirm: no mailNo in result (order=%d)", task.ID, orderID)
		return
	}
	if task.MailNo != mailNo {
		task.MailNo = mailNo
		_ = s.repos.DB.Model(task).Update("mail_no", mailNo).Error
	}

	orderRaw, err := json.Marshal(payload["order"])
	if err != nil || len(orderRaw) == 0 || string(orderRaw) == "null" {
		log.Printf("[kdzs-print] task=%d skip auto-confirm: missing order snapshot", task.ID)
		return
	}
	var orderSnap dto.OrderSnapshotDTO
	if err := json.Unmarshal(orderRaw, &orderSnap); err != nil {
		log.Printf("[kdzs-print] task=%d skip auto-confirm: bad order snapshot: %v", task.ID, err)
		return
	}

	expressCompany := strings.TrimSpace(fmt.Sprint(payload["expressCompany"]))
	if expressCompany == "" || expressCompany == "<nil>" {
		expressCompany = "快递"
	}
	reship, _ := payloadBool(payload, "reship")
	var groupID *uint64
	if gid := parsePayloadUint64(payload["groupId"]); gid > 0 {
		groupID = &gid
	}

	token, err := s.jwt.IssueServiceToken(task.TenantID, task.CreatedBy, 15*time.Minute)
	if err != nil {
		log.Printf("[kdzs-print] task=%d issue token failed: %v", task.ID, err)
		return
	}
	shipSvc := s.shipments.ForTenant(task.TenantID)
	_, err = shipSvc.ConfirmKdzsShip(context.Background(), token, &dto.ConfirmKdzsShipDTO{
		OrderID:        orderID,
		ExpressNo:      mailNo,
		ExpressCompany: expressCompany,
		Order:          orderSnap,
		GroupID:        groupID,
		Reship:         reship,
	})
	if err != nil {
		// 发货中心/订单中心已有同号发货记录时，仍标记任务已确认，避免 12s 轮询空转
		if isOrderAlreadyShippedErr(err) || strings.Contains(err.Error(), "发货单已存在") {
			log.Printf("[kdzs-print] task=%d order=%d mail=%s already shipped, mark confirmed: %v", task.ID, orderID, mailNo, err)
		} else {
			log.Printf("[kdzs-print] task=%d auto ConfirmKdzsShip order=%d mail=%s failed: %v", task.ID, orderID, mailNo, err)
			return
		}
	}
	now := time.Now()
	task.ShipConfirmedAt = &now
	_ = s.repos.DB.Model(task).Updates(map[string]any{
		"mail_no":           mailNo,
		"ship_confirmed_at": now,
		"updated_at":        now,
	}).Error
	log.Printf("[kdzs-print] task=%d auto-confirmed ship order=%d mail=%s", task.ID, orderID, mailNo)
}

func isPrintTaskTerminal(status string) bool {
	switch status {
	case model.KdzsPrintTaskDone, model.KdzsPrintTaskFailed, model.KdzsPrintTaskCancelled:
		return true
	default:
		return false
	}
}

func mapAgentsJobStatus(j agentscenter.JobStatus) (status, errMsg string) {
	errMsg = strings.TrimSpace(j.ErrorMessage)
	switch strings.TrimSpace(j.Status) {
	case "pending":
		return model.KdzsPrintTaskPending, ""
	case "claimed", "running":
		return model.KdzsPrintTaskClaimed, ""
	case "succeeded":
		return model.KdzsPrintTaskDone, ""
	case "failed", "cancelled":
		return model.KdzsPrintTaskFailed, errMsg
	default:
		return "", ""
	}
}

func parseAgentsJobID(errMsg string) uint64 {
	msg := strings.TrimSpace(errMsg)
	if !strings.HasPrefix(msg, "agentsJobId=") {
		return 0
	}
	n, _ := strconv.ParseUint(strings.TrimPrefix(msg, "agentsJobId="), 10, 64)
	return n
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
