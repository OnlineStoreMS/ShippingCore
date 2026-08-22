package dto

type CarrierAccountDTO struct {
	ID           uint64 `json:"id,omitempty"`
	CarrierCode  string `json:"carrierCode"`
	Name         string `json:"name"`
	PartnerID    string `json:"partnerId"`
	Checkword    string `json:"checkword,omitempty"`
	UseMonthly   bool   `json:"useMonthly"`
	CustID       string `json:"custId"`
	ExpressType  string `json:"expressType"`
	TemplateCode       string `json:"templateCode"`
	CustomTemplateCode string `json:"customTemplateCode,omitempty"`
	SignMode           string `json:"signMode"`     // standard|simple|sm3
	PrintChannel       string `json:"printChannel"` // pdf|plugin
	PrintLogo          bool   `json:"printLogo"`   // 热敏纸无预印 Logo 时打印顺丰 Logo
	Env          string `json:"env"`
	Enabled      bool   `json:"enabled"`
	Remark       string `json:"remark"`
}

type ShipperProfileDTO struct {
	ID        uint64 `json:"id,omitempty"`
	Name      string `json:"name"`
	Company   string `json:"company"`
	Mobile    string `json:"mobile"`
	Province  string `json:"province"`
	City      string `json:"city"`
	County    string `json:"county"`
	Address   string `json:"address"`
	IsDefault bool   `json:"isDefault"`
	Enabled   bool   `json:"enabled"`
}

// CheckPickupTimeRequest 按寄件地址查顺丰可约上门时间窗。
type CheckPickupTimeRequest struct {
	CarrierAccountID uint64 `json:"carrierAccountId"`
	Province         string `json:"province"`
	City             string `json:"city"`
	County           string `json:"county"`
	Address          string `json:"address"`
	CityCode         string `json:"cityCode,omitempty"`
}

type PickupAppointSlotOption struct {
	Value       string `json:"value"` // dayOffset|slotKey
	Text        string `json:"text"`
	SlotKey     string `json:"slotKey"`
	SendStartTm string `json:"sendStartTm"`
}

type PickupAppointDayOption struct {
	Value    int                       `json:"value"` // dayOffset
	Text     string                    `json:"text"`
	Children []PickupAppointSlotOption `json:"children"`
}

type CheckPickupTimeResult struct {
	StartTm         string                    `json:"startTm"`
	EndTm           string                    `json:"endTm"`
	Status          bool                      `json:"status"`
	ExceptionReason string                    `json:"exceptionReason,omitempty"`
	CityCode        string                    `json:"cityCode,omitempty"`
	Address         string                    `json:"address,omitempty"`
	Options         []PickupAppointDayOption  `json:"options"`
}

// QueryDeliverTmRequest 按寄/收件地址查时效与预估运费。
type QueryDeliverTmRequest struct {
	CarrierAccountID uint64  `json:"carrierAccountId"`
	SrcProvince      string  `json:"srcProvince"`
	SrcCity          string  `json:"srcCity"`
	SrcCounty        string  `json:"srcCounty"`
	SrcAddress       string  `json:"srcAddress"`
	DestProvince     string  `json:"destProvince"`
	DestCity         string  `json:"destCity"`
	DestCounty       string  `json:"destCounty"`
	DestAddress      string  `json:"destAddress"`
	WeightKG         float64 `json:"weightKg"`
	UseMonthly       bool    `json:"useMonthly"`
	ConsignedTime    string  `json:"consignedTime,omitempty"`
	BusinessType     string  `json:"businessType,omitempty"`
}

type DeliverProductOption struct {
	Value        string  `json:"value"` // expressTypeId / businessType
	Name         string  `json:"name"`
	Tag          string  `json:"tag,omitempty"`
	Hint         string  `json:"hint,omitempty"`
	Fee          float64 `json:"fee,omitempty"`
	DeliverTime  string  `json:"deliverTime,omitempty"`
	DeliverLabel string  `json:"deliverLabel,omitempty"` // 预计 明天 15:00 前送达
}

type QueryDeliverTmResult struct {
	Products []DeliverProductOption `json:"products"`
}

// SearchPromiseTmResult 出单后预计派送时间（EXP_RECE_SEARCH_PROMITM）。
type SearchPromiseTmResult struct {
	MailNo       string `json:"mailNo"`
	PromiseTm    string `json:"promiseTm,omitempty"`
	PromiseLabel string `json:"promiseLabel,omitempty"` // 预计 明天 15:00 前送达
	Hint         string `json:"hint,omitempty"`
}

type OrderGoodsDTO struct {
	OrderItemID uint64  `json:"orderItemId"`
	PlanLineID  uint64  `json:"planLineId,omitempty"`
	Title       string  `json:"title"`
	SkuName     string  `json:"skuName"`
	Num         int     `json:"num"`
	OuterID     string  `json:"outerId"`
	Price       float64 `json:"price"`
}

type OrderSnapshotDTO struct {
	Platform         string          `json:"platform"`
	ShopID           string          `json:"shopId"`
	ShopName         string          `json:"shopName"`
	SourceChannel    string          `json:"sourceChannel"`
	ManualSourceName string          `json:"manualSourceName"`
	OrderNo          string          `json:"orderNo"` // 订单中心订单号，如 OC202608130007
	SysTid           string          `json:"sysTid"`
	SourceTid        string          `json:"sourceTid"`
	ReceiverName     string          `json:"receiverName"`
	ReceiverMobile   string          `json:"receiverMobile"`
	ReceiverProvince string          `json:"receiverProvince"`
	ReceiverCity     string          `json:"receiverCity"`
	ReceiverCounty   string          `json:"receiverCounty"`
	ReceiverAddress  string          `json:"receiverAddress"`
	Goods            []OrderGoodsDTO `json:"goods"`
}

type CreateShipmentFromOrderDTO struct {
	CarrierAccountID uint64           `json:"carrierAccountId"`
	ShipperProfileID uint64           `json:"shipperProfileId"`
	UseMonthly       *bool            `json:"useMonthly,omitempty"`
	ExpressType      string           `json:"expressType,omitempty"`
	PayMethod        int              `json:"payMethod,omitempty"`
	Remark           string           `json:"remark,omitempty"`
	CourierNote      string           `json:"courierNote,omitempty"`
	RemarkImages     []string         `json:"remarkImages,omitempty"`
	CargoName        string           `json:"cargoName,omitempty"`
	ParcelQty        int              `json:"parcelQty,omitempty"`
	CargoCount       int              `json:"cargoCount,omitempty"`
	TotalWeight      float64          `json:"totalWeight,omitempty"` // kg
	LengthCM         float64          `json:"lengthCm,omitempty"`
	WidthCM          float64          `json:"widthCm,omitempty"`
	HeightCM         float64          `json:"heightCm,omitempty"`
	TotalVolume      float64          `json:"totalVolume,omitempty"` // m³
	PickupMode       string           `json:"pickupMode,omitempty"`  // self | appoint
	SendStartTm      string           `json:"sendStartTm,omitempty"` // 预约上门开始时间 YYYY-MM-DD HH:mm:ss
	OrderID          uint64           `json:"orderId,omitempty"`
	SourceSystem     string           `json:"sourceSystem,omitempty"`
	GroupID          *uint64          `json:"groupId,omitempty"` // 拆分发货挂组
	Reship           bool             `json:"reship,omitempty"`  // 重新发货：回写 OC 空明细追加
	Order            OrderSnapshotDTO `json:"order"`
}

type ConfirmKdzsShipDTO struct {
	OrderID        uint64           `json:"orderId" binding:"required"`
	ExpressNo      string           `json:"expressNo" binding:"required"`
	ExpressCompany string           `json:"expressCompany"`
	Order          OrderSnapshotDTO `json:"order"`
	GroupID        *uint64          `json:"groupId,omitempty"`
	Reship         bool             `json:"reship,omitempty"` // 重新发货：回写 OC 空明细追加
}

// SplitShipLineDTO 拆分发货一行：商品数量 + 运单号。
// OrderItemID/Qty 均为 0 时表示「无商品明细追加包裹」（订单已全部发完后追加运单）。
type SplitShipLineDTO struct {
	OrderItemID    uint64 `json:"orderItemId"`
	PlanLineID     uint64 `json:"planLineId,omitempty"`
	Qty            int    `json:"qty"`
	ExpressNo      string `json:"expressNo" binding:"required"`
	ExpressCompany string `json:"expressCompany"`
	Title          string `json:"title"`
	SkuName        string `json:"skuName"`
	OuterID        string `json:"outerId"`
}

// ShipPlanLineInput 保存发货计划行。
// OrderItemID=0 表示整单拆分规格行（不对应原商品；保存后打单只认计划行）。
type ShipPlanLineInput struct {
	OrderItemID uint64 `json:"orderItemId"`
	SkuName     string `json:"skuName" binding:"required"`
	Qty         int    `json:"qty" binding:"required"`
	SortNo      int    `json:"sortNo"`
}

// PutShipPlanDTO 覆盖保存订单的待发计划行（仅替换 pending；已发 shipped 保留）。
type PutShipPlanDTO struct {
	Lines []ShipPlanLineInput `json:"lines"`
}

// ShipPlanLineDTO 发货计划行回传。
type ShipPlanLineDTO struct {
	ID               uint64 `json:"id"`
	OrderCoreID      uint64 `json:"orderCoreId"`
	OrderItemID      uint64 `json:"orderItemId"`
	SplitOrderItemID uint64 `json:"splitOrderItemId"`
	SkuName          string `json:"skuName"`
	Qty              int    `json:"qty"`
	SortNo           int    `json:"sortNo"`
	Status           string `json:"status"`
}

// MarkShipPlanShippedDTO 将计划行标为已发。
type MarkShipPlanShippedDTO struct {
	IDs []uint64 `json:"ids" binding:"required,min=1"`
}

// ConfirmKdzsSplitShipDTO 一次拆分确认：建发货组 + 多运单。
type ConfirmKdzsSplitShipDTO struct {
	OrderID        uint64            `json:"orderId" binding:"required"`
	ExpressCompany string            `json:"expressCompany"`
	Order          OrderSnapshotDTO  `json:"order"`
	Lines          []SplitShipLineDTO `json:"lines" binding:"required,min=1"`
}

// CreateShipmentGroupDTO 创建拆分发货主单（顺丰等分段出单前先建组）。
type CreateShipmentGroupDTO struct {
	OrderID   uint64 `json:"orderId"`
	OrderNo   string `json:"orderNo"`
	SourceRef string `json:"sourceRef"`
	ShipVia   string `json:"shipVia"`
}

// DeleteShipmentsByOrderCoreDTO 按订单中心销售单删除发货运单。
type DeleteShipmentsByOrderCoreDTO struct {
	OrderCoreOrderID uint64 `json:"orderCoreOrderId"`
	SourceRef        string `json:"sourceRef"`
}

// SyncShippedAtDTO 按运单号对齐发货时间（与快递助手/订单中心一致）。
type SyncShippedAtDTO struct {
	OrderCoreOrderID uint64 `json:"orderCoreOrderId"`
	MailNo           string `json:"mailNo" binding:"required"`
	ShippedAt        string `json:"shippedAt" binding:"required"` // RFC3339 或 2006-01-02 15:04:05
}

// UpsertKdzsFromSyncDTO 订单中心同步快递助手已发货后，补建/对齐发货中心发货单。
type UpsertKdzsFromSyncDTO struct {
	OrderID        uint64           `json:"orderId" binding:"required"`
	ExpressNo      string           `json:"expressNo" binding:"required"`
	ExpressCompany string           `json:"expressCompany"`
	ShippedAt      string           `json:"shippedAt"` // 快递助手发货时间；空则不改已有值
	Order          OrderSnapshotDTO `json:"order"`
}

type DecryptPendingOrdersDTO struct {
	Platform    string   `json:"platform"`
	TradeStatus string   `json:"tradeStatus"`
	SysTids     []string `json:"sysTids"`
}
