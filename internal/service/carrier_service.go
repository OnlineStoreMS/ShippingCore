package service

import (
	"fmt"
	"strings"

	"shippingcore/internal/dto"
	"shippingcore/internal/model"
	"shippingcore/internal/repo"

	"gorm.io/gorm"
)

type CarrierService struct {
	repos    *repo.Repos
	tenantID uint64
}

func NewCarrierService(repos *repo.Repos) *CarrierService {
	return &CarrierService{repos: repos}
}

func (s *CarrierService) ForTenant(tenantID uint64) *CarrierService {
	return &CarrierService{repos: s.repos, tenantID: repo.NormalizeTenantID(tenantID)}
}

func (s *CarrierService) db() *gorm.DB {
	return s.repos.ForTenant(s.tenantID)
}

func MaskCheckword(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if len(v) <= 4 {
		return "****"
	}
	return "****" + v[len(v)-4:]
}

func (s *CarrierService) maskAccount(item *model.CarrierAccount) *model.CarrierAccount {
	if item == nil {
		return nil
	}
	copy := *item
	copy.Checkword = MaskCheckword(copy.Checkword)
	return &copy
}

func (s *CarrierService) List(keyword string, page, pageSize int) ([]model.CarrierAccount, int64, error) {
	q := s.db().Model(&model.CarrierAccount{})
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + strings.ToLower(keyword) + "%"
		q = q.Where("LOWER(name) LIKE ? OR LOWER(partner_id) LIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.CarrierAccount
	offset := (page - 1) * pageSize
	if err := q.Order("id DESC").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	for i := range list {
		list[i].Checkword = MaskCheckword(list[i].Checkword)
	}
	return list, total, nil
}

func (s *CarrierService) Get(id uint64) (*model.CarrierAccount, error) {
	var item model.CarrierAccount
	if err := s.db().First(&item, id).Error; err != nil {
		if errorsIsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s.maskAccount(&item), nil
}

func (s *CarrierService) GetRaw(id uint64) (*model.CarrierAccount, error) {
	var item model.CarrierAccount
	if err := s.db().First(&item, id).Error; err != nil {
		if errorsIsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (s *CarrierService) Create(in *dto.CarrierAccountDTO) (*model.CarrierAccount, error) {
	if in == nil || strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.PartnerID) == "" || strings.TrimSpace(in.Checkword) == "" {
		return nil, ErrBadRequest
	}
	item := dtoToCarrierAccount(in)
	item.TenantID = s.tenantID
	if item.CarrierCode == "" {
		item.CarrierCode = model.CarrierCodeSF
	}
	if item.ExpressType == "" {
		item.ExpressType = "2"
	}
	if item.Env == "" {
		item.Env = model.CarrierEnvSandbox
	}
	if err := s.ensureUniqueName(item.Name, 0); err != nil {
		return nil, err
	}
	if err := s.db().Create(&item).Error; err != nil {
		return nil, err
	}
	return s.maskAccount(&item), nil
}

func (s *CarrierService) Update(id uint64, in *dto.CarrierAccountDTO) (*model.CarrierAccount, error) {
	item, err := s.GetRaw(id)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, ErrBadRequest
	}
	if name := strings.TrimSpace(in.Name); name != "" {
		if err := s.ensureUniqueName(name, id); err != nil {
			return nil, err
		}
		item.Name = name
	}
	if in.PartnerID != "" {
		item.PartnerID = in.PartnerID
	}
	if strings.TrimSpace(in.Checkword) != "" && !strings.HasPrefix(in.Checkword, "****") {
		item.Checkword = in.Checkword
	}
	item.UseMonthly = in.UseMonthly
	item.CustID = in.CustID
	if in.ExpressType != "" {
		item.ExpressType = in.ExpressType
	}
	if in.Env != "" {
		item.Env = in.Env
	}
	if in.CarrierCode != "" {
		item.CarrierCode = in.CarrierCode
	}
	item.Enabled = in.Enabled
	item.Remark = in.Remark
	if err := s.db().Save(item).Error; err != nil {
		return nil, err
	}
	return s.maskAccount(item), nil
}

func (s *CarrierService) Delete(id uint64) error {
	res := s.db().Delete(&model.CarrierAccount{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *CarrierService) ensureUniqueName(name string, excludeID uint64) error {
	q := s.db().Model(&model.CarrierAccount{}).Where("name = ?", name)
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrDuplicateCode
	}
	return nil
}

func dtoToCarrierAccount(in *dto.CarrierAccountDTO) model.CarrierAccount {
	return model.CarrierAccount{
		CarrierCode: in.CarrierCode,
		Name:        strings.TrimSpace(in.Name),
		PartnerID:   strings.TrimSpace(in.PartnerID),
		Checkword:   strings.TrimSpace(in.Checkword),
		UseMonthly:  in.UseMonthly,
		CustID:      strings.TrimSpace(in.CustID),
		ExpressType: in.ExpressType,
		Env:         in.Env,
		Enabled:     in.Enabled,
		Remark:      in.Remark,
	}
}

func errorsIsNotFound(err error) bool {
	return err == gorm.ErrRecordNotFound
}

func shipmentOrderID(shipmentID uint64) string {
	return fmt.Sprintf("SC%d", shipmentID)
}
