package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"shippingcore/internal/carrier/sf"
	"shippingcore/internal/dto"
	"shippingcore/internal/integrations/ordercore"
	"shippingcore/internal/integrations/storesyncagent"
	"shippingcore/internal/model"
	"shippingcore/internal/repo"
	"shippingcore/internal/storage"

	"gorm.io/gorm"
)

type ShipmentService struct {
	repos     *repo.Repos
	carrier   *CarrierService
	shipper   *ShipperService
	ssAgent   *storesyncagent.Client
	orderCore *ordercore.Client
	store     storage.Storage
	tenantID  uint64
}

func NewShipmentService(repos *repo.Repos, carrier *CarrierService, shipper *ShipperService, ssAgent *storesyncagent.Client, orderCore *ordercore.Client, store storage.Storage) *ShipmentService {
	return &ShipmentService{repos: repos, carrier: carrier, shipper: shipper, ssAgent: ssAgent, orderCore: orderCore, store: store}
}

func (s *ShipmentService) ForTenant(tenantID uint64) *ShipmentService {
	tid := repo.NormalizeTenantID(tenantID)
	return &ShipmentService{
		repos:     s.repos,
		carrier:   s.carrier.ForTenant(tid),
		shipper:   s.shipper.ForTenant(tid),
		ssAgent:   s.ssAgent,
		orderCore: s.orderCore,
		store:     s.store,
		tenantID:  tid,
	}
}

func (s *ShipmentService) db() *gorm.DB {
	return s.repos.ForTenant(s.tenantID)
}

// ShipmentListQuery 发货单列表筛选。
type ShipmentListQuery struct {
	Status         string
	Keyword        string // 模糊：运单号/系统单号/平台单号/收件人/手机
	MailNo         string
	SourceRef      string
	SourceTid      string
	Receiver       string // 收件人姓名或手机
	Platform       string
	Goods          string // 商品名称
	PrintedAtStart *time.Time
	PrintedAtEnd   *time.Time
	Page           int
	PageSize       int
}

func (s *ShipmentService) List(q ShipmentListQuery) ([]model.Shipment, int64, error) {
	page, pageSize := q.Page, q.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	dbq := s.db().Model(&model.Shipment{})
	if status := strings.TrimSpace(q.Status); status != "" {
		dbq = dbq.Where("status = ?", status)
	}
	if mailNo := strings.TrimSpace(q.MailNo); mailNo != "" {
		dbq = dbq.Where("mail_no LIKE ?", "%"+mailNo+"%")
	}
	if sourceRef := strings.TrimSpace(q.SourceRef); sourceRef != "" {
		dbq = dbq.Where("source_ref LIKE ?", "%"+sourceRef+"%")
	}
	if sourceTid := strings.TrimSpace(q.SourceTid); sourceTid != "" {
		dbq = dbq.Where("source_tid LIKE ?", "%"+sourceTid+"%")
	}
	if platform := strings.TrimSpace(q.Platform); platform != "" {
		dbq = dbq.Where("platform = ?", platform)
	}
	if receiver := strings.TrimSpace(q.Receiver); receiver != "" {
		like := "%" + receiver + "%"
		dbq = dbq.Where("(receiver_name LIKE ? OR receiver_mobile LIKE ? OR receiver_address LIKE ?)", like, like, like)
	}
	if keyword := strings.TrimSpace(q.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		dbq = dbq.Where(
			"(mail_no LIKE ? OR source_ref LIKE ? OR source_tid LIKE ? OR receiver_name LIKE ? OR receiver_mobile LIKE ?)",
			like, like, like, like, like,
		)
	}
	if goods := strings.TrimSpace(q.Goods); goods != "" {
		sub := s.db().Model(&model.ShipmentItem{}).
			Select("shipment_id").
			Where("goods_name LIKE ?", "%"+goods+"%")
		dbq = dbq.Where("id IN (?)", sub)
	}
	if q.PrintedAtStart != nil {
		dbq = dbq.Where("COALESCE(printed_at, created_at) >= ?", q.PrintedAtStart)
	}
	if q.PrintedAtEnd != nil {
		dbq = dbq.Where("COALESCE(printed_at, created_at) <= ?", q.PrintedAtEnd)
	}

	var total int64
	if err := dbq.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.Shipment
	offset := (page - 1) * pageSize
	if err := dbq.Preload("Items").Order("id DESC").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
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

// DeleteByOrderCore 按订单中心销售单删除发货运单（手工单删除级联）。
func (s *ShipmentService) DeleteByOrderCore(orderCoreOrderID uint64, sourceRef string) (int, error) {
	sourceRef = strings.TrimSpace(sourceRef)
	if orderCoreOrderID == 0 && sourceRef == "" {
		return 0, fmt.Errorf("%w: orderCoreOrderId 或 sourceRef 必填", ErrBadRequest)
	}

	dbq := s.db().Model(&model.Shipment{})
	switch {
	case orderCoreOrderID > 0 && sourceRef != "":
		dbq = dbq.Where("order_core_order_id = ? OR source_ref = ? OR source_tid = ?", orderCoreOrderID, sourceRef, sourceRef)
	case orderCoreOrderID > 0:
		dbq = dbq.Where("order_core_order_id = ?", orderCoreOrderID)
	default:
		dbq = dbq.Where("source_ref = ? OR source_tid = ?", sourceRef, sourceRef)
	}

	var ids []uint64
	if err := dbq.Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}

	err := s.repos.DB.Transaction(func(tx *gorm.DB) error {
		// shipment_items 无 tenant_id，不能走 ForTenant scope
		if err := tx.Where("shipment_id IN ?", ids).Delete(&model.ShipmentItem{}).Error; err != nil {
			return err
		}
		return tx.Where("tenant_id = ? AND id IN ?", s.tenantID, ids).Delete(&model.Shipment{}).Error
	})
	if err != nil {
		return 0, err
	}
	return len(ids), nil
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
	cargoCountFromGoods := 0
	for _, g := range order.Goods {
		// 发货内容优先规格名称（skuName），商品名称仅作兜底
		name := orderGoodsShipName(g)
		qty := g.Num
		if qty <= 0 {
			qty = 1
		}
		cargoCountFromGoods += qty
		if cargoName == "商品" && name != "" {
			cargoName = name
		}
		items = append(items, model.ShipmentItem{
			OrderItemID: g.OrderItemID,
			GoodsName:   name,
			Quantity:    qty,
			// SkuCode 固定存规格名称，供下顺丰单 cargoDetails 使用
			SkuCode: strings.TrimSpace(g.SkuName),
			OuterID: g.OuterID,
		})
	}
	// 有明细时托寄物=首个规格名；无明细才用手填 cargoName
	if len(items) > 0 {
		cargoName = items[0].GoodsName
	} else if override := strings.TrimSpace(in.CargoName); override != "" {
		cargoName = override
	}
	parcelQty := in.ParcelQty
	if parcelQty <= 0 {
		parcelQty = 1
	}
	cargoCount := in.CargoCount
	if cargoCount <= 0 {
		cargoCount = cargoCountFromGoods
	}
	if cargoCount <= 0 {
		cargoCount = 1
	}
	remarkImagesJSON := ""
	if len(in.RemarkImages) > 0 {
		if b, err := json.Marshal(in.RemarkImages); err == nil {
			remarkImagesJSON = string(b)
		}
	}
	volume := in.TotalVolume
	if volume <= 0 && in.LengthCM > 0 && in.WidthCM > 0 && in.HeightCM > 0 {
		volume = in.LengthCM * in.WidthCM * in.HeightCM / 1_000_000
	}
	pickupMode := strings.TrimSpace(in.PickupMode)
	if pickupMode == "" {
		pickupMode = "self"
	}
	sendStartTm := strings.TrimSpace(in.SendStartTm)
	if strings.EqualFold(pickupMode, "appoint") && sendStartTm == "" {
		return nil, fmt.Errorf("%w: 预约寄件请选择上门时间", ErrBadRequest)
	}
	if sendStartTm != "" {
		if _, err := time.ParseInLocation("2006-01-02 15:04:05", sendStartTm, time.Local); err != nil {
			return nil, fmt.Errorf("%w: sendStartTm 格式须为 YYYY-MM-DD HH:mm:ss", ErrBadRequest)
		}
	}
	if !strings.EqualFold(pickupMode, "appoint") {
		sendStartTm = ""
	}

	sourceSystem := strings.TrimSpace(in.SourceSystem)
	if sourceSystem == "" {
		sourceSystem = model.SourceSystemStoreSyncAgent
	}

	orderCoreOrderID := in.OrderID
	if sourceSystem == model.SourceSystemOrderCore && orderCoreOrderID == 0 {
		return nil, fmt.Errorf("%w: orderId required for ordercore source", ErrBadRequest)
	}

	orderNo := strings.TrimSpace(order.OrderNo)
	sourceRef := firstNonEmptyTrim(
		order.SysTid,
		orderNo,
		order.SourceTid,
	)
	if sourceRef == "" && orderCoreOrderID > 0 {
		sourceRef = fmt.Sprintf("OC%d", orderCoreOrderID)
	}
	if sourceRef == "" {
		return nil, fmt.Errorf("%w: sysTid/sourceTid/orderId required", ErrBadRequest)
	}
	if orderNo == "" {
		// 兼容未传 orderNo：sourceTid 常为平台单号或手工单 OC…
		if tid := strings.TrimSpace(order.SourceTid); strings.HasPrefix(strings.ToUpper(tid), "OC") {
			orderNo = tid
		} else if strings.HasPrefix(strings.ToUpper(sourceRef), "OC") {
			orderNo = sourceRef
		}
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
		OrderNo:          orderNo,
		Platform:         strings.TrimSpace(order.Platform),
		ShopID:           strings.TrimSpace(order.ShopID),
		ShopName:         strings.TrimSpace(order.ShopName),
		SourceChannel:    strings.TrimSpace(order.SourceChannel),
		ManualSourceName: strings.TrimSpace(order.ManualSourceName),
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
		ParcelQty:        parcelQty,
		CargoCount:       cargoCount,
		Remark:           strings.TrimSpace(in.Remark),
		RemarkImages:     remarkImagesJSON,
		TotalWeight:      in.TotalWeight,
		LengthCM:         in.LengthCM,
		WidthCM:          in.WidthCM,
		HeightCM:         in.HeightCM,
		TotalVolume:      volume,
		PickupMode:       pickupMode,
		SendStartTm:      sendStartTm,
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

	// 下顺丰单物品信息：底层固定取规格名称（SkuCode），不是商品名称
	cargos := buildSFCargoDetails(shipment)
	sfCargoName := shipmentSFCargoName(shipment)

	client := newSFClient(carrier)
	// 面单「订单号」用平台订单号（如 OC202608110009），无则回退 SC{发货单id}
	orderID := sfCustomerOrderID(shipment)
	result, err := client.CreateOrder(ctx, sf.CreateOrderRequest{
		OrderID:      orderID,
		UseMonthly:   shipment.UseMonthly,
		CustID:       shipment.CustID,
		ExpressType:  shipment.ExpressType,
		PayMethod:    shipment.PayMethod,
		ParcelQty:    shipment.ParcelQty,
		CargoName:    sfCargoName,
		CargoDetails: cargos,
		Remark:       strings.TrimSpace(shipment.Remark),
		TotalWeight:  shipment.TotalWeight,
		TotalVolume:  shipment.TotalVolume,
		LengthCM:     shipment.LengthCM,
		WidthCM:      shipment.WidthCM,
		HeightCM:     shipment.HeightCM,
		IsDoCall:     strings.EqualFold(shipment.PickupMode, "appoint"),
		SendStartTm:  strings.TrimSpace(shipment.SendStartTm),
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

	// 下单成功后尽量取云打印数据；失败不阻断出单
	if tpl := resolvePrintTemplateCode(carrier); tpl != "" {
		if err := s.applyCloudPrint(ctx, client, carrier, shipment, tpl); err != nil {
			log.Printf("sf cloud print after create waybill %s: %v", shipment.MailNo, err)
		}
	}

	if err := s.db().Save(shipment).Error; err != nil {
		return nil, err
	}

	if shipment.OrderCoreOrderID > 0 && shipment.MailNo != "" {
		if err := s.shipOrderCore(ctx, token, shipment.OrderCoreOrderID, "顺丰", shipment.MailNo, shipment.Items); err != nil {
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
	tpl := resolvePrintTemplateCode(carrier)
	if tpl == "" {
		return nil, fmt.Errorf("%w: 请在物流账号配置丰桥云打印模板编码（templateCode，如 fm_76130_standard_XXXX）", ErrBadRequest)
	}
	client := newSFClient(carrier)
	if err := s.applyCloudPrint(ctx, client, carrier, shipment, tpl); err != nil {
		return nil, err
	}
	if err := s.db().Save(shipment).Error; err != nil {
		return nil, err
	}
	return s.Get(shipment.ID)
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max])
}

func (s *ShipmentService) printDocOpt(_ *model.Shipment, _ *model.CarrierAccount) *sf.PrintDocOptions {
	// 仅使用标准模板；不传自定义区 / remark（托寄物走 cargoDetails）
	return &sf.PrintDocOptions{}
}

// resolveSFPrintTemplates 仅返回标准模板；自定义区暂不使用。
func resolveSFPrintTemplates(carrier *model.CarrierAccount) (templateCode, customTemplateCode string) {
	std := strings.TrimSpace(carrier.TemplateCode)
	// 误把自定义码填进「面单模板」时改回顾客标准模板
	if strings.Contains(std, "_custom_") {
		std = ""
	}
	if std == "" {
		if p := strings.TrimSpace(carrier.PartnerID); p != "" {
			std = "fm_76130_standard_" + p
		}
	}
	return std, ""
}

func resolvePrintTemplateCode(carrier *model.CarrierAccount) string {
	std, _ := resolveSFPrintTemplates(carrier)
	return std
}

func markShipmentPrinted(shipment *model.Shipment) {
	if shipment == nil {
		return
	}
	now := time.Now()
	shipment.PrintedAt = &now
	if shipment.Status == model.ShipmentStatusCancelled {
		return
	}
	if shipment.Status == model.ShipmentStatusCreated ||
		shipment.Status == model.ShipmentStatusFailed ||
		shipment.Status == model.ShipmentStatusDraft {
		shipment.Status = model.ShipmentStatusPrinted
	}
}

func (s *ShipmentService) applyCloudPrint(ctx context.Context, client *sf.Client, carrier *model.CarrierAccount, shipment *model.Shipment, tpl string) error {
	std, _ := resolveSFPrintTemplates(carrier)
	if std != "" {
		tpl = std
	}
	opt := s.printDocOpt(shipment, carrier)
	channel := strings.ToLower(strings.TrimSpace(carrier.PrintChannel))
	if channel == "plugin" || channel == "parsed" || channel == "parseddata" {
		parsed, err := client.CloudPrintParsedData(ctx, shipment.MailNo, tpl, opt)
		if err != nil {
			return err
		}
		shipment.LabelURL = "sf-plugin://" + shipment.MailNo
		shipment.LabelToken = ""
		shipment.LabelData = string(parsed.ObjJSON)
		markShipmentPrinted(shipment)
		// 插件通道刚出单后立刻拉 PDF 常被丰桥返回空结果；异步重试存档，不挡出单/打印
		s.scheduleArchiveLabelPDF(shipment.ID, shipment.CarrierAccountID, shipment.MailNo, tpl, "")
		return nil
	}

	result, err := client.CloudPrint(ctx, shipment.MailNo, tpl, opt)
	if err != nil {
		return err
	}
	shipment.LabelURL = result.LabelURL
	shipment.LabelToken = result.LabelToken
	shipment.LabelData = result.LabelData
	if shipment.LabelURL != "" && !strings.HasPrefix(shipment.LabelURL, "sf://") {
		markShipmentPrinted(shipment)
	}
	if url, err := s.archiveLabelPDFOnce(ctx, client, carrier, shipment, tpl, "", result); err == nil && url != "" {
		shipment.LabelPdfURL = url
	} else {
		if err != nil {
			log.Printf("archive label pdf immediate %s: %v", shipment.MailNo, err)
		}
		s.scheduleArchiveLabelPDF(shipment.ID, shipment.CarrierAccountID, shipment.MailNo, tpl, "")
	}
	return nil
}

// scheduleArchiveLabelPDF 后台重试拉取云打印 PDF 并写入 label_pdf_url。
// 首次下单/插件打印后立刻调 COM_RECE_CLOUD_PRINT_WAYBILLS 常得到空 apiResultData，需稍后再试。
func (s *ShipmentService) scheduleArchiveLabelPDF(shipmentID, carrierAccountID uint64, mailNo, tpl, custom string) {
	if s.store == nil || shipmentID == 0 || strings.TrimSpace(mailNo) == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		// 首次稍等，避开出单后立刻拉 PDF 的空包窗口；间隔拉长提高成功率
		delays := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 60 * time.Second}
		for i, wait := range delays {
			select {
			case <-ctx.Done():
				log.Printf("archive label pdf aborted %s: %v", mailNo, ctx.Err())
				return
			case <-time.After(wait):
			}
			var existing model.Shipment
			if err := s.db().Select("id", "label_pdf_url", "mail_no").First(&existing, shipmentID).Error; err != nil {
				log.Printf("archive label pdf load %d: %v", shipmentID, err)
				return
			}
			if strings.TrimSpace(existing.LabelPdfURL) != "" {
				return
			}
			carrier, err := s.carrier.GetRaw(carrierAccountID)
			if err != nil {
				log.Printf("archive label pdf carrier %d: %v", carrierAccountID, err)
				return
			}
			client := newSFClient(carrier)
			url, err := s.archiveLabelPDFOnce(ctx, client, carrier, &existing, tpl, custom, nil)
			if err != nil {
				log.Printf("archive label pdf attempt %d/%d %s: %v", i+1, len(delays), mailNo, err)
				continue
			}
			if url == "" {
				log.Printf("archive label pdf attempt %d/%d %s: empty url", i+1, len(delays), mailNo)
				continue
			}
			if err := s.db().Model(&model.Shipment{}).Where("id = ?", shipmentID).Update("label_pdf_url", url).Error; err != nil {
				log.Printf("archive label pdf save %s: %v", mailNo, err)
				return
			}
			log.Printf("archive label pdf ok %s -> %s", mailNo, url)
			return
		}
		log.Printf("archive label pdf gave up %s after %d attempts", mailNo, len(delays))
	}()
}

// archiveLabelPDFOnce 单次拉取并上传面单 PDF；成功返回永久 URL。
func (s *ShipmentService) archiveLabelPDFOnce(ctx context.Context, client *sf.Client, carrier *model.CarrierAccount, shipment *model.Shipment, tpl, custom string, printRes *sf.PrintResult) (string, error) {
	if s.store == nil || shipment == nil || strings.TrimSpace(shipment.MailNo) == "" {
		return "", fmt.Errorf("store/shipment unavailable")
	}
	if tpl == "" {
		tpl, custom = resolveSFPrintTemplates(carrier)
	}
	if strings.TrimSpace(tpl) == "" {
		return "", fmt.Errorf("templateCode empty")
	}
	opt := s.printDocOpt(shipment, carrier)
	opt.CustomTemplateCode = custom

	var pdf []byte
	var err error
	if printRes != nil && strings.TrimSpace(printRes.LabelURL) != "" {
		pdf, err = client.DownloadLabelPDF(ctx, printRes.LabelURL, printRes.LabelToken)
	} else {
		res, cErr := client.CloudPrint(ctx, shipment.MailNo, tpl, opt)
		if cErr != nil {
			return "", cErr
		}
		if res == nil || strings.TrimSpace(res.LabelURL) == "" {
			return "", fmt.Errorf("cloud print empty url")
		}
		shipment.LabelURL = res.LabelURL
		shipment.LabelToken = res.LabelToken
		if res.LabelData != "" {
			shipment.LabelData = res.LabelData
		}
		pdf, err = client.DownloadLabelPDF(ctx, res.LabelURL, res.LabelToken)
	}
	if err != nil {
		return "", err
	}
	if len(pdf) == 0 {
		return "", fmt.Errorf("downloaded pdf empty")
	}
	filename := "sf-" + shipment.MailNo + ".pdf"
	url, upErr := s.store.UploadBytes(pdf, filename, "shipment-labels", "application/pdf")
	if upErr != nil {
		return "", upErr
	}
	shipment.LabelPdfURL = url
	return url, nil
}

// FetchPrintPluginData 返回官方云打印插件打印参数。
// 前端 SCPPrint.print 需 accessToken（OAuth2）+ templateCode + documents；
// SDK 内部会调 COM_RECE_CLOUD_PRINT_PARSEDDATA。此处同时尽力预取 PARSEDDATA 供排查。
func (s *ShipmentService) FetchPrintPluginData(ctx context.Context, id uint64, overrideTpl, overrideCustom string) (map[string]interface{}, error) {
	shipment, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if shipment.MailNo == "" {
		return nil, fmt.Errorf("%w: waybill not created", ErrBadRequest)
	}
	carrier, err := s.carrier.GetRaw(shipment.CarrierAccountID)
	if err != nil {
		return nil, err
	}
	tpl := resolvePrintTemplateCode(carrier)
	if tpl == "" {
		return nil, fmt.Errorf("%w: 请在物流账号配置丰桥云打印模板编码", ErrBadRequest)
	}
	client := newSFClient(carrier)

	accessToken, err := client.GetAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取丰桥 accessToken 失败: %w", err)
	}

	requestID := fmt.Sprintf("SC%d-%d", shipment.ID, time.Now().UnixMilli())
	env := "sbox"
	if strings.EqualFold(carrier.Env, "prod") || strings.EqualFold(carrier.Env, "production") {
		env = "pro"
	}

	std, _ := resolveSFPrintTemplates(carrier)
	if std != "" {
		tpl = std
	}
	if ot := strings.TrimSpace(overrideTpl); ot != "" && !strings.Contains(ot, "_custom_") {
		tpl = ot
	}
	_ = overrideCustom // 自定义模板已停用，忽略
	opt := s.printDocOpt(shipment, carrier)
	doc := map[string]interface{}{"masterWaybillNo": shipment.MailNo}
	sdkData := map[string]interface{}{
		"requestID":    requestID,
		"accessToken":  accessToken,
		"templateCode": tpl,
		"documents":    []map[string]interface{}{doc},
	}
	out := map[string]interface{}{
		"partnerId":    carrier.PartnerID,
		"env":          env,
		"templateCode": tpl,
		"mailNo":       shipment.MailNo,
		"requestId":    requestID,
		"accessToken":  accessToken,
		"sdkPrintData": sdkData,
	}

	// 预取插件排版数据（非必须；失败不影响官方 SDK 打印）
	if parsed, err := client.CloudPrintParsedData(ctx, shipment.MailNo, tpl, opt); err == nil && parsed != nil {
		shipment.LabelURL = "sf-plugin://" + shipment.MailNo
		shipment.LabelData = string(parsed.ObjJSON)
		markShipmentPrinted(shipment)
		_ = s.db().Save(shipment).Error
		s.scheduleArchiveLabelPDF(shipment.ID, shipment.CarrierAccountID, shipment.MailNo, tpl, "")

		out["requestId"] = firstNonEmptyTrim(parsed.RequestID, requestID)
		out["fileType"] = parsed.FileType
		out["templateCode"] = firstNonEmptyTrim(parsed.TemplateCode, tpl)
		var obj interface{}
		_ = json.Unmarshal(parsed.ObjJSON, &obj)
		var files interface{}
		_ = json.Unmarshal(parsed.FilesJSON, &files)
		out["obj"] = obj
		out["files"] = files
		sdk := out["sdkPrintData"].(map[string]interface{})
		sdk["requestID"] = firstNonEmptyTrim(parsed.RequestID, requestID)
		sdk["templateCode"] = firstNonEmptyTrim(parsed.TemplateCode, tpl)
	} else {
		shipment.LabelURL = "sf-plugin://" + shipment.MailNo
		markShipmentPrinted(shipment)
		_ = s.db().Save(shipment).Error
		s.scheduleArchiveLabelPDF(shipment.ID, shipment.CarrierAccountID, shipment.MailNo, tpl, "")
		if err != nil {
			out["parsedDataError"] = err.Error()
		}
	}
	return out, nil
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
	client := newSFClient(carrier)

	// 始终重新云打印：旧 LabelURL 可能是缓存面单
	tpl, _ := resolveSFPrintTemplates(carrier)
	if tpl == "" {
		return nil, "", fmt.Errorf("%w: 请在物流账号配置归属本顾客编码的标准模板（如 fm_76130_standard_%s）", ErrBadRequest, strings.TrimSpace(carrier.PartnerID))
	}
	opt := s.printDocOpt(shipment, carrier)
	printRes, err := client.CloudPrint(ctx, shipment.MailNo, tpl, opt)
	if err != nil {
		return nil, "", fmt.Errorf("云打印失败: %w", err)
	}
	if printRes == nil || strings.TrimSpace(printRes.LabelURL) == "" {
		return nil, "", fmt.Errorf("云打印未返回可用面单，请检查丰桥模板编码/权限（当前 templateCode=%s）", tpl)
	}
	labelURL := printRes.LabelURL
	labelToken := printRes.LabelToken
	shipment.LabelURL = labelURL
	shipment.LabelToken = labelToken
	shipment.LabelData = printRes.LabelData
	markShipmentPrinted(shipment)

	pdf, err := client.DownloadLabelPDF(ctx, labelURL, labelToken)
	if err != nil {
		return nil, "", err
	}
	if s.store != nil && len(pdf) > 0 {
		if url, upErr := s.store.UploadBytes(pdf, "sf-"+shipment.MailNo+".pdf", "shipment-labels", "application/pdf"); upErr != nil {
			log.Printf("archive label pdf upload %s: %v", shipment.MailNo, upErr)
		} else {
			shipment.LabelPdfURL = url
		}
	}
	_ = s.db().Save(shipment).Error

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
	// 草稿未向顺丰下单：仅作废本地发货单
	if shipment.Status == model.ShipmentStatusDraft || (shipment.MailNo == "" && strings.TrimSpace(shipment.SFOrderID) == "") {
		shipment.Status = model.ShipmentStatusCancelled
		shipment.ErrorMessage = ""
		if err := s.db().Save(shipment).Error; err != nil {
			return nil, err
		}
		return s.Get(shipment.ID)
	}

	carrier, err := s.carrier.GetRaw(shipment.CarrierAccountID)
	if err != nil {
		return nil, err
	}
	client := newSFClient(carrier)
	// 取消须与下单 orderId 一致：优先顺丰回写，其次平台订单号 / SC{id}
	sfOrderID := firstNonEmptyTrim(shipment.SFOrderID, sfCustomerOrderID(shipment))
	if err := client.CancelOrder(ctx, sfOrderID, shipment.MailNo, 2); err != nil {
		return nil, fmt.Errorf("取消顺丰快递单失败: %w", err)
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
		query.ShipStatus = "need_ship"
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
		name := orderGoodsShipName(g)
		qty := g.Num
		if qty <= 0 {
			qty = 1
		}
		if cargoName == "商品" && name != "" {
			cargoName = name
		}
		items = append(items, model.ShipmentItem{
			OrderItemID: g.OrderItemID,
			GoodsName:   name,
			Quantity:    qty,
			SkuCode:     strings.TrimSpace(g.SkuName),
			OuterID:     g.OuterID,
		})
	}

	orderNo := strings.TrimSpace(order.OrderNo)
	sourceTid := strings.TrimSpace(order.SourceTid)
	sourceRef := strings.TrimSpace(order.SysTid)
	if orderNo == "" {
		if strings.HasPrefix(strings.ToUpper(sourceTid), "OC") {
			orderNo = sourceTid
		} else if strings.HasPrefix(strings.ToUpper(sourceRef), "OC") {
			orderNo = sourceRef
		}
	}
	if sourceRef == "" {
		sourceRef = firstNonEmptyTrim(orderNo, sourceTid)
	}
	shipment := model.Shipment{
		TenantID:         s.tenantID,
		SourceSystem:     model.SourceSystemOrderCore,
		SourceRef:        sourceRef,
		SourceTid:        sourceTid,
		OrderNo:          orderNo,
		Platform:         strings.TrimSpace(order.Platform),
		ShopID:           strings.TrimSpace(order.ShopID),
		ShopName:         strings.TrimSpace(order.ShopName),
		SourceChannel:    strings.TrimSpace(order.SourceChannel),
		ManualSourceName: strings.TrimSpace(order.ManualSourceName),
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

	if err := s.shipOrderCore(ctx, token, in.OrderID, expressCompany, in.ExpressNo, items); err != nil {
		return nil, err
	}
	return s.Get(shipment.ID)
}

func (s *ShipmentService) shipOrderCore(ctx context.Context, token string, orderID uint64, expressCompany, expressNo string, shipmentItems []model.ShipmentItem) error {
	if s.orderCore == nil || orderID == 0 || strings.TrimSpace(expressNo) == "" {
		return nil
	}
	shipItems := make([]ordercore.ShipItemInput, 0, len(shipmentItems))
	for _, it := range shipmentItems {
		if it.OrderItemID == 0 || it.Quantity <= 0 {
			continue
		}
		shipItems = append(shipItems, ordercore.ShipItemInput{
			OrderItemID: it.OrderItemID,
			Qty:         it.Quantity,
		})
	}
	// 有发货明细却丢了销售行 ID 时，禁止回落成「空 items=整单发完」，避免部分发货被标成全部已发
	if len(shipmentItems) > 0 && len(shipItems) == 0 {
		return fmt.Errorf("发货明细缺少订单商品行 ID，无法按商品同步订单中心；请从待发货勾选商品重新下单")
	}
	_, err := s.orderCore.Ship(ctx, token, orderID, ordercore.ShipRequest{
		ExpressCompany: expressCompany,
		ExpressNo:      strings.TrimSpace(expressNo),
		Callback:       true,
		Items:          shipItems,
	})
	return err
}

// orderGoodsShipName 发货内容：优先规格名称（skuName），无规格时才用商品名称。
func orderGoodsShipName(g dto.OrderGoodsDTO) string {
	if name := strings.TrimSpace(g.SkuName); name != "" {
		return name
	}
	if name := strings.TrimSpace(g.Title); name != "" {
		return name
	}
	return "商品"
}

// shipmentItemShipName 明细发货名：SkuCode 存规格名；历史单 GoodsName 可能是商品名。
func shipmentItemShipName(it model.ShipmentItem) string {
	if name := strings.TrimSpace(it.SkuCode); name != "" {
		return name
	}
	return strings.TrimSpace(it.GoodsName)
}

// 丰桥 cargoDetails[].name 上限 String(128)；76×130 面单「托寄物」通常只显示一行。
const sfCargoNameMaxRunes = 128

// joinShipmentCargoLabel 对齐企服托寄物：规格*数量, 规格2*数量 * 总数量
func joinShipmentCargoLabel(shipment *model.Shipment) string {
	if shipment == nil {
		return ""
	}
	parts := make([]string, 0, len(shipment.Items))
	for _, it := range shipment.Items {
		n := shipmentItemShipName(it)
		if n == "" {
			continue
		}
		qty := it.Quantity
		if qty <= 0 {
			qty = 1
		}
		parts = append(parts, fmt.Sprintf("%s*%d", n, qty))
	}
	out := strings.Join(parts, ", ")
	if out == "" {
		out = strings.TrimSpace(shipment.CargoName)
	}
	if out == "" {
		return ""
	}
	total := shipmentCargoTotalQty(shipment)
	out = fmt.Sprintf("%s * %d", out, total)
	return truncateRunes(out, sfCargoNameMaxRunes)
}

// shipmentCargoTotalQty 订单商品总数量（托寄物 count）。
func shipmentCargoTotalQty(shipment *model.Shipment) int {
	if shipment == nil {
		return 1
	}
	if shipment.CargoCount > 0 {
		return shipment.CargoCount
	}
	sum := 0
	for _, it := range shipment.Items {
		qty := it.Quantity
		if qty <= 0 {
			qty = 1
		}
		if shipmentItemShipName(it) != "" || strings.TrimSpace(it.GoodsName) != "" {
			sum += qty
		}
	}
	if sum > 0 {
		return sum
	}
	return 1
}

// buildSFCargoDetails 组装丰桥 cargoDetails。
// 面单托寄物区一般只画第一条 name，故多规格合并为一条完整文案（限 128 字）；
// count 传订单商品总数量。
func buildSFCargoDetails(shipment *model.Shipment) []sf.CargoDetail {
	label := joinShipmentCargoLabel(shipment)
	if label == "" {
		return nil
	}
	return []sf.CargoDetail{{Name: label, Count: shipmentCargoTotalQty(shipment)}}
}

// shipmentSFCargoName 托寄物摘要（无明细时的兜底）。
func shipmentSFCargoName(shipment *model.Shipment) string {
	if label := joinShipmentCargoLabel(shipment); label != "" {
		return label
	}
	return "商品"
}

func newSFClient(carrier *model.CarrierAccount) *sf.Client {
	return sf.NewClientWithSignMode(carrier.PartnerID, carrier.Checkword, carrier.Env, carrier.SignMode)
}
