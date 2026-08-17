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

	// ShipVia：发货通道（与订单来源 SourceChannel 不同）
	ShipViaSF   = "sf"   // 自建物流/丰桥取号打印
	ShipViaKdzs = "kdzs" // 推送快递助手或快递助手打单后确认发货

	CarrierEnvSandbox = "sandbox"
	CarrierEnvProd    = "prod"
)

type CarrierAccount struct {
	ID           uint64 `gorm:"primaryKey" json:"id"`
	TenantID     uint64 `gorm:"index;not null" json:"tenantId"`
	CarrierCode  string `gorm:"size:32;not null;default:SF" json:"carrierCode"`
	Name         string `gorm:"size:128;not null" json:"name"`
	PartnerID    string `gorm:"size:64;not null" json:"partnerId"`
	Checkword    string `gorm:"size:128;not null" json:"checkword,omitempty"`
	UseMonthly   bool   `gorm:"default:false" json:"useMonthly"`
	CustID       string `gorm:"size:64" json:"custId"`
	ExpressType  string `gorm:"size:16;default:2" json:"expressType"`
	TemplateCode       string `gorm:"size:128" json:"templateCode"`                 // 标准模板，如 fm_76130_standard_XXXX
	CustomTemplateCode string `gorm:"size:128" json:"customTemplateCode,omitempty"` // 自定义区模板，如 fm_76130_standard_custom_…
	SignMode           string `gorm:"size:16;default:simple" json:"signMode"`       // standard|simple|sm3，须与丰桥应用一致
	// PrintChannel 云打印通道：pdf=COM_RECE_CLOUD_PRINT_WAYBILLS；plugin=COM_RECE_CLOUD_PRINT_PARSEDDATA
	PrintChannel string `gorm:"size:16;default:pdf" json:"printChannel"`
	// PrintLogo 热敏纸无预印顺丰 Logo 时开启，云打印 documents.isPrintLogo=true
	PrintLogo bool      `gorm:"default:false" json:"printLogo"`
	Env       string    `gorm:"size:16;default:sandbox" json:"env"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	Remark    string    `gorm:"size:512" json:"remark"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
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
	ID               uint64 `gorm:"primaryKey" json:"id"`
	TenantID         uint64 `gorm:"index;not null" json:"tenantId"`
	SourceSystem     string `gorm:"size:64;not null" json:"sourceSystem"`
	SourceRef        string `gorm:"size:128;index" json:"sourceRef"`
	SourceTid        string `gorm:"size:128" json:"sourceTid"`
	// OrderNo 订单中心订单号（与待发货 orderNo 一致，如 OC202608130007）
	OrderNo          string `gorm:"size:128;index" json:"orderNo"`
	Platform         string `gorm:"size:64" json:"platform"`
	ShopID           string `gorm:"size:64" json:"shopId"`
	ShopName         string `gorm:"size:128" json:"shopName"`
	SourceChannel    string `gorm:"size:32" json:"sourceChannel"`
	ManualSourceName string `gorm:"size:128" json:"manualSourceName"`
	CarrierAccountID uint64 `gorm:"index" json:"carrierAccountId"`
	ShipperProfileID uint64 `gorm:"index" json:"shipperProfileId"`

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

	// GroupID 拆分发货主单；空表示普通单笔发货
	GroupID *uint64 `gorm:"index" json:"groupId,omitempty"`

	MailNo          string `gorm:"size:64;index" json:"mailNo"`
	// ShipVia 发货通道：sf=丰桥；kdzs=快递助手（此类单不支持本系统取消运单/云打印）
	ShipVia         string `gorm:"size:16;index" json:"shipVia,omitempty"`
	ExpressCompany  string `gorm:"size:64" json:"expressCompany,omitempty"` // 快递公司名（快递助手/手工填单等）
	SFOrderID       string `gorm:"size:128" json:"sfOrderId"`
	LabelURL        string `gorm:"size:1024" json:"labelUrl"`              // 顺丰临时面单链接（会过期）
	LabelPdfURL     string `gorm:"size:1024" json:"labelPdfUrl,omitempty"` // 发货中心永久存档 PDF
	LabelToken   string `gorm:"size:512" json:"labelToken,omitempty"`              // 云打印 PDF 下载 token
	LabelData    string `gorm:"type:text" json:"labelData,omitempty"`
	Status       string     `gorm:"size:32;index;default:draft" json:"status"`
	ErrorMessage string     `gorm:"size:1024" json:"errorMessage,omitempty"`
	// ShippedAt 首次取得运单号 / 确认发货时间（不因再次打印改动）；快递助手单优先对齐订单中心/KDZS
	ShippedAt *time.Time `json:"shippedAt,omitempty"`
	// PrintedAt 本系统最近一次成功打印时间；快递助手打单不写此字段
	PrintedAt *time.Time `json:"printedAt,omitempty"`

	CargoName   string  `gorm:"size:256" json:"cargoName"`
	ParcelQty   int     `gorm:"default:1" json:"parcelQty"`
	CargoCount  int     `gorm:"default:1" json:"cargoCount"` // 总包裹物品数
	Remark      string  `gorm:"size:512" json:"remark,omitempty"`
	CourierNote string  `gorm:"size:128" json:"courierNote,omitempty"` // 给快递员捎话，不印面单
	RemarkImages string `gorm:"type:text" json:"remarkImages,omitempty"` // JSON 图片 URL 列表
	TotalWeight float64 `gorm:"type:numeric(10,3);default:0" json:"totalWeight,omitempty"` // kg
	LengthCM    float64 `gorm:"type:numeric(10,2);default:0" json:"lengthCm,omitempty"`
	WidthCM     float64 `gorm:"type:numeric(10,2);default:0" json:"widthCm,omitempty"`
	HeightCM    float64 `gorm:"type:numeric(10,2);default:0" json:"heightCm,omitempty"`
	TotalVolume float64 `gorm:"type:numeric(12,6);default:0" json:"totalVolume,omitempty"` // m³
	PickupMode  string `gorm:"size:16;default:self" json:"pickupMode,omitempty"` // self | appoint
	// SendStartTm 预约上门取件开始时间（丰桥 sendStartTm），格式 2006-01-02 15:04:05
	SendStartTm string `gorm:"size:32" json:"sendStartTm,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	Items []ShipmentItem `gorm:"foreignKey:ShipmentID" json:"items,omitempty"`
}

func (Shipment) TableName() string { return "shipments" }

type ShipmentItem struct {
	ID          uint64 `gorm:"primaryKey" json:"id"`
	ShipmentID  uint64 `gorm:"index;not null" json:"shipmentId"`
	OrderItemID uint64 `gorm:"index;default:0" json:"orderItemId"`
	GoodsName   string `gorm:"size:256" json:"goodsName"`
	Quantity    int    `gorm:"default:1" json:"quantity"`
	SkuCode     string `gorm:"size:128" json:"skuCode"`
	OuterID     string `gorm:"size:128" json:"outerId"`
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
	ID                 uint64     `gorm:"primaryKey" json:"id"`
	TenantID           uint64     `gorm:"uniqueIndex;not null" json:"tenantId"`
	DefaultAccountCode string     `gorm:"size:64" json:"defaultAccountCode"`
	ActiveAccountCode  string     `gorm:"size:64" json:"activeAccountCode"`
	AutoSyncFromSSA    bool       `gorm:"not null;default:true" json:"autoSyncFromSSA"`
	LastSyncedAt       *time.Time `json:"lastSyncedAt,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

func (KdzsSetting) TableName() string { return "kdzs_settings" }

// ShipmentGroup 拆分发货主单：一次拆分确认归组，组内每运单仍是独立 shipments 行。
type ShipmentGroup struct {
	ID               uint64    `gorm:"primaryKey" json:"id"`
	TenantID         uint64    `gorm:"index;not null" json:"tenantId"`
	OrderCoreOrderID uint64    `gorm:"index" json:"orderCoreOrderId,omitempty"`
	OrderNo          string    `gorm:"size:128;index" json:"orderNo"`
	SourceRef        string    `gorm:"size:128;index" json:"sourceRef"`
	ShipVia          string    `gorm:"size:16;index" json:"shipVia,omitempty"`
	Status           string    `gorm:"size:32;index;default:printed" json:"status"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`

	Shipments []Shipment `gorm:"foreignKey:GroupID" json:"shipments,omitempty"`
}

func (ShipmentGroup) TableName() string { return "shipment_groups" }

const (
	ShipPlanStatusPending = "pending"
	ShipPlanStatusShipped = "shipped"
)

// ShipPlanLine 待发货拆分计划行：保存后打单时勾选发货。
// OrderItemID>0：按商品拆分，替换该原商品行；OrderItemID=0：整单拆分，打单只认计划行。
type ShipPlanLine struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	TenantID      uint64    `gorm:"index;not null" json:"tenantId"`
	OrderCoreID   uint64    `gorm:"index;not null;column:order_core_id" json:"orderCoreId"`
	OrderItemID   uint64    `gorm:"index;not null" json:"orderItemId"`
	SkuName       string    `gorm:"size:256;not null" json:"skuName"`
	Qty           int       `gorm:"not null;default:1" json:"qty"`
	SortNo        int       `gorm:"not null;default:0" json:"sortNo"`
	Status        string    `gorm:"size:16;index;not null;default:pending" json:"status"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func (ShipPlanLine) TableName() string { return "ship_plan_lines" }
