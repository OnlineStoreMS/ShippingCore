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

type OrderGoodsDTO struct {
	Title   string  `json:"title"`
	SkuName string  `json:"skuName"`
	Num     int     `json:"num"`
	OuterID string  `json:"outerId"`
	Price   float64 `json:"price"`
}

type OrderSnapshotDTO struct {
	Platform         string          `json:"platform"`
	ShopID           string          `json:"shopId"`
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

type DecryptPendingOrdersDTO struct {
	Platform    string   `json:"platform"`
	TradeStatus string   `json:"tradeStatus"`
	SysTids     []string `json:"sysTids"`
}
