package database

import (
	"fmt"
	"os"
	"path/filepath"

	"shippingcore/internal/config"
	"shippingcore/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(cfg *config.DatabaseConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch cfg.Driver {
	case "postgres":
		dialector = postgres.Open(cfg.PostgresDSN)
	case "sqlite":
		if err := os.MkdirAll(filepath.Dir(cfg.SQLitePath), 0o755); err != nil {
			return nil, err
		}
		dialector = sqlite.Open(cfg.SQLitePath)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	return db, nil
}

func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&model.CarrierAccount{},
		&model.ShipperProfile{},
		&model.ShipmentGroup{},
		&model.Shipment{},
		&model.ShipmentItem{},
		&model.ExpressTemplate{},
		&model.WaybillAuth{},
		&model.KdzsAccount{},
		&model.KdzsSetting{},
	); err != nil {
		return err
	}
	return ensureIndexes(db)
}

func ensureIndexes(db *gorm.DB) error {
	switch db.Dialector.Name() {
	case "postgres":
		return db.Exec(`
			CREATE UNIQUE INDEX IF NOT EXISTS idx_carrier_accounts_tenant_name ON carrier_accounts (tenant_id, name);
			CREATE INDEX IF NOT EXISTS idx_shipments_tenant_status ON shipments (tenant_id, status);
			CREATE INDEX IF NOT EXISTS idx_shipments_tenant_source_ref ON shipments (tenant_id, source_ref);
			CREATE INDEX IF NOT EXISTS idx_shipments_tenant_group ON shipments (tenant_id, group_id);
			CREATE INDEX IF NOT EXISTS idx_shipment_groups_tenant_order ON shipment_groups (tenant_id, order_core_order_id);
		`).Error
	default:
		return nil
	}
}
