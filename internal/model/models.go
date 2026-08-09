package model

import "time"

const (
	CarrierCodeSF = "SF"

	ShipmentStatusDraft     = "draft"
	ShipmentStatusCreated   = "created"
	ShipmentStatusPrinted   = "printed"
	ShipmentStatusCancelled = "cancelled"
	ShipmentStatusFailed    = "failed"

	SourceSystemStoreSyncAgent = "storesyncagent"
	SourceSystemOrderCore      = "ordercore"

	SourceKdzs = "kdzs"

	CarrierEnvSandbox = "sandbox"
	CarrierEnvProd    = "prod"
)

type CarrierAccount struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	TenantID    uint64    `gorm:"index;not null" json:"tenantId"`
	CarrierCode string    `gorm:"size:32;not null;default:SF" json:"carrierCode"`
	Name        string    `gorm:"size:128;not null" json:"name"`
	PartnerID   string    `gorm:"size:64;not null" json:"partnerId"`
	Checkword   string    `gorm:"size:128;not null" json:"checkword,omitempty"`
	UseMonthly  bool      `gorm:"default:false" json:"useMonthly"`
	CustID      string    `gorm:"size:64" json:"custId"`
	ExpressType string    `gorm:"size:16;default:2" json:"expressType"`
	Env         string    `gorm:"size:16;default:sandbox" json:"env"`
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	Remark      string    `gorm:"size:512" json:"remark"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (CarrierAccount) TableName() string { return "carrier_accounts" }

type ShipperProfile struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	TenantID  uint64    `gorm:"index;not null" json:"tenantId"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Company   string    `gorm:"size:256" json:"company"`
	Mobile    string    `gorm:"size:32;not null" json:"mobile"`
	Province  string    `gorm:"size:64" json:"province"`
	City      string    `gorm:"size:64" json:"city"`
	County    string    `gorm:"size:64" json:"county"`
	Address   string    `gorm:"size:512;not null" json:"address"`
	IsDefault bool      `gorm:"default:false" json:"isDefault"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (ShipperProfile) TableName() string { return "shipper_profiles" }

type Shipment struct {
	ID               uint64    `gorm:"primaryKey" json:"id"`
	TenantID         uint64    `gorm:"index;not null" json:"tenantId"`
	SourceSystem     string    `gorm:"size:64;not null" json:"sourceSystem"`
	SourceRef        string    `gorm:"size:128;index" json:"sourceRef"`
	SourceTid        string    `gorm:"size:128" json:"sourceTid"`
	Platform         string    `gorm:"size:64" json:"platform"`
	ShopID           string    `gorm:"size:64" json:"shopId"`
	CarrierAccountID uint64    `gorm:"index" json:"carrierAccountId"`
	ShipperProfileID uint64    `gorm:"index" json:"shipperProfileId"`

	ReceiverName     string `gorm:"size:128" json:"receiverName"`
	ReceiverMobile   string `gorm:"size:32" json:"receiverMobile"`
	ReceiverProvince string `gorm:"size:64" json:"receiverProvince"`
	ReceiverCity     string `gorm:"size:64" json:"receiverCity"`
	ReceiverCounty   string `gorm:"size:64" json:"receiverCounty"`
	ReceiverAddress  string `gorm:"size:512" json:"receiverAddress"`

	ShipperName     string `gorm:"size:128" json:"shipperName"`
	ShipperMobile   string `gorm:"size:32" json:"shipperMobile"`
	ShipperProvince string `gorm:"size:64" json:"shipperProvince"`
	ShipperCity     string `gorm:"size:64" json:"shipperCity"`
	ShipperCounty   string `gorm:"size:64" json:"shipperCounty"`
	ShipperAddress  string `gorm:"size:512" json:"shipperAddress"`
	ShipperCompany  string `gorm:"size:256" json:"shipperCompany"`

	UseMonthly  bool   `gorm:"default:false" json:"useMonthly"`
	PayMethod   int    `gorm:"default:1" json:"payMethod"`
	CustID      string `gorm:"size:64" json:"custId"`
	ExpressType string `gorm:"size:16;default:2" json:"expressType"`

	OrderCoreOrderID uint64 `gorm:"index" json:"orderCoreOrderId,omitempty"`

	MailNo     string `gorm:"size:64;index" json:"mailNo"`
	SFOrderID  string `gorm:"size:128" json:"sfOrderId"`
	LabelURL   string `gorm:"size:1024" json:"labelUrl"`
	LabelData  string `gorm:"type:text" json:"labelData,omitempty"`
	Status     string `gorm:"size:32;index;default:draft" json:"status"`
	ErrorMessage string `gorm:"size:1024" json:"errorMessage,omitempty"`

	CargoName string `gorm:"size:256" json:"cargoName"`
	ParcelQty int    `gorm:"default:1" json:"parcelQty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	Items []ShipmentItem `gorm:"foreignKey:ShipmentID" json:"items,omitempty"`
}

func (Shipment) TableName() string { return "shipments" }

type ShipmentItem struct {
	ID         uint64 `gorm:"primaryKey" json:"id"`
	ShipmentID uint64 `gorm:"index;not null" json:"shipmentId"`
	GoodsName  string `gorm:"size:256" json:"goodsName"`
	Quantity   int    `gorm:"default:1" json:"quantity"`
	SkuCode    string `gorm:"size:128" json:"skuCode"`
	OuterID    string `gorm:"size:128" json:"outerId"`
}

func (ShipmentItem) TableName() string { return "shipment_items" }

type ExpressTemplate struct {
	ID              uint64    `gorm:"primaryKey" json:"id"`
	TenantID        uint64    `gorm:"index;not null" json:"tenantId"`
	Source          string    `gorm:"size:32;not null;default:kdzs" json:"source"`
	KdzsAccountCode string    `gorm:"size:64;index" json:"kdzsAccountCode"`
	KdzsAccountName string    `gorm:"size:128" json:"kdzsAccountName"`
	Platform        string    `gorm:"size:32;index" json:"platform"`
	TemplateID      string    `gorm:"size:128;index" json:"templateId"`
	TemplateName    string    `gorm:"size:256" json:"templateName"`
	CarrierCode     string    `gorm:"size:64" json:"carrierCode"`
	CarrierName     string    `gorm:"size:128" json:"carrierName"`
	ShopID          string    `gorm:"size:64" json:"shopId"`
	ShopName        string    `gorm:"size:128" json:"shopName"`
	Enabled         bool      `gorm:"default:true" json:"enabled"`
	RawJSON         string    `gorm:"type:text" json:"rawJson,omitempty"`
	SyncedAt        time.Time `json:"syncedAt"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func (ExpressTemplate) TableName() string { return "express_templates" }

type WaybillAuth struct {
	ID              uint64    `gorm:"primaryKey" json:"id"`
	TenantID        uint64    `gorm:"index;not null" json:"tenantId"`
	Source          string    `gorm:"size:32;not null;default:kdzs" json:"source"`
	KdzsAccountCode string    `gorm:"size:64;index" json:"kdzsAccountCode"`
	KdzsAccountName string    `gorm:"size:128" json:"kdzsAccountName"`
	Platform        string    `gorm:"size:32;index" json:"platform"`
	AccountName     string    `gorm:"size:128" json:"accountName"`
	ShopName        string    `gorm:"size:128" json:"shopName"`
	AuthStatus      string    `gorm:"size:64" json:"authStatus"`
	Detail          string    `gorm:"size:512" json:"detail"`
	RawJSON         string    `gorm:"type:text" json:"rawJson,omitempty"`
	SyncedAt        time.Time `json:"syncedAt"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func (WaybillAuth) TableName() string { return "waybill_auths" }

const (
	KdzsAccountSourceSSA   = "ssa"
	KdzsAccountSourceLocal = "local"
)

// KdzsAccount 发货中心本地快递助手账号（默认同步自 StoreSyncAgent，也可本地独立维护）。
type KdzsAccount struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	TenantID  uint64    `gorm:"not null;uniqueIndex:idx_sc_kdzs_acc_tenant_code,priority:1" json:"tenantId"`
	Code      string    `gorm:"size:64;not null;uniqueIndex:idx_sc_kdzs_acc_tenant_code,priority:2" json:"code"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Role      string    `gorm:"size:32;not null;default:merchant" json:"role"`
	Mobile    string    `gorm:"size:32;not null" json:"mobile"`
	Password  string    `gorm:"size:256;not null" json:"-"`
	SortOrder int       `gorm:"not null;default:0" json:"sortOrder"`
	Enabled   bool      `gorm:"not null;default:true" json:"enabled"`
	Source    string    `gorm:"size:16;not null;default:ssa" json:"source"` // ssa | local
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (KdzsAccount) TableName() string { return "kdzs_accounts" }

// KdzsSetting 发货中心快递助手账号偏好（可与 StoreSyncAgent 不一致）。
type KdzsSetting struct {
	ID                 uint64    `gorm:"primaryKey" json:"id"`
	TenantID           uint64    `gorm:"uniqueIndex;not null" json:"tenantId"`
	DefaultAccountCode string    `gorm:"size:64" json:"defaultAccountCode"`
	ActiveAccountCode  string    `gorm:"size:64" json:"activeAccountCode"`
	AutoSyncFromSSA    bool      `gorm:"not null;default:true" json:"autoSyncFromSSA"`
	LastSyncedAt       *time.Time `json:"lastSyncedAt,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

func (KdzsSetting) TableName() string { return "kdzs_settings" }
