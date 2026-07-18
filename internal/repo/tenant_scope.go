package repo

import "gorm.io/gorm"

func scopeTenant(tenantID uint64) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("tenant_id = ?", tenantID)
	}
}

func NormalizeTenantID(tenantID uint64) uint64 {
	if tenantID == 0 {
		return 1
	}
	return tenantID
}
