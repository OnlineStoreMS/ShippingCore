package repo

import "gorm.io/gorm"

type Repos struct {
	DB *gorm.DB
}

func New(db *gorm.DB) *Repos {
	return &Repos{DB: db}
}

func (r *Repos) ForTenant(tenantID uint64) *gorm.DB {
	return r.DB.Scopes(scopeTenant(NormalizeTenantID(tenantID)))
}
