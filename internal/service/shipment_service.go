package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"shippingcore/internal/carrier/sf"
	"shippingcore/internal/dto"
	"shippingcore/internal/integrations/ordercore"
	"shippingcore/internal/integrations/storesyncagent"
	"shippingcore/internal/model"
	"shippingcore/internal/repo"

	"gorm.io/gorm"
)

type ShipmentService struct {
	repos    *repo.Repos
	carrier  *CarrierService
	shipper  *ShipperService
	ssAgent  *storesyncagent.Client
	orderCore *ordercore.Client
	tenantID uint64
}

func NewShipmentService(repos *repo.Repos, carrier *CarrierService, shipper *ShipperService, ssAgent *storesyncagent.Client, orderCore *ordercore.Client) *ShipmentService {
	return &ShipmentService{repos: repos, carrier: carrier, shipper: shipper, ssAgent: ssAgent, orderCore: orderCore}
}

func (s *ShipmentService) ForTenant(tenantID uint64) *ShipmentService {
	tid := repo.NormalizeTenantID(tenantID)
	return &ShipmentService{
		repos:     s.repos,
		carrier:   s.carrier.ForTenant(tid),
		shipper:   s.shipper.ForTenant(tid),
		ssAgent:   s.ssAgent,
		orderCore: s.orderCore,
		tenantID:  tid,
	}
}

func (s *ShipmentService) db() *gorm.DB {
	return s.repos.ForTenant(s.tenantID)
}

func (s *ShipmentService) List(status, sourceRef string, page, pageSize int) ([]model.Shipment, int64, error) {
	q := s.db().Model(&model.Shipment{}).Preload("Items")
	if status = strings.TrimSpace(status); status != "" {
		q = q.Where("status = ?", status)
	}
	if sourceRef = strings.TrimSpace(sourceRef); sourceRef != "" {
		q = q.Where("source_ref = ?", sourceRef)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.Shipment
	offset := (page - 1) * pageSize
	if err := q.Order("id DESC").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (s *ShipmentService) Get(id uint64) (*model.Shipment, error) {
	var item model.Shipment
	if err := s.db().Preload("Items").First(&item, id).Error; err != nil {
		if errorsIsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (s *ShipmentService) CreateFromOrder(in *dto.CreateShipmentFromOrderDTO) (*model.Shipment, error) {
	if in == nil || in.CarrierAccountID == 0 || in.ShipperProfileID == 0 {
		return nil, ErrBadRequest
	}
	order := in.Order

	carrier, err := s.carrier.GetRaw(in.CarrierAccountID)
	if err != nil {
		return nil, err
	}
	if !carrier.Enabled {
		return nil, fmt.Errorf("%w: carrier account disabled", ErrBadRequest)
	}
	if strings.TrimSpace(carrier.PartnerID) == "" || strings.TrimSpace(carrier.Checkword) == "" {
		return nil, fmt.Errorf("%w: carrier partnerId/checkword required", ErrBadRequest)
	}

	shipper, err := s.shipper.Get(in.ShipperProfileID)
	if err != nil {
		return nil, err
	}
	if !shipper.Enabled {
		return nil, fmt.Errorf("%w: shipper profile disabled", ErrBadRequest)
	}

	useMonthly := carrier.UseMonthly
	if in.UseMonthly != nil {
		useMonthly = *in.UseMonthly
	}
	if useMonthly && strings.TrimSpace(carrier.CustID) == "" {
		return nil, fmt.Errorf("%w: monthly settlement requires custId on carrier account", ErrBadRequest)
	}

	cargoName := "商品"
	items := make([]model.ShipmentItem, 0, len(order.Goods))
	for _, g := range order.Goods {
		name := strings.TrimSpace(g.Title)
		if name == "" {
			name = strings.TrimSpace(g.SkuName)
		}
		if name == "" {
			name = "商品"
		}
		qty := g.Num
		if qty <= 0 {
			qty = 1
		}
		if cargoName == "商品" && name != "" {
			cargoName = name
		}
		items = append(items, model.ShipmentItem{
			GoodsName: name,
			Quantity:  qty,
			SkuCode:   g.SkuName,
			OuterID:   g.OuterID,
		})
	}

	sourceSystem := strings.TrimSpace(in.SourceSystem)
	if sourceSystem == "" {
		sourceSystem = model.SourceSystemStoreSyncAgent
	}

	orderCoreOrderID := in.OrderID
	if sourceSystem == model.SourceSystemOrderCore && orderCoreOrderID == 0 {
		return nil, fmt.Errorf("%w: orderId required for ordercore source", ErrBadRequest)
	}

	sourceRef := firstNonEmptyTrim(
		order.SysTid,
		order.SourceTid,
	)
	if sourceRef == "" && orderCoreOrderID > 0 {
		sourceRef = fmt.Sprintf("OC%d", orderCoreOrderID)
	}
	if sourceRef == "" {
		return nil, fmt.Errorf("%w: sysTid/sourceTid/orderId required", ErrBadRequest)
	}

	expressType := strings.TrimSpace(in.ExpressType)
	if expressType == "" {
		expressType = strings.TrimSpace(carrier.ExpressType)
	}
	if expressType == "" {
		expressType = "2"
	}
	payMethod := in.PayMethod
	if payMethod == 0 {
		payMethod = 1
	}

	shipment := model.Shipment{
		TenantID:         s.tenantID,
		SourceSystem:     sourceSystem,
		SourceRef:        sourceRef,
		SourceTid:        strings.TrimSpace(order.SourceTid),
		Platform:         strings.TrimSpace(order.Platform),
		ShopID:           strings.TrimSpace(order.ShopID),
		OrderCoreOrderID: orderCoreOrderID,
		CarrierAccountID: carrier.ID,
		ShipperProfileID: shipper.ID,
		ReceiverName:     strings.TrimSpace(order.ReceiverName),
		ReceiverMobile:   strings.TrimSpace(order.ReceiverMobile),
		ReceiverProvince: strings.TrimSpace(order.ReceiverProvince),
		ReceiverCity:     strings.TrimSpace(order.ReceiverCity),
		ReceiverCounty:   strings.TrimSpace(order.ReceiverCounty),
		ReceiverAddress:  strings.TrimSpace(order.ReceiverAddress),
		ShipperName:      shipper.Name,
		ShipperMobile:    shipper.Mobile,
		ShipperProvince:  shipper.Province,
		ShipperCity:      shipper.City,
		ShipperCounty:    shipper.County,
		ShipperAddress:   shipper.Address,
		ShipperCompany:   shipper.Company,
		UseMonthly:       useMonthly,
		PayMethod:        payMethod,
		CustID:           "",
		ExpressType:      expressType,
		Status:           model.ShipmentStatusDraft,
		CargoName:        cargoName,
		ParcelQty:        1,
		Remark:           strings.TrimSpace(in.Remark),
		TotalWeight:      in.TotalWeight,
		Items:            items,
	}

	if useMonthly {
		shipment.CustID = carrier.CustID
	}

	if shipment.ReceiverAddress == "" {
		return nil, fmt.Errorf("%w: receiver address required", ErrBadRequest)
	}
	if shipment.ReceiverName == "" || shipment.ReceiverMobile == "" {
		return nil, fmt.Errorf("%w: receiver name and mobile required", ErrBadRequest)
	}

	if err := s.db().Create(&shipment).Error; err != nil {
		return nil, err
	}
	return s.Get(shipment.ID)
}

func (s *ShipmentService) CreateWaybill(ctx context.Context, token string, id uint64) (*model.Shipment, error) {
	shipment, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if shipment.Status != model.ShipmentStatusDraft && shipment.Status != model.ShipmentStatusFailed {
		return nil, ErrInvalidStatus
	}

	carrier, err := s.carrier.GetRaw(shipment.CarrierAccountID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(carrier.PartnerID) == "" || strings.TrimSpace(carrier.Checkword) == "" {
		return nil, fmt.Errorf("%w: carrier partnerId/checkword required", ErrBadRequest)
	}

	cargos := make([]sf.CargoDetail, 0, len(shipment.Items))
	for _, it := range shipment.Items {
		name := strings.TrimSpace(it.GoodsName)
		if name == "" {
			continue
		}
		qty := it.Quantity
		if qty <= 0 {
			qty = 1
		}
		cargos = append(cargos, sf.CargoDetail{Name: name, Count: qty})
	}

	client := sf.NewClient(carrier.PartnerID, carrier.Checkword, carrier.Env)
	orderID := shipmentOrderID(shipment.ID)
	result, err := client.CreateOrder(ctx, sf.CreateOrderRequest{
		OrderID:      orderID,
		UseMonthly:   shipment.UseMonthly,
		CustID:       shipment.CustID,
		ExpressType:  shipment.ExpressType,
		PayMethod:    shipment.PayMethod,
		ParcelQty:    shipment.ParcelQty,
		CargoName:    shipment.CargoName,
		CargoDetails: cargos,
		Remark:       shipment.Remark,
		TotalWeight:  shipment.TotalWeight,
		Shipper: sf.ContactInfo{
			ContactType: 1,
			Contact:     shipment.ShipperName,
			Mobile:      shipment.ShipperMobile,
			Province:    shipment.ShipperProvince,
			City:        shipment.ShipperCity,
			County:      shipment.ShipperCounty,
			Address:     shipment.ShipperAddress,
			Company:     shipment.ShipperCompany,
		},
		Receiver: sf.ContactInfo{
			ContactType: 2,
			Contact:     shipment.ReceiverName,
			Mobile:      shipment.ReceiverMobile,
			Province:    shipment.ReceiverProvince,
			City:        shipment.ReceiverCity,
			County:      shipment.ReceiverCounty,
			Address:     shipment.ReceiverAddress,
		},
	})
	if err != nil {
		shipment.Status = model.ShipmentStatusFailed
		shipment.ErrorMessage = err.Error()
		_ = s.db().Save(shipment).Error
		return nil, err
	}

	shipment.SFOrderID = result.SFOrderID
	shipment.MailNo = result.MailNo
	shipment.Status = model.ShipmentStatusCreated
	shipment.ErrorMessage = ""

	// 下单成功后尽量取云打印面单（同步 PDF，供本机打印）；失败不阻断出单
	if tpl := strings.TrimSpace(carrier.TemplateCode); tpl != "" {
		if printRes, printErr := client.CloudPrint(ctx, shipment.MailNo, tpl); printErr != nil {
			log.Printf("sf cloud print after create waybill %s: %v", shipment.MailNo, printErr)
		} else if printRes != nil {
			shipment.LabelURL = printRes.LabelURL
			shipment.LabelToken = printRes.LabelToken
			shipment.LabelData = printRes.LabelData
			if shipment.LabelURL != "" {
				shipment.Status = model.ShipmentStatusPrinted
			}
		}
	}

	if err := s.db().Save(shipment).Error; err != nil {
		return nil, err
	}

	if shipment.OrderCoreOrderID > 0 && shipment.MailNo != "" {
		if err := s.shipOrderCore(ctx, token, shipment.OrderCoreOrderID, "顺丰", shipment.MailNo); err != nil {
			return nil, fmt.Errorf("运单已出(%s)，回写订单中心失败: %w", shipment.MailNo, err)
		}
	}

	return s.Get(shipment.ID)
}

func firstNonEmptyTrim(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func (s *ShipmentService) Print(ctx context.Context, id uint64) (*model.Shipment, error) {
	shipment, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if shipment.MailNo == "" {
		return nil, fmt.Errorf("%w: waybill not created", ErrBadRequest)
	}
	if shipment.Status == model.ShipmentStatusCancelled {
		return nil, ErrInvalidStatus
	}

	carrier, err := s.carrier.GetRaw(shipment.CarrierAccountID)
	if err != nil {
		return nil, err
	}
	tpl := strings.TrimSpace(carrier.TemplateCode)
	if tpl == "" {
		return nil, fmt.Errorf("%w: 请在物流账号配置丰桥云打印模板编码（templateCode，如 fm_76130_standard_XXXX）", ErrBadRequest)
	}
	client := sf.NewClient(carrier.PartnerID, carrier.Checkword, carrier.Env)
	result, err := client.CloudPrint(ctx, shipment.MailNo, tpl)
	if err != nil {
		return nil, err
	}
	shipment.LabelURL = result.LabelURL
	shipment.LabelToken = result.LabelToken
	shipment.LabelData = result.LabelData
	if shipment.Status == model.ShipmentStatusCreated || shipment.Status == model.ShipmentStatusFailed {
		shipment.Status = model.ShipmentStatusPrinted
	}
	if err := s.db().Save(shipment).Error; err != nil {
		return nil, err
	}
	return s.Get(shipment.ID)
}

// FetchLabelPDF 代理下载顺丰云打印 PDF（带 token），供浏览器/本机打印组件打开。
func (s *ShipmentService) FetchLabelPDF(ctx context.Context, id uint64) ([]byte, string, error) {
	shipment, err := s.Get(id)
	if err != nil {
		return nil, "", err
	}
	if shipment.MailNo == "" {
		return nil, "", fmt.Errorf("%w: waybill not created", ErrBadRequest)
	}
	carrier, err := s.carrier.GetRaw(shipment.CarrierAccountID)
	if err != nil {
		return nil, "", err
	}
	client := sf.NewClient(carrier.PartnerID, carrier.Checkword, carrier.Env)

	labelURL := strings.TrimSpace(shipment.LabelURL)
	labelToken := strings.TrimSpace(shipment.LabelToken)
	if labelURL == "" || strings.HasPrefix(labelURL, "sf://") {
		tpl := strings.TrimSpace(carrier.TemplateCode)
		if tpl == "" {
			return nil, "", fmt.Errorf("%w: 请在物流账号配置丰桥云打印模板编码（templateCode，如 fm_76130_standard_XXXX）", ErrBadRequest)
		}
		printRes, err := client.CloudPrint(ctx, shipment.MailNo, tpl)
		if err != nil {
			return nil, "", fmt.Errorf("云打印失败: %w", err)
		}
		if printRes == nil || strings.TrimSpace(printRes.LabelURL) == "" {
			return nil, "", fmt.Errorf("云打印未返回可用面单，请检查丰桥模板编码/权限（当前 templateCode=%s）", tpl)
		}
		labelURL = printRes.LabelURL
		labelToken = printRes.LabelToken
		shipment.LabelURL = labelURL
		shipment.LabelToken = labelToken
		shipment.LabelData = printRes.LabelData
		if shipment.Status == model.ShipmentStatusCreated || shipment.Status == model.ShipmentStatusFailed {
			shipment.Status = model.ShipmentStatusPrinted
		}
		_ = s.db().Save(shipment).Error
	}

	pdf, err := client.DownloadLabelPDF(ctx, labelURL, labelToken)
	if err != nil {
		return nil, "", err
	}
	filename := "sf-" + shipment.MailNo + ".pdf"
	return pdf, filename, nil
}

func (s *ShipmentService) Cancel(ctx context.Context, id uint64) (*model.Shipment, error) {
	shipment, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if shipment.Status == model.ShipmentStatusCancelled {
		return shipment, nil
	}
	if shipment.Status == model.ShipmentStatusDraft {
		shipment.Status = model.ShipmentStatusCancelled
		if err := s.db().Save(shipment).Error; err != nil {
			return nil, err
		}
		return s.Get(shipment.ID)
	}

	carrier, err := s.carrier.GetRaw(shipment.CarrierAccountID)
	if err != nil {
		return nil, err
	}
	client := sf.NewClient(carrier.PartnerID, carrier.Checkword, carrier.Env)
	if err := client.CancelOrder(ctx, shipment.SFOrderID, shipment.MailNo, 2); err != nil {
		return nil, err
	}
	shipment.Status = model.ShipmentStatusCancelled
	shipment.ErrorMessage = ""
	if err := s.db().Save(shipment).Error; err != nil {
		return nil, err
	}
	return s.Get(shipment.ID)
}

func (s *ShipmentService) ListPendingOrders(ctx context.Context, token string, query storesyncagent.OrderQuery) (json.RawMessage, error) {
	if s.ssAgent == nil {
		return nil, fmt.Errorf("storesyncagent 未配置")
	}
	if query.TradeStatus == "" {
		query.TradeStatus = "wait_send"
	}
	return s.ssAgent.ListOrders(ctx, token, query)
}

func (s *ShipmentService) DecryptPendingOrders(ctx context.Context, token string, req dto.DecryptPendingOrdersDTO) (json.RawMessage, error) {
	if s.ssAgent == nil {
		return nil, fmt.Errorf("storesyncagent 未配置")
	}
	if strings.TrimSpace(req.Platform) == "" || len(req.SysTids) == 0 {
		return nil, ErrBadRequest
	}
	return s.ssAgent.DecryptOrders(ctx, token, storesyncagent.DecryptOrdersRequest{
		Platform:    req.Platform,
		TradeStatus: req.TradeStatus,
		SysTids:     req.SysTids,
	})
}

func (s *ShipmentService) ListPendingOMSOrders(ctx context.Context, token string, query ordercore.OrderQuery) (json.RawMessage, error) {
	if s.orderCore == nil {
		return nil, fmt.Errorf("ordercore 未配置")
	}
	if query.ShipStatus == "" {
		query.ShipStatus = "wait_ship"
	}
	if query.AllocType == "" {
		query.AllocType = "self_ship"
	}
	return s.orderCore.ListOrders(ctx, token, query)
}

func (s *ShipmentService) ConfirmKdzsShip(ctx context.Context, token string, in *dto.ConfirmKdzsShipDTO) (*model.Shipment, error) {
	if in == nil || in.OrderID == 0 || strings.TrimSpace(in.ExpressNo) == "" {
		return nil, ErrBadRequest
	}
	expressCompany := strings.TrimSpace(in.ExpressCompany)
	if expressCompany == "" {
		expressCompany = "快递"
	}

	order := in.Order
	cargoName := "商品"
	items := make([]model.ShipmentItem, 0, len(order.Goods))
	for _, g := range order.Goods {
		name := strings.TrimSpace(g.Title)
		if name == "" {
			name = strings.TrimSpace(g.SkuName)
		}
		if name == "" {
			name = "商品"
		}
		qty := g.Num
		if qty <= 0 {
			qty = 1
		}
		if cargoName == "商品" && name != "" {
			cargoName = name
		}
		items = append(items, model.ShipmentItem{
			GoodsName: name,
			Quantity:  qty,
			SkuCode:   g.SkuName,
			OuterID:   g.OuterID,
		})
	}

	shipment := model.Shipment{
		TenantID:         s.tenantID,
		SourceSystem:     model.SourceSystemOrderCore,
		SourceRef:        strings.TrimSpace(order.SysTid),
		SourceTid:        strings.TrimSpace(order.SourceTid),
		Platform:         strings.TrimSpace(order.Platform),
		ShopID:           strings.TrimSpace(order.ShopID),
		OrderCoreOrderID: in.OrderID,
		ReceiverName:     strings.TrimSpace(order.ReceiverName),
		ReceiverMobile:   strings.TrimSpace(order.ReceiverMobile),
		ReceiverProvince: strings.TrimSpace(order.ReceiverProvince),
		ReceiverCity:     strings.TrimSpace(order.ReceiverCity),
		ReceiverCounty:   strings.TrimSpace(order.ReceiverCounty),
		ReceiverAddress:  strings.TrimSpace(order.ReceiverAddress),
		MailNo:           strings.TrimSpace(in.ExpressNo),
		Status:           model.ShipmentStatusPrinted,
		CargoName:        cargoName,
		ParcelQty:        1,
		Items:            items,
	}
	if err := s.db().Create(&shipment).Error; err != nil {
		return nil, err
	}

	if err := s.shipOrderCore(ctx, token, in.OrderID, expressCompany, in.ExpressNo); err != nil {
		return nil, err
	}
	return s.Get(shipment.ID)
}

func (s *ShipmentService) shipOrderCore(ctx context.Context, token string, orderID uint64, expressCompany, expressNo string) error {
	if s.orderCore == nil || orderID == 0 || strings.TrimSpace(expressNo) == "" {
		return nil
	}
	_, err := s.orderCore.Ship(ctx, token, orderID, ordercore.ShipRequest{
		ExpressCompany: expressCompany,
		ExpressNo:      strings.TrimSpace(expressNo),
		Callback:       true,
	})
	return err
}
