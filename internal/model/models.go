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
