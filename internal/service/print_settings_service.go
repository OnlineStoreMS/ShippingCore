package service

import (
	"strings"

	"shippingcore/internal/model"
	"shippingcore/internal/repo"

	"gorm.io/gorm"
)

type PrintSettingsService struct {
	repos    *repo.Repos
	tenantID uint64
}

func NewPrintSettingsService(repos *repo.Repos) *PrintSettingsService {
	return &PrintSettingsService{repos: repos}
}

func (s *PrintSettingsService) ForTenant(tenantID uint64) *PrintSettingsService {
	cp := *s
	cp.tenantID = tenantID
	return &cp
}

func (s *PrintSettingsService) db() *gorm.DB {
	return s.repos.ForTenant(s.tenantID)
}

type PrintSettingsDTO struct {
	SfPrinterName   string `json:"sfPrinterName"`
	KdzsPrinterName string `json:"kdzsPrinterName"`
}

type UpdatePrintSettingsInput struct {
	SfPrinterName   *string `json:"sfPrinterName"`
	KdzsPrinterName *string `json:"kdzsPrinterName"`
}

func (s *PrintSettingsService) getOrCreate() (*model.PrintSetting, error) {
	var st model.PrintSetting
	err := s.db().Where("tenant_id = ?", s.tenantID).First(&st).Error
	if err == nil {
		return &st, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}
	st = model.PrintSetting{TenantID: s.tenantID}
	if err := s.repos.DB.Create(&st).Error; err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *PrintSettingsService) Get() (*PrintSettingsDTO, error) {
	st, err := s.getOrCreate()
	if err != nil {
		return nil, err
	}
	return &PrintSettingsDTO{
		SfPrinterName:   strings.TrimSpace(st.SfPrinterName),
		KdzsPrinterName: strings.TrimSpace(st.KdzsPrinterName),
	}, nil
}

func (s *PrintSettingsService) Update(in *UpdatePrintSettingsInput) (*PrintSettingsDTO, error) {
	if in == nil {
		return nil, ErrBadRequest
	}
	st, err := s.getOrCreate()
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if in.SfPrinterName != nil {
		updates["sf_printer_name"] = strings.TrimSpace(*in.SfPrinterName)
	}
	if in.KdzsPrinterName != nil {
		updates["kdzs_printer_name"] = strings.TrimSpace(*in.KdzsPrinterName)
	}
	if len(updates) == 0 {
		return s.Get()
	}
	if err := s.repos.DB.Model(st).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.Get()
}

// KdzsDefaultPrinterName 租户默认远程打单打印机名（空表示未配置）。
func (s *PrintSettingsService) KdzsDefaultPrinterName() string {
	st, err := s.getOrCreate()
	if err != nil || st == nil {
		return ""
	}
	return strings.TrimSpace(st.KdzsPrinterName)
}
