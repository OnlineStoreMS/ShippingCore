package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"shippingcore/internal/integrations/storesyncagent"
	"shippingcore/internal/model"
	"shippingcore/internal/repo"

	"gorm.io/gorm"
)

type KdzsService struct {
	repos    *repo.Repos
	ssAgent  *storesyncagent.Client
	tenantID uint64
}

func NewKdzsService(repos *repo.Repos, ssAgent *storesyncagent.Client) *KdzsService {
	return &KdzsService{repos: repos, ssAgent: ssAgent}
}

func (s *KdzsService) ForTenant(tenantID uint64) *KdzsService {
	return &KdzsService{
		repos:    s.repos,
		ssAgent:  s.ssAgent,
		tenantID: repo.NormalizeTenantID(tenantID),
	}
}

func (s *KdzsService) db() *gorm.DB {
	return s.repos.ForTenant(s.tenantID)
}

func (s *KdzsService) proxy(ctx context.Context, token string, fn func(*storesyncagent.Client, string) (json.RawMessage, error)) (json.RawMessage, error) {
	if s.ssAgent == nil {
		return nil, fmt.Errorf("storesyncagent 未配置")
	}
	return fn(s.ssAgent, token)
}

func (s *KdzsService) ListAccounts(ctx context.Context, token string) ([]KdzsAccountDetail, error) {
	return s.ListAccountDetailsLocal(ctx, token)
}

func (s *KdzsService) ListAccountDetails(ctx context.Context, token string) ([]KdzsAccountDetail, error) {
	return s.ListAccountDetailsLocal(ctx, token)
}

func (s *KdzsService) CreateAccount(ctx context.Context, token string, in storesyncagent.KdzsAccountInput) (*KdzsAccountDetail, error) {
	return s.CreateLocalAccount(ctx, token, in)
}

func (s *KdzsService) UpdateAccount(ctx context.Context, token, id string, in storesyncagent.KdzsAccountUpdateInput) (*KdzsAccountDetail, error) {
	return s.UpdateLocalAccount(ctx, token, id, in)
}

func (s *KdzsService) DeleteAccount(ctx context.Context, token, id string) error {
	return s.DeleteLocalAccount(ctx, token, id)
}

func (s *KdzsService) SetDefaultAccount(ctx context.Context, token, accountID string) error {
	return s.SetLocalDefault(ctx, token, accountID)
}

func (s *KdzsService) SwitchAccount(ctx context.Context, token, accountID string) error {
	return s.SwitchLocalAccount(ctx, token, accountID)
}

func (s *KdzsService) GetBatchPrintURL(ctx context.Context, token, platform string) (json.RawMessage, error) {
	if err := s.EnsureActiveSSASession(ctx, token); err != nil {
		return nil, err
	}
	return s.proxy(ctx, token, func(c *storesyncagent.Client, t string) (json.RawMessage, error) {
		return c.GetBatchPrintURL(ctx, t, platform)
	})
}

func (s *KdzsService) QueryPrintWaybills(ctx context.Context, token string, body storesyncagent.PrintWaybillQueryRequest) (json.RawMessage, error) {
	if err := s.EnsureActiveSSASession(ctx, token); err != nil {
		return nil, err
	}
	return s.proxy(ctx, token, func(c *storesyncagent.Client, t string) (json.RawMessage, error) {
		return c.QueryPrintWaybills(ctx, t, body)
	})
}

func (s *KdzsService) ListExpressTemplates(page, pageSize int, platform string) ([]model.ExpressTemplate, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	q := s.db().Model(&model.ExpressTemplate{}).Where("source = ?", model.SourceKdzs)
	if p := strings.TrimSpace(platform); p != "" {
		q = q.Where("platform = ?", p)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.ExpressTemplate
	offset := (page - 1) * pageSize
	if err := q.Order("id DESC").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (s *KdzsService) ListWaybillAuths(page, pageSize int) ([]model.WaybillAuth, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	q := s.db().Model(&model.WaybillAuth{}).Where("source = ?", model.SourceKdzs)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.WaybillAuth
	offset := (page - 1) * pageSize
	if err := q.Order("id DESC").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

type syncListPayload struct {
	Items []json.RawMessage `json:"items"`
	Total int               `json:"total"`
}

type elecAuthItem struct {
	Platform    string          `json:"platform"`
	ShopName    string          `json:"shopName"`
	AccountName string          `json:"accountName"`
	AuthStatus  string          `json:"authStatus"`
	Raw         json.RawMessage `json:"raw"`
}

type expressTemplateItem struct {
	TemplateID   string          `json:"templateId"`
	TemplateName string          `json:"templateName"`
	Platform     string          `json:"platform"`
	CarrierCode  string          `json:"carrierCode"`
	CarrierName  string          `json:"carrierName"`
	ShopID       string          `json:"shopId"`
	ShopName     string          `json:"shopName"`
	Raw          json.RawMessage `json:"raw"`
}

func (s *KdzsService) activeKdzsAccountLabel() (code, name string) {
	st, err := s.getOrCreateSettings()
	if err != nil || st == nil {
		return "", ""
	}
	code = strings.TrimSpace(st.ActiveAccountCode)
	if code == "" {
		code = strings.TrimSpace(st.DefaultAccountCode)
	}
	if code == "" {
		return "", ""
	}
	var rec model.KdzsAccount
	if err := s.db().Where("code = ?", code).First(&rec).Error; err == nil {
		name = strings.TrimSpace(rec.Name)
		if name == "" {
			name = strings.TrimSpace(rec.Mobile)
		}
	}
	if name == "" {
		name = code
	}
	return code, name
}

func inferTemplatePlatform(name, platform string) string {
	platform = strings.TrimSpace(platform)
	if platform != "" {
		return platform
	}
	n := strings.TrimSpace(name)
	switch {
	case strings.Contains(n, "抖音"), strings.Contains(n, "抖店"):
		return "抖店"
	case strings.Contains(n, "菜鸟"), strings.Contains(n, "淘宝"):
		return "菜鸟"
	case strings.Contains(n, "拼多多"):
		return "拼多多"
	case strings.Contains(n, "快手"):
		return "快手小店"
	case strings.Contains(n, "小红书"):
		return "小红书"
	case strings.Contains(n, "京东"):
		return "京东"
	case strings.Contains(n, "视频号"):
		return "视频号"
	default:
		return ""
	}
}

func (s *KdzsService) SyncPrintAssets(ctx context.Context, token string) (map[string]int, error) {
	if s.ssAgent == nil {
		return nil, fmt.Errorf("storesyncagent 未配置")
	}
	if err := s.EnsureActiveSSASession(ctx, token); err != nil {
		return nil, err
	}
	kdzsCode, kdzsName := s.activeKdzsAccountLabel()
	now := time.Now()
	stats := map[string]int{
		"auths": 0, "templates": 0,
		"authsDeleted": 0, "templatesDeleted": 0,
	}

	authRaw, err := s.ssAgent.ListElecAuth(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("拉取面单授权: %w", err)
	}
	var authPayload syncListPayload
	authSeen := map[string]struct{}{}
	authListOK := false
	if err := json.Unmarshal(authRaw, &authPayload); err == nil {
		authListOK = true
		for _, raw := range authPayload.Items {
			var item elecAuthItem
			if err := json.Unmarshal(raw, &item); err != nil {
				continue
			}
			rawJSON := string(raw)
			if item.Raw != nil {
				rawJSON = string(item.Raw)
			}
			rec := model.WaybillAuth{
				TenantID:        s.tenantID,
				Source:          model.SourceKdzs,
				KdzsAccountCode: kdzsCode,
				KdzsAccountName: kdzsName,
				Platform:        strings.TrimSpace(item.Platform),
				AccountName:     strings.TrimSpace(item.AccountName),
				ShopName:        strings.TrimSpace(item.ShopName),
				AuthStatus:      strings.TrimSpace(item.AuthStatus),
				RawJSON:         rawJSON,
				SyncedAt:        now,
			}
			key := waybillAuthKey(rec.Platform, rec.AccountName, rec.ShopName)
			authSeen[key] = struct{}{}
			if err := s.upsertWaybillAuth(&rec); err == nil {
				stats["auths"]++
			}
		}
		if n, err := s.reconcileWaybillAuths(kdzsCode, authSeen); err == nil {
			stats["authsDeleted"] = n
		}
	}

	tplRaw, err := s.ssAgent.ListExpressTemplates(ctx, token)
	if err != nil {
		// 模板接口偶发失败时仍保留已同步的面单授权，且不删除本地模板
		if authListOK || stats["auths"] > 0 {
			return stats, nil
		}
		return stats, fmt.Errorf("拉取快递模板: %w", err)
	}
	var tplPayload syncListPayload
	if err := json.Unmarshal(tplRaw, &tplPayload); err == nil {
		tplSeen := map[string]struct{}{}
		for _, raw := range tplPayload.Items {
			var item expressTemplateItem
			if err := json.Unmarshal(raw, &item); err != nil {
				continue
			}
			rawJSON := string(raw)
			if item.Raw != nil {
				rawJSON = string(item.Raw)
			}
			tplName := strings.TrimSpace(item.TemplateName)
			rec := model.ExpressTemplate{
				TenantID:        s.tenantID,
				Source:          model.SourceKdzs,
				KdzsAccountCode: kdzsCode,
				KdzsAccountName: kdzsName,
				Platform:        inferTemplatePlatform(tplName, item.Platform),
				TemplateID:      strings.TrimSpace(item.TemplateID),
				TemplateName:    tplName,
				CarrierCode:     strings.TrimSpace(item.CarrierCode),
				CarrierName:     strings.TrimSpace(item.CarrierName),
				ShopID:          strings.TrimSpace(item.ShopID),
				ShopName:        strings.TrimSpace(item.ShopName),
				Enabled:         true,
				RawJSON:         rawJSON,
				SyncedAt:        now,
			}
			if rec.TemplateID == "" {
				// 兜底：用名称+承运商生成稳定 ID，避免空 ID 跳过
				rec.TemplateID = strings.TrimSpace(rec.TemplateName + "|" + rec.CarrierCode + "|" + rec.ShopID)
			}
			if rec.TemplateID == "" || rec.TemplateID == "||" {
				continue
			}
			tplSeen[rec.TemplateID] = struct{}{}
			if err := s.upsertExpressTemplate(&rec); err == nil {
				stats["templates"]++
			}
		}
		if n, err := s.reconcileExpressTemplates(kdzsCode, tplSeen); err == nil {
			stats["templatesDeleted"] = n
		}
	}

	return stats, nil
}

func waybillAuthKey(platform, accountName, shopName string) string {
	return strings.TrimSpace(platform) + "\x00" + strings.TrimSpace(accountName) + "\x00" + strings.TrimSpace(shopName)
}

// reconcileExpressTemplates 删除当前账号下远端已不存在的本地模板，与快递助手保持一致。
func (s *KdzsService) reconcileExpressTemplates(kdzsCode string, seen map[string]struct{}) (int, error) {
	q := s.db().Model(&model.ExpressTemplate{}).
		Where("source = ?", model.SourceKdzs)
	if kdzsCode != "" {
		q = q.Where("kdzs_account_code = ?", kdzsCode)
	} else {
		q = q.Where("kdzs_account_code = '' OR kdzs_account_code IS NULL")
	}
	var locals []model.ExpressTemplate
	if err := q.Select("id", "template_id").Find(&locals).Error; err != nil {
		return 0, err
	}
	deleted := 0
	for _, loc := range locals {
		if _, ok := seen[loc.TemplateID]; ok {
			continue
		}
		if err := s.db().Delete(&model.ExpressTemplate{}, loc.ID).Error; err == nil {
			deleted++
		}
	}
	return deleted, nil
}

// reconcileWaybillAuths 删除当前账号下远端已不存在的本地面单授权。
func (s *KdzsService) reconcileWaybillAuths(kdzsCode string, seen map[string]struct{}) (int, error) {
	q := s.db().Model(&model.WaybillAuth{}).
		Where("source = ?", model.SourceKdzs)
	if kdzsCode != "" {
		q = q.Where("kdzs_account_code = ?", kdzsCode)
	} else {
		q = q.Where("kdzs_account_code = '' OR kdzs_account_code IS NULL")
	}
	var locals []model.WaybillAuth
	if err := q.Select("id", "platform", "account_name", "shop_name").Find(&locals).Error; err != nil {
		return 0, err
	}
	deleted := 0
	for _, loc := range locals {
		key := waybillAuthKey(loc.Platform, loc.AccountName, loc.ShopName)
		if _, ok := seen[key]; ok {
			continue
		}
		if err := s.db().Delete(&model.WaybillAuth{}, loc.ID).Error; err == nil {
			deleted++
		}
	}
	return deleted, nil
}

func (s *KdzsService) upsertWaybillAuth(rec *model.WaybillAuth) error {
	var existing model.WaybillAuth
	base := s.db().Where(
		"source = ? AND platform = ? AND account_name = ? AND shop_name = ?",
		rec.Source, rec.Platform, rec.AccountName, rec.ShopName,
	)
	err := base.Where("kdzs_account_code = ?", rec.KdzsAccountCode).First(&existing).Error
	if errorsIsNotFound(err) && rec.KdzsAccountCode != "" {
		// 兼容旧数据：无来源账号字段时就地回填
		err = base.Where("kdzs_account_code = '' OR kdzs_account_code IS NULL").First(&existing).Error
	}
	if err != nil {
		if errorsIsNotFound(err) {
			return s.db().Create(rec).Error
		}
		return err
	}
	rec.ID = existing.ID
	rec.CreatedAt = existing.CreatedAt
	return s.db().Save(rec).Error
}

func (s *KdzsService) upsertExpressTemplate(rec *model.ExpressTemplate) error {
	var existing model.ExpressTemplate
	base := s.db().Where("source = ? AND template_id = ?", rec.Source, rec.TemplateID)
	err := base.Where("kdzs_account_code = ?", rec.KdzsAccountCode).First(&existing).Error
	if errorsIsNotFound(err) && rec.KdzsAccountCode != "" {
		err = base.Where("kdzs_account_code = '' OR kdzs_account_code IS NULL").First(&existing).Error
	}
	if err != nil {
		if errorsIsNotFound(err) {
			return s.db().Create(rec).Error
		}
		return err
	}
	rec.ID = existing.ID
	rec.CreatedAt = existing.CreatedAt
	rec.Enabled = existing.Enabled
	return s.db().Save(rec).Error
}
