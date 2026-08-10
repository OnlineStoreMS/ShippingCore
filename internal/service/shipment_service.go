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

	"gorm.io/gorm"
)

type ShipmentService struct {
	repos     *repo.Repos
	carrier   *CarrierService
	shipper   *ShipperService
	ssAgent   *storesyncagent.Client
	orderCore *ordercore.Client
	tenantID  uint64
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

// ShipmentListQuery 发货单列表筛选。
type ShipmentListQuery struct {
	Status    string
	Keyword   string // 模糊：运单号/系统单号/平台单号/收件人/手机
	MailNo    string
	SourceRef string
	SourceTid string
	Receiver  string // 收件人姓名或手机
	Platform  string
	Goods     string // 商品名称
	Page      int
	PageSize  int
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
			GoodsName: name,
			Quantity:  qty,
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
		ParcelQty:        parcelQty,
		CargoCount:       cargoCount,
		Remark:           strings.TrimSpace(in.Remark),
		CourierNote:      strings.TrimSpace(in.CourierNote),
		RemarkImages:     remarkImagesJSON,
		TotalWeight:      in.TotalWeight,
		LengthCM:         in.LengthCM,
		WidthCM:          in.WidthCM,
		HeightCM:         in.HeightCM,
		TotalVolume:      volume,
		PickupMode:       pickupMode,
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
	orderID := shipmentOrderID(shipment.ID)
	sfRemark := strings.TrimSpace(shipment.Remark)
	if note := strings.TrimSpace(shipment.CourierNote); note != "" {
		// 开放平台无独立「捎话」字段时并入 remark，前缀区分
		if sfRemark != "" {
			sfRemark = sfRemark + "；捎话:" + note
		} else {
			sfRemark = "捎话:" + note
		}
	}
	result, err := client.CreateOrder(ctx, sf.CreateOrderRequest{
		OrderID:      orderID,
		UseMonthly:   shipment.UseMonthly,
		CustID:       shipment.CustID,
		ExpressType:  shipment.ExpressType,
		PayMethod:    shipment.PayMethod,
		ParcelQty:    shipment.ParcelQty,
		CargoName:    sfCargoName,
		CargoDetails: cargos,
		Remark:       sfRemark,
		TotalWeight:  shipment.TotalWeight,
		TotalVolume:  shipment.TotalVolume,
		LengthCM:     shipment.LengthCM,
		WidthCM:      shipment.WidthCM,
		HeightCM:     shipment.HeightCM,
		IsDoCall:     strings.EqualFold(shipment.PickupMode, "appoint"),
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

// buildLabelRemark 拼进云打印 documents.remark（模板自定义区「备注」），含托寄物与商品摘要。
func buildLabelRemark(shipment *model.Shipment) string {
	parts := make([]string, 0, 4)
	if name := strings.TrimSpace(shipment.CargoName); name != "" {
		parts = append(parts, "托寄物:"+name)
	}
	if len(shipment.Items) > 0 {
		goods := make([]string, 0, len(shipment.Items))
		for _, it := range shipment.Items {
			n := shipmentItemShipName(it)
			if n == "" {
				continue
			}
			qty := it.Quantity
			if qty <= 0 {
				qty = 1
			}
			goods = append(goods, fmt.Sprintf("%s×%d", n, qty))
			if len(goods) >= 6 {
				break
			}
		}
		if len(goods) > 0 {
			parts = append(parts, "商品:"+strings.Join(goods, ","))
		}
	}
	if r := strings.TrimSpace(shipment.Remark); r != "" {
		parts = append(parts, r)
	}
	out := strings.Join(parts, "；")
	// 自定义区空间有限，避免过长
	const maxLen = 180
	if len([]rune(out)) > maxLen {
		rs := []rune(out)
		out = string(rs[:maxLen]) + "…"
	}
	return out
}

func (s *ShipmentService) printDocOpt(shipment *model.Shipment, carrier *model.CarrierAccount) *sf.PrintDocOptions {
	opt := &sf.PrintDocOptions{Remark: buildLabelRemark(shipment)}
	if carrier != nil {
		opt.CustomTemplateCode = strings.TrimSpace(carrier.CustomTemplateCode)
	}
	return opt
}

// resolveSFPrintTemplates 拆分标准模板 / 自定义模板。
// 丰桥要求 templateCode 必须归属当前顾客编码；*_custom_* 只能走 customTemplateCode。
func resolveSFPrintTemplates(carrier *model.CarrierAccount) (templateCode, customTemplateCode string) {
	std := strings.TrimSpace(carrier.TemplateCode)
	custom := strings.TrimSpace(carrier.CustomTemplateCode)

	// 误把自定义码填进「面单模板」时自动纠正
	if strings.Contains(std, "_custom_") {
		if custom == "" {
			custom = std
		}
		std = ""
	}
	if custom != "" && !strings.Contains(custom, "_custom_") && std == "" {
		// 自定义栏误填了标准码
		std = custom
		custom = ""
	}

	if std == "" {
		// 常见：fm_76130_standard_{partnerID}
		if p := strings.TrimSpace(carrier.PartnerID); p != "" {
			std = "fm_76130_standard_" + p
		}
	}
	return std, custom
}

func resolvePrintTemplateCode(carrier *model.CarrierAccount) string {
	std, _ := resolveSFPrintTemplates(carrier)
	return std
}

func (s *ShipmentService) applyCloudPrint(ctx context.Context, client *sf.Client, carrier *model.CarrierAccount, shipment *model.Shipment, tpl string) error {
	std, custom := resolveSFPrintTemplates(carrier)
	if std != "" {
		tpl = std
	}
	opt := s.printDocOpt(shipment, carrier)
	opt.CustomTemplateCode = custom
	channel := strings.ToLower(strings.TrimSpace(carrier.PrintChannel))
	if channel == "plugin" || channel == "parsed" || channel == "parseddata" {
		parsed, err := client.CloudPrintParsedData(ctx, shipment.MailNo, tpl, opt)
		if err != nil {
			return err
		}
		shipment.LabelURL = "sf-plugin://" + shipment.MailNo
		shipment.LabelToken = ""
		shipment.LabelData = string(parsed.ObjJSON)
		if shipment.Status == model.ShipmentStatusCreated || shipment.Status == model.ShipmentStatusFailed {
			shipment.Status = model.ShipmentStatusPrinted
		}
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
		if shipment.Status == model.ShipmentStatusCreated || shipment.Status == model.ShipmentStatusFailed {
			shipment.Status = model.ShipmentStatusPrinted
		}
	}
	return nil
}

// FetchPrintPluginData 返回官方云打印插件打印参数。
// 前端 SCPPrint.print 需 accessToken（OAuth2）+ templateCode + documents；
// SDK 内部会调 COM_RECE_CLOUD_PRINT_PARSEDDATA。此处同时尽力预取 PARSEDDATA 供排查。
func (s *ShipmentService) FetchPrintPluginData(ctx context.Context, id uint64) (map[string]interface{}, error) {
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

	std, custom := resolveSFPrintTemplates(carrier)
	if std != "" {
		tpl = std
	}
	opt := s.printDocOpt(shipment, carrier)
	opt.CustomTemplateCode = custom
	doc := map[string]interface{}{"masterWaybillNo": shipment.MailNo}
	if r := strings.TrimSpace(opt.Remark); r != "" {
		doc["remark"] = r
		doc["cargoDesc"] = r
		doc["goods"] = r
		doc["product"] = r
	}
	sdkData := map[string]interface{}{
		"requestID":    requestID,
		"accessToken":  accessToken,
		"templateCode": tpl,
		"documents":    []map[string]interface{}{doc},
	}
	if custom != "" {
		sdkData["customTemplateCode"] = custom
	}
	if r := strings.TrimSpace(opt.Remark); r != "" {
		sdkData["extJson"] = map[string]interface{}{
			"remark": r, "cargoDesc": r, "goods": r, "product": r,
		}
	}
	out := map[string]interface{}{
		"partnerId":    carrier.PartnerID,
		"env":          env,
		"templateCode": tpl,
		"mailNo":       shipment.MailNo,
		"requestId":    requestID,
		"accessToken":  accessToken,
		"labelRemark":  opt.Remark,
		"sdkPrintData": sdkData,
	}
	if custom != "" {
		out["customTemplateCode"] = custom
	}

	// 预取插件排版数据（非必须；失败不影响官方 SDK 打印）
	if parsed, err := client.CloudPrintParsedData(ctx, shipment.MailNo, tpl, opt); err == nil && parsed != nil {
		shipment.LabelURL = "sf-plugin://" + shipment.MailNo
		shipment.LabelData = string(parsed.ObjJSON)
		if shipment.Status == model.ShipmentStatusCreated || shipment.Status == model.ShipmentStatusFailed {
			shipment.Status = model.ShipmentStatusPrinted
		}
		_ = s.db().Save(shipment).Error

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
		if shipment.Status == model.ShipmentStatusCreated || shipment.Status == model.ShipmentStatusFailed {
			shipment.Status = model.ShipmentStatusPrinted
		}
		_ = s.db().Save(shipment).Error
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

	// 始终重新云打印：旧 LabelURL 可能是无 remark/无自定义区的缓存面单
	tpl, custom := resolveSFPrintTemplates(carrier)
	if tpl == "" {
		return nil, "", fmt.Errorf("%w: 请在物流账号配置归属本顾客编码的标准模板（如 fm_76130_standard_%s），自定义模板填到「自定义模板」", ErrBadRequest, strings.TrimSpace(carrier.PartnerID))
	}
	opt := s.printDocOpt(shipment, carrier)
	opt.CustomTemplateCode = custom
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
	if shipment.Status == model.ShipmentStatusCreated || shipment.Status == model.ShipmentStatusFailed {
		shipment.Status = model.ShipmentStatusPrinted
	}
	_ = s.db().Save(shipment).Error

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
	// 下单时 orderId 为 SC{id}；优先用顺丰回写的 sfOrderId，否则回退本地约定
	sfOrderID := firstNonEmptyTrim(shipment.SFOrderID, shipmentOrderID(shipment.ID))
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
		name := orderGoodsShipName(g)
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
			SkuCode:   strings.TrimSpace(g.SkuName),
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

// buildSFCargoDetails 组装丰桥 cargoDetails，名称一律用规格。
func buildSFCargoDetails(shipment *model.Shipment) []sf.CargoDetail {
	if shipment == nil {
		return nil
	}
	out := make([]sf.CargoDetail, 0, len(shipment.Items))
	for _, it := range shipment.Items {
		name := shipmentItemShipName(it)
		if name == "" {
			continue
		}
		qty := it.Quantity
		if qty <= 0 {
			qty = 1
		}
		out = append(out, sf.CargoDetail{Name: name, Count: qty})
	}
	return out
}

// shipmentSFCargoName 顺丰托寄物摘要：首个规格名，否则 CargoName。
func shipmentSFCargoName(shipment *model.Shipment) string {
	if shipment == nil {
		return "商品"
	}
	for _, it := range shipment.Items {
		if name := shipmentItemShipName(it); name != "" {
			return name
		}
	}
	if name := strings.TrimSpace(shipment.CargoName); name != "" {
		return name
	}
	return "商品"
}

func newSFClient(carrier *model.CarrierAccount) *sf.Client {
	return sf.NewClientWithSignMode(carrier.PartnerID, carrier.Checkword, carrier.Env, carrier.SignMode)
}
