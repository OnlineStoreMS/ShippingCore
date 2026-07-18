package dto

type CarrierAccountDTO struct {
	ID          uint64 `json:"id,omitempty"`
	CarrierCode string `json:"carrierCode"`
	Name        string `json:"name"`
	PartnerID   string `json:"partnerId"`
	Checkword   string `json:"checkword,omitempty"`
	UseMonthly  bool   `json:"useMonthly"`
	CustID      string `json:"custId"`
	ExpressType string `json:"expressType"`
	Env         string `json:"env"`
	Enabled     bool   `json:"enabled"`
	Remark      string `json:"remark"`
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
	Order            OrderSnapshotDTO `json:"order"`
}

type DecryptPendingOrdersDTO struct {
	Platform    string   `json:"platform"`
	TradeStatus string   `json:"tradeStatus"`
	SysTids     []string `json:"sysTids"`
}
