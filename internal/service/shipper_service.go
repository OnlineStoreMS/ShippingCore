package service

import (
	"strings"

	"shippingcore/internal/dto"
	"shippingcore/internal/model"
	"shippingcore/internal/repo"

	"gorm.io/gorm"
)

type ShipperService struct {
	repos    *repo.Repos
	tenantID uint64
}

func NewShipperService(repos *repo.Repos) *ShipperService {
	return &ShipperService{repos: repos}
}

func (s *ShipperService) ForTenant(tenantID uint64) *ShipperService {
	return &ShipperService{repos: s.repos, tenantID: repo.NormalizeTenantID(tenantID)}
}

func (s *ShipperService) db() *gorm.DB {
	return s.repos.ForTenant(s.tenantID)
}

func (s *ShipperService) List(keyword string, page, pageSize int) ([]model.ShipperProfile, int64, error) {
	q := s.db().Model(&model.ShipperProfile{})
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + strings.ToLower(keyword) + "%"
		q = q.Where("LOWER(name) LIKE ? OR LOWER(mobile) LIKE ? OR LOWER(company) LIKE ?", like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.ShipperProfile
	offset := (page - 1) * pageSize
	if err := q.Order("is_default DESC, id DESC").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (s *ShipperService) Get(id uint64) (*model.ShipperProfile, error) {
	var item model.ShipperProfile
	if err := s.db().First(&item, id).Error; err != nil {
		if errorsIsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (s *ShipperService) Create(in *dto.ShipperProfileDTO) (*model.ShipperProfile, error) {
	if in == nil || strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Mobile) == "" || strings.TrimSpace(in.Address) == "" {
		return nil, ErrBadRequest
	}
	item := dtoToShipperProfile(in)
	item.TenantID = s.tenantID
	return s.saveWithDefault(&item, in.IsDefault)
}

func (s *ShipperService) Update(id uint64, in *dto.ShipperProfileDTO) (*model.ShipperProfile, error) {
	item, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, ErrBadRequest
	}
	if name := strings.TrimSpace(in.Name); name != "" {
		item.Name = name
	}
	if mobile := strings.TrimSpace(in.Mobile); mobile != "" {
		item.Mobile = mobile
	}
	item.Company = in.Company
	item.Province = in.Province
	item.City = in.City
	item.County = in.County
	if addr := strings.TrimSpace(in.Address); addr != "" {
		item.Address = addr
	}
	item.Enabled = in.Enabled
	return s.saveWithDefault(item, in.IsDefault)
}

func (s *ShipperService) Delete(id uint64) error {
	res := s.db().Delete(&model.ShipperProfile{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *ShipperService) SetDefault(id uint64) (*model.ShipperProfile, error) {
	item, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if err := s.db().Model(&model.ShipperProfile{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
		return nil, err
	}
	item.IsDefault = true
	if err := s.db().Save(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (s *ShipperService) saveWithDefault(item *model.ShipperProfile, isDefault bool) (*model.ShipperProfile, error) {
	if isDefault {
		if err := s.db().Model(&model.ShipperProfile{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
			return nil, err
		}
		item.IsDefault = true
	}
	var err error
	if item.ID == 0 {
		err = s.db().Create(item).Error
	} else {
		err = s.db().Save(item).Error
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

func dtoToShipperProfile(in *dto.ShipperProfileDTO) model.ShipperProfile {
	return model.ShipperProfile{
		Name:      strings.TrimSpace(in.Name),
		Company:   strings.TrimSpace(in.Company),
		Mobile:    strings.TrimSpace(in.Mobile),
		Province:  strings.TrimSpace(in.Province),
		City:      strings.TrimSpace(in.City),
		County:    strings.TrimSpace(in.County),
		Address:   strings.TrimSpace(in.Address),
		IsDefault: in.IsDefault,
		Enabled:   in.Enabled,
	}
}
