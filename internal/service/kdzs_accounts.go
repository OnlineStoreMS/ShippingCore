package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"shippingcore/internal/integrations/storesyncagent"
	"shippingcore/internal/model"

	"gorm.io/gorm"
)

type KdzsAccountDetail struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Role        string `json:"role"`
	RoleLabel   string `json:"roleLabel"`
	Mobile      string `json:"mobile"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sortOrder"`
	PasswordSet bool   `json:"passwordSet"`
	Active      bool   `json:"active"`
	IsDefault   bool   `json:"isDefault"`
	Source      string `json:"source"`
	SourceLabel string `json:"sourceLabel"`
}

type ssaExportPayload struct {
	Items              []ssaExportItem `json:"items"`
	Total              int             `json:"total"`
	DefaultAccountCode string          `json:"defaultAccountCode"`
	ActiveAccountCode  string          `json:"activeAccountCode"`
}

type ssaExportItem struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Mobile    string `json:"mobile"`
	Password  string `json:"password"`
	Enabled   bool   `json:"enabled"`
	SortOrder int    `json:"sortOrder"`
	IsDefault bool   `json:"isDefault"`
	Active    bool   `json:"active"`
}

func roleLabel(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "factory":
		return "厂家版"
	default:
		return "商家版"
	}
}

func sourceLabel(source string) string {
	if source == model.KdzsAccountSourceLocal {
		return "本地"
	}
	return "StoreSyncAgent"
}

func (s *KdzsService) getOrCreateSettings() (*model.KdzsSetting, error) {
	var st model.KdzsSetting
	err := s.db().Where("tenant_id = ?", s.tenantID).First(&st).Error
	if err == nil {
		return &st, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}
	st = model.KdzsSetting{
		TenantID:        s.tenantID,
		AutoSyncFromSSA: true,
	}
	if err := s.repos.DB.Create(&st).Error; err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *KdzsService) listLocalAccounts() ([]model.KdzsAccount, error) {
	var list []model.KdzsAccount
	err := s.db().Order("sort_order ASC, id ASC").Find(&list).Error
	return list, err
}

func (s *KdzsService) toDetails(list []model.KdzsAccount, st *model.KdzsSetting) []KdzsAccountDetail {
	items := make([]KdzsAccountDetail, 0, len(list))
	for _, rec := range list {
		items = append(items, KdzsAccountDetail{
			Code:        rec.Code,
			Name:        rec.Name,
			Role:        rec.Role,
			RoleLabel:   roleLabel(rec.Role),
			Mobile:      rec.Mobile,
			Enabled:     rec.Enabled,
			SortOrder:   rec.SortOrder,
			PasswordSet: rec.Password != "",
			Active:      st != nil && rec.Code == st.ActiveAccountCode,
			IsDefault:   st != nil && rec.Code == st.DefaultAccountCode,
			Source:      rec.Source,
			SourceLabel: sourceLabel(rec.Source),
		})
	}
	return items
}

// EnsureAccountsReady 默认自动从 StoreSyncAgent 同步；本地已有账号时按 AutoSync 策略处理。
func (s *KdzsService) EnsureAccountsReady(ctx context.Context, token string) error {
	st, err := s.getOrCreateSettings()
	if err != nil {
		return err
	}
	var count int64
	if err := s.db().Model(&model.KdzsAccount{}).Count(&count).Error; err != nil {
		return err
	}
	// 本地为空时强制同步；开启自动同步时每次进入也刷新 SSA 账号
	if count == 0 || st.AutoSyncFromSSA {
		_, err := s.SyncAccountsFromSSA(ctx, token, false)
		return err
	}
	return nil
}

// SyncAccountsFromSSA 从 StoreSyncAgent 同步账号到发货中心。
// adoptSettings=true 时强制采用 SSA 的默认/当前账号；否则仅在本地未设置时采用。
func (s *KdzsService) SyncAccountsFromSSA(ctx context.Context, token string, adoptSettings bool) (map[string]any, error) {
	if s.ssAgent == nil {
		return nil, fmt.Errorf("storesyncagent 未配置")
	}
	raw, err := s.ssAgent.ExportKdzsAccounts(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("从 StoreSyncAgent 导出账号失败: %w", err)
	}
	var payload ssaExportPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("解析 StoreSyncAgent 账号导出失败: %w", err)
	}

	st, err := s.getOrCreateSettings()
	if err != nil {
		return nil, err
	}

	synced := 0
	for _, item := range payload.Items {
		code := strings.TrimSpace(item.Code)
		if code == "" || strings.TrimSpace(item.Mobile) == "" {
			continue
		}
		var existing model.KdzsAccount
		err := s.db().Where("code = ?", code).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			name := strings.TrimSpace(item.Name)
			if name == "" {
				name = item.Mobile
			}
			role := strings.TrimSpace(item.Role)
			if role == "" {
				role = "merchant"
			}
			rec := model.KdzsAccount{
				TenantID:  s.tenantID,
				Code:      code,
				Name:      name,
				Role:      role,
				Mobile:    strings.TrimSpace(item.Mobile),
				Password:  item.Password,
				SortOrder: item.SortOrder,
				Enabled:   item.Enabled,
				Source:    model.KdzsAccountSourceSSA,
			}
			if err := s.repos.DB.Create(&rec).Error; err != nil {
				return nil, err
			}
			synced++
			continue
		}
		if err != nil {
			return nil, err
		}
		// 已存在：更新 SSA 来源的元数据与密码；本地账号保留本地密码（除非本地密码为空）
		existing.Name = firstNonEmpty(strings.TrimSpace(item.Name), existing.Name)
		if r := strings.TrimSpace(item.Role); r != "" {
			existing.Role = r
		}
		existing.Mobile = strings.TrimSpace(item.Mobile)
		existing.SortOrder = item.SortOrder
		existing.Enabled = item.Enabled
		if item.Password != "" && (existing.Source == model.KdzsAccountSourceSSA || existing.Password == "") {
			existing.Password = item.Password
		}
		if existing.Source == "" {
			existing.Source = model.KdzsAccountSourceSSA
		}
		if err := s.repos.DB.Save(&existing).Error; err != nil {
			return nil, err
		}
		synced++
	}

	now := time.Now()
	st.LastSyncedAt = &now
	if adoptSettings || st.DefaultAccountCode == "" {
		if payload.DefaultAccountCode != "" {
			st.DefaultAccountCode = payload.DefaultAccountCode
		} else if len(payload.Items) > 0 {
			st.DefaultAccountCode = payload.Items[0].Code
		}
	}
	if adoptSettings || st.ActiveAccountCode == "" {
		if payload.ActiveAccountCode != "" {
			st.ActiveAccountCode = payload.ActiveAccountCode
		} else if st.DefaultAccountCode != "" {
			st.ActiveAccountCode = st.DefaultAccountCode
		}
	}
	if err := s.repos.DB.Save(st).Error; err != nil {
		return nil, err
	}

	// 同步后切换 SSA 会话到发货中心当前账号，保证后续打单/同步可用
	if st.ActiveAccountCode != "" {
		_ = s.ensureSSASession(ctx, token, st.ActiveAccountCode)
	}

	return map[string]any{
		"synced":             synced,
		"defaultAccountCode": st.DefaultAccountCode,
		"activeAccountCode":  st.ActiveAccountCode,
		"lastSyncedAt":       now,
	}, nil
}

func (s *KdzsService) ListAccountDetailsLocal(ctx context.Context, token string) ([]KdzsAccountDetail, error) {
	if err := s.EnsureAccountsReady(ctx, token); err != nil {
		return nil, err
	}
	st, err := s.getOrCreateSettings()
	if err != nil {
		return nil, err
	}
	list, err := s.listLocalAccounts()
	if err != nil {
		return nil, err
	}
	return s.toDetails(list, st), nil
}

func (s *KdzsService) CreateLocalAccount(ctx context.Context, token string, in storesyncagent.KdzsAccountInput) (*KdzsAccountDetail, error) {
	code := strings.TrimSpace(in.Code)
	mobile := strings.TrimSpace(in.Mobile)
	if code == "" {
		return nil, fmt.Errorf("账号 ID 不能为空")
	}
	if mobile == "" {
		return nil, fmt.Errorf("手机号不能为空")
	}
	if strings.TrimSpace(in.Password) == "" {
		return nil, fmt.Errorf("密码不能为空")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = mobile
	}
	role := strings.TrimSpace(in.Role)
	if role == "" {
		role = "merchant"
	}
	var count int64
	_ = s.db().Model(&model.KdzsAccount{}).Where("code = ?", code).Count(&count).Error
	if count > 0 {
		return nil, fmt.Errorf("账号 %s 已存在", code)
	}
	rec := model.KdzsAccount{
		TenantID:  s.tenantID,
		Code:      code,
		Name:      name,
		Role:      role,
		Mobile:    mobile,
		Password:  in.Password,
		Enabled:   true, // 新建默认启用；前端也可随后编辑关闭
		Source:    model.KdzsAccountSourceLocal,
		SortOrder: in.SortOrder,
	}

	if err := s.repos.DB.Create(&rec).Error; err != nil {
		return nil, err
	}
	st, err := s.getOrCreateSettings()
	if err != nil {
		return nil, err
	}
	var total int64
	_ = s.db().Model(&model.KdzsAccount{}).Count(&total).Error
	if total == 1 || st.DefaultAccountCode == "" {
		st.DefaultAccountCode = rec.Code
		st.ActiveAccountCode = rec.Code
		_ = s.repos.DB.Save(st).Error
	}
	// 推送到 SSA，便于打单会话（失败不阻断本地创建）
	_, _ = s.ssAgent.CreateKdzsAccount(ctx, token, storesyncagent.KdzsAccountInput{
		Code:      rec.Code,
		Name:      rec.Name,
		Role:      rec.Role,
		Mobile:    rec.Mobile,
		Password:  rec.Password,
		Enabled:   rec.Enabled,
		SortOrder: rec.SortOrder,
	})

	details, err := s.ListAccountDetailsLocal(ctx, token)
	if err != nil {
		return nil, err
	}
	for i := range details {
		if details[i].Code == rec.Code {
			return &details[i], nil
		}
	}
	return nil, fmt.Errorf("account created but not found")
}

func (s *KdzsService) UpdateLocalAccount(ctx context.Context, token, code string, in storesyncagent.KdzsAccountUpdateInput) (*KdzsAccountDetail, error) {
	var rec model.KdzsAccount
	if err := s.db().Where("code = ?", code).First(&rec).Error; err != nil {
		return nil, err
	}
	if v := strings.TrimSpace(in.Name); v != "" {
		rec.Name = v
	}
	if v := strings.TrimSpace(in.Role); v != "" {
		rec.Role = v
	}
	if v := strings.TrimSpace(in.Mobile); v != "" {
		rec.Mobile = v
	}
	if in.Password != "" {
		rec.Password = in.Password
	}
	if in.Enabled != nil {
		rec.Enabled = *in.Enabled
	}
	if in.SortOrder != nil {
		rec.SortOrder = *in.SortOrder
	}
	if err := s.repos.DB.Save(&rec).Error; err != nil {
		return nil, err
	}
	upd := storesyncagent.KdzsAccountUpdateInput{
		Name:     rec.Name,
		Role:     rec.Role,
		Mobile:   rec.Mobile,
		Password: in.Password,
		Enabled:  in.Enabled,
	}
	_, _ = s.ssAgent.UpdateKdzsAccount(ctx, token, code, upd)

	details, err := s.ListAccountDetailsLocal(ctx, token)
	if err != nil {
		return nil, err
	}
	for i := range details {
		if details[i].Code == code {
			return &details[i], nil
		}
	}
	return nil, fmt.Errorf("account not found")
}

func (s *KdzsService) DeleteLocalAccount(ctx context.Context, token, code string) error {
	st, err := s.getOrCreateSettings()
	if err != nil {
		return err
	}
	if st.DefaultAccountCode == code {
		return fmt.Errorf("不能删除默认账号，请先设置其他账号为默认")
	}
	if st.ActiveAccountCode == code {
		return fmt.Errorf("不能删除当前使用中的账号，请先切换到其他账号")
	}
	res := s.db().Where("code = ?", code).Delete(&model.KdzsAccount{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("账号不存在")
	}
	// 不强制删除 SSA 侧，允许两边不一致
	return nil
}

func (s *KdzsService) SetLocalDefault(ctx context.Context, token, code string) error {
	var count int64
	if err := s.db().Model(&model.KdzsAccount{}).Where("code = ? AND enabled = ?", code, true).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("账号 %s 不存在或未启用", code)
	}
	st, err := s.getOrCreateSettings()
	if err != nil {
		return err
	}
	st.DefaultAccountCode = code
	return s.repos.DB.Save(st).Error
}

func (s *KdzsService) SwitchLocalAccount(ctx context.Context, token, code string) error {
	var rec model.KdzsAccount
	if err := s.db().Where("code = ? AND enabled = ?", code, true).First(&rec).Error; err != nil {
		return fmt.Errorf("账号 %s 不存在或未启用", code)
	}
	if err := s.ensureSSASession(ctx, token, code); err != nil {
		return err
	}
	st, err := s.getOrCreateSettings()
	if err != nil {
		return err
	}
	st.ActiveAccountCode = code
	return s.repos.DB.Save(st).Error
}

// ensureSSASession 确保 SSA 侧存在该账号并切换会话。
func (s *KdzsService) ensureSSASession(ctx context.Context, token, code string) error {
	if s.ssAgent == nil {
		return fmt.Errorf("storesyncagent 未配置")
	}
	var rec model.KdzsAccount
	if err := s.db().Where("code = ?", code).First(&rec).Error; err != nil {
		return fmt.Errorf("本地账号不存在: %s", code)
	}
	if rec.Password == "" {
		return fmt.Errorf("账号 %s 未设置密码，请先编辑补充", code)
	}
	// 尝试切换；失败则先创建/更新再切换
	if _, err := s.ssAgent.SwitchKdzsAccount(ctx, token, code); err == nil {
		return nil
	}
	enabled := rec.Enabled
	_, _ = s.ssAgent.CreateKdzsAccount(ctx, token, storesyncagent.KdzsAccountInput{
		Code:      rec.Code,
		Name:      rec.Name,
		Role:      rec.Role,
		Mobile:    rec.Mobile,
		Password:  rec.Password,
		Enabled:   enabled,
		SortOrder: rec.SortOrder,
	})
	_, _ = s.ssAgent.UpdateKdzsAccount(ctx, token, code, storesyncagent.KdzsAccountUpdateInput{
		Name:     rec.Name,
		Role:     rec.Role,
		Mobile:   rec.Mobile,
		Password: rec.Password,
		Enabled:  &enabled,
	})
	_, err := s.ssAgent.SwitchKdzsAccount(ctx, token, code)
	return err
}

func (s *KdzsService) EnsureActiveSSASession(ctx context.Context, token string) error {
	if err := s.EnsureAccountsReady(ctx, token); err != nil {
		return err
	}
	st, err := s.getOrCreateSettings()
	if err != nil {
		return err
	}
	code := st.ActiveAccountCode
	if code == "" {
		code = st.DefaultAccountCode
	}
	if code == "" {
		list, err := s.listLocalAccounts()
		if err != nil || len(list) == 0 {
			return fmt.Errorf("请先同步或添加快递助手账号")
		}
		code = list[0].Code
		st.ActiveAccountCode = code
		if st.DefaultAccountCode == "" {
			st.DefaultAccountCode = code
		}
		_ = s.repos.DB.Save(st).Error
	}
	return s.ensureSSASession(ctx, token, code)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
