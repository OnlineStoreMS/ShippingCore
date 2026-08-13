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

type OrderGoodsDTO struct {
	OrderItemID uint64  `json:"orderItemId"`
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
	Order            OrderSnapshotDTO `json:"order"`
}

type ConfirmKdzsShipDTO struct {
	OrderID        uint64           `json:"orderId" binding:"required"`
	ExpressNo      string           `json:"expressNo" binding:"required"`
	ExpressCompany string           `json:"expressCompany"`
	Order          OrderSnapshotDTO `json:"order"`
}

// DeleteShipmentsByOrderCoreDTO 按订单中心销售单删除发货运单。
type DeleteShipmentsByOrderCoreDTO struct {
	OrderCoreOrderID uint64 `json:"orderCoreOrderId"`
	SourceRef        string `json:"sourceRef"`
}

type DecryptPendingOrdersDTO struct {
	Platform    string   `json:"platform"`
	TradeStatus string   `json:"tradeStatus"`
	SysTids     []string `json:"sysTids"`
}
