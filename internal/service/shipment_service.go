package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"shippingcore/internal/carrier/sf"
	"shippingcore/internal/dto"
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
	tenantID uint64
}

func NewShipmentService(repos *repo.Repos, carrier *CarrierService, shipper *ShipperService, ssAgent *storesyncagent.Client) *ShipmentService {
	return &ShipmentService{repos: repos, carrier: carrier, shipper: shipper, ssAgent: ssAgent}
}

func (s *ShipmentService) ForTenant(tenantID uint64) *ShipmentService {
	tid := repo.NormalizeTenantID(tenantID)
	return &ShipmentService{
		repos:    s.repos,
		carrier:  s.carrier.ForTenant(tid),
		shipper:  s.shipper.ForTenant(tid),
		ssAgent:  s.ssAgent,
		tenantID: tid,
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
	if strings.TrimSpace(order.SysTid) == "" {
		return nil, fmt.Errorf("%w: sysTid required", ErrBadRequest)
	}

	carrier, err := s.carrier.GetRaw(in.CarrierAccountID)
	if err != nil {
		return nil, err
	}
	if !carrier.Enabled {
		return nil, fmt.Errorf("%w: carrier account disabled", ErrBadRequest)
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

	shipment := model.Shipment{
		TenantID:         s.tenantID,
		SourceSystem:     model.SourceSystemStoreSyncAgent,
		SourceRef:        strings.TrimSpace(order.SysTid),
		SourceTid:        strings.TrimSpace(order.SourceTid),
		Platform:         strings.TrimSpace(order.Platform),
		ShopID:           strings.TrimSpace(order.ShopID),
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
		PayMethod:        1,
		CustID:           "",
		ExpressType:      carrier.ExpressType,
		Status:           model.ShipmentStatusDraft,
		CargoName:        cargoName,
		ParcelQty:        1,
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

func (s *ShipmentService) CreateWaybill(ctx context.Context, id uint64) (*model.Shipment, error) {
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

	client := sf.NewClient(carrier.PartnerID, carrier.Checkword, carrier.Env)
	orderID := shipmentOrderID(shipment.ID)
	result, err := client.CreateOrder(ctx, sf.CreateOrderRequest{
		OrderID:     orderID,
		UseMonthly:  shipment.UseMonthly,
		CustID:      shipment.CustID,
		ExpressType: shipment.ExpressType,
		PayMethod:   shipment.PayMethod,
		ParcelQty:   shipment.ParcelQty,
		CargoName:   shipment.CargoName,
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
	if err := s.db().Save(shipment).Error; err != nil {
		return nil, err
	}
	return s.Get(shipment.ID)
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
	client := sf.NewClient(carrier.PartnerID, carrier.Checkword, carrier.Env)
	result, _ := client.CloudPrint(ctx, shipment.MailNo, carrier.PartnerID)
	if result != nil {
		shipment.LabelURL = result.LabelURL
		shipment.LabelData = result.LabelData
	}
	if shipment.Status == model.ShipmentStatusCreated || shipment.Status == model.ShipmentStatusFailed {
		shipment.Status = model.ShipmentStatusPrinted
	}
	if err := s.db().Save(shipment).Error; err != nil {
		return nil, err
	}
	return s.Get(shipment.ID)
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
