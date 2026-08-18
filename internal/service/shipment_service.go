package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
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
		if strings.Contains(status, ",") {
			parts := make([]string, 0, 4)
			for _, p := range strings.Split(status, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					parts = append(parts, p)
				}
			}
			if len(parts) > 0 {
				dbq = dbq.Where("status IN ?", parts)
			}
		} else {
			dbq = dbq.Where("status = ?", status)
		}
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
	s.rewriteShipmentList(list)
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
	s.rewriteShipmentURLs(&item)
	return &item, nil
}

func (s *ShipmentService) rewriteShipmentURLs(item *model.Shipment) {
	if item == nil || s.store == nil {
		return
	}
	item.LabelPdfURL = s.store.ResolvePublicURL(item.LabelPdfURL)
}

func (s *ShipmentService) rewriteShipmentList(list []model.Shipment) {
	for i := range list {
		s.rewriteShipmentURLs(&list[i])
	}
}

// isKdzsShipment 快递助手推送/打单确认的发货单（本系统取消运单、云打印、面单存档均无效）。
func isKdzsShipment(shipment *model.Shipment) bool {
	if shipment == nil {
		return false
	}
	via := strings.ToLower(strings.TrimSpace(shipment.ShipVia))
	if via == model.ShipViaKdzs {
		return true
	}
	if via == model.ShipViaSF {
		return false
	}
	// 有运单号但从未丰桥取号：快递助手/手工填单（勿因误绑 carrier_account_id 判成顺丰）
	return strings.TrimSpace(shipment.MailNo) != "" && strings.TrimSpace(shipment.SFOrderID) == ""
}

// isSFManagedShipment 顺丰取号或已关联丰桥物流账号的发货单（支持预计派送/云打印/面单存档）。
func isSFManagedShipment(shipment *model.Shipment) bool {
	if shipment == nil || isKdzsShipment(shipment) {
		return false
	}
	via := strings.ToLower(strings.TrimSpace(shipment.ShipVia))
	if via == model.ShipViaSF {
		return true
	}
	if strings.TrimSpace(shipment.SFOrderID) != "" {
		return true
	}
	return shipment.CarrierAccountID > 0
}

// SearchPromiseTm 出单后查预计派送时间（EXP_RECE_SEARCH_PROMITM）。
func (s *ShipmentService) SearchPromiseTm(ctx context.Context, id uint64) (*dto.SearchPromiseTmResult, error) {
	shipment, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	mailNo := strings.TrimSpace(shipment.MailNo)
	if mailNo == "" {
		return nil, fmt.Errorf("%w: 无运单号，无法查询预计派送时间", ErrBadRequest)
	}
	if !isSFManagedShipment(shipment) {
		return &dto.SearchPromiseTmResult{
			MailNo: mailNo,
			Hint:   "非顺丰运单无需查询预计派送",
		}, nil
	}
	if shipment.CarrierAccountID == 0 {
		return nil, fmt.Errorf("%w: 发货单未关联物流账号", ErrBadRequest)
	}
	carrier, err := s.carrier.GetRaw(shipment.CarrierAccountID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(carrier.PartnerID) == "" || strings.TrimSpace(carrier.Checkword) == "" {
		return nil, fmt.Errorf("%w: 物流账号未配置顾客编码/校验码", ErrBadRequest)
	}

	recvDigits := digitsOnly(shipment.ReceiverMobile)
	recv4 := ""
	if len(recvDigits) >= 4 {
		recv4 = recvDigits[len(recvDigits)-4:]
	}
	client := newSFClient(carrier)
	custID := strings.TrimSpace(carrier.CustID)
	// 丰桥官方：checkType=1 电话号码校验；checkType=2 月结卡号校验。
	// 官方示例：checkType=2, checkNos=[月结卡号]
	tryReqs := make([]sf.SearchPromitmRequest, 0, 4)
	if custID != "" {
		tryReqs = append(tryReqs, sf.SearchPromitmRequest{SearchNo: mailNo, CheckType: 2, CheckNos: []string{custID}})
	}
	if custID != "" && recvDigits != "" {
		// 部分对接示例为「月结卡+手机」，官方页示例仅月结卡；作兼容回退
		tryReqs = append(tryReqs, sf.SearchPromitmRequest{SearchNo: mailNo, CheckType: 2, CheckNos: []string{custID, recvDigits}})
	}
	if recv4 != "" {
		tryReqs = append(tryReqs, sf.SearchPromitmRequest{SearchNo: mailNo, CheckType: 1, CheckNos: []string{recv4}})
	}
	if recvDigits != "" && recvDigits != recv4 {
		tryReqs = append(tryReqs, sf.SearchPromitmRequest{SearchNo: mailNo, CheckType: 1, CheckNos: []string{recvDigits}})
	}
	if len(tryReqs) == 0 {
		return nil, fmt.Errorf("%w: 无月结卡号且收件手机不足，无法校验查询", ErrBadRequest)
	}

	var (
		res    *sf.SearchPromitmResult
		lastErr error
	)
	for _, req := range tryReqs {
		r, e := client.SearchPromitm(ctx, req)
		if e == nil {
			res = r
			lastErr = nil
			break
		}
		lastErr = e
	}
	if lastErr != nil || res == nil {
		hint := "暂无预计派送时间"
		if lastErr != nil {
			hint = friendlyPromiseTmHint(lastErr.Error())
		}
		return &dto.SearchPromiseTmResult{
			MailNo: mailNo,
			Hint:   hint,
		}, nil
	}

	promiseTm := ""
	if res != nil {
		promiseTm = strings.TrimSpace(res.PromiseTm)
	}
	_, label := formatDeliverTimeLabel(time.Now(), promiseTm)
	if label == "" && promiseTm != "" {
		label = "预计 " + promiseTm + " 前送达"
	}
	out := &dto.SearchPromiseTmResult{
		MailNo:       mailNo,
		PromiseTm:    promiseTm,
		PromiseLabel: label,
	}
	if promiseTm == "" {
		out.Hint = "未返回预计派送时间"
	}
	return out, nil
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func friendlyPromiseTmHint(raw string) string {
	msg := strings.TrimSpace(raw)
	switch {
	case strings.Contains(msg, "无对应服务权限"), strings.Contains(msg, "A1004"):
		return "请在丰桥开通「预计派送时间查询」(EXP_RECE_SEARCH_PROMITM)"
	case strings.Contains(msg, "查询清单结果为空"), strings.Contains(msg, "结果为空"):
		// 刚下单、未揽收时常见；接口已通但顺丰侧尚无承诺时效
		return "暂无预计派送时间（通常揽收或产生路由后可查）"
	case strings.Contains(msg, "运单号不合法"):
		return "运单号无效或尚未生效，请稍后重试"
	default:
		if msg == "" {
			return "暂无预计派送时间"
		}
		return msg
	}
}

// SyncShippedAtByMailNo 把发货中心运单发货时间对齐为快递助手/订单中心时间。
// 快递助手单同时清空 printed_at（本系统未打印）。
func (s *ShipmentService) SyncShippedAtByMailNo(orderCoreOrderID uint64, mailNo, shippedAtRaw string) (int, error) {
	mailNo = strings.TrimSpace(mailNo)
	t := parseFlexibleTime(shippedAtRaw)
	if mailNo == "" || t == nil {
		return 0, fmt.Errorf("%w: mailNo 与 shippedAt 必填", ErrBadRequest)
	}
	dbq := s.db().Model(&model.Shipment{}).
		Where("tenant_id = ? AND mail_no = ? AND status <> ?", s.tenantID, mailNo, model.ShipmentStatusCancelled)
	if orderCoreOrderID > 0 {
		dbq = dbq.Where("order_core_order_id = ?", orderCoreOrderID)
	}
	res := dbq.Update("shipped_at", *t)
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}

// UpsertKdzsFromSync 订单中心同步到快递助手已发货后：有则对齐发货时间并清空打印时间，无则补建发货单。
func (s *ShipmentService) UpsertKdzsFromSync(in *dto.UpsertKdzsFromSyncDTO) (*model.Shipment, error) {
	if in == nil || in.OrderID == 0 || strings.TrimSpace(in.ExpressNo) == "" {
		return nil, ErrBadRequest
	}
	expressNo := strings.TrimSpace(in.ExpressNo)
	expressCompany := strings.TrimSpace(in.ExpressCompany)
	if expressCompany == "" {
		expressCompany = "快递"
	}
	shippedAt := parseFlexibleTime(in.ShippedAt)

	if existing, ok := s.findActiveKdzsShipment(in.OrderID, expressNo); ok {
		updates := map[string]any{
			"printed_at": nil, // 快递助手打单：不记本系统打印时间
			"ship_via":   model.ShipViaKdzs,
		}
		if shippedAt != nil {
			updates["shipped_at"] = *shippedAt
		}
		if expressCompany != "" && strings.TrimSpace(existing.ExpressCompany) == "" {
			updates["express_company"] = expressCompany
		}
		if err := s.db().Model(existing).Updates(updates).Error; err != nil {
			return nil, err
		}
		return s.Get(existing.ID)
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
	if shippedAt == nil {
		now := time.Now()
		shippedAt = &now
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
		MailNo:           expressNo,
		ShipVia:          model.ShipViaKdzs,
		ExpressCompany:   expressCompany,
		Status:           model.ShipmentStatusPrinted,
		ShippedAt:        shippedAt,
		// PrintedAt 故意不写：快递助手侧打印
		CargoName: cargoName,
		ParcelQty: 1,
		Items:     items,
	}
	if err := s.db().Create(&shipment).Error; err != nil {
		return nil, err
	}
	return s.Get(shipment.ID)
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
		GroupID:          in.GroupID,
		CarrierAccountID: carrier.ID,
		ShipperProfileID: shipper.ID,
		ShipVia:          model.ShipViaSF,
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
	_ = s.ApplyShipPlanProgressFromGoods(order.Goods)
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
	orderID := s.resolveSFCustomerOrderID(shipment)
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
	markShipmentShipped(shipment)

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
		if _, err := s.shipOrderCore(ctx, token, shipment.OrderCoreOrderID, "顺丰", shipment.MailNo, shipment.Items, true); err != nil {
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

// resolveSFCustomerOrderID 丰桥客户订单号：首次业务单号，取消后再下为 -2、-3…
func (s *ShipmentService) resolveSFCustomerOrderID(shipment *model.Shipment) string {
	if shipment == nil {
		return ""
	}
	base := sfCustomerOrderBase(shipment)
	if base == "" {
		return shipmentOrderID(shipment.ID)
	}
	seq := s.nextSFOrderSeq(shipment)
	return formatSFCustomerOrderID(base, seq)
}

// nextSFOrderSeq 同业务单下，已取过号（含已取消）的次数 + 1。
func (s *ShipmentService) nextSFOrderSeq(shipment *model.Shipment) int {
	if shipment == nil {
		return 1
	}
	dbq := s.db().Model(&model.Shipment{}).Where("tenant_id = ?", s.tenantID)
	if shipment.ID > 0 {
		dbq = dbq.Where("id <> ?", shipment.ID)
	}
	switch {
	case shipment.OrderCoreOrderID > 0:
		dbq = dbq.Where("order_core_order_id = ?", shipment.OrderCoreOrderID)
	case strings.TrimSpace(shipment.OrderNo) != "":
		dbq = dbq.Where("order_no = ?", strings.TrimSpace(shipment.OrderNo))
	default:
		return 1
	}
	// 仅统计曾向顺丰取号/留下客户订单号的发货单
	dbq = dbq.Where("(COALESCE(mail_no, '') <> '' OR COALESCE(sf_order_id, '') <> '')")
	var n int64
	if err := dbq.Count(&n).Error; err != nil {
		return 1
	}
	return int(n) + 1
}

func (s *ShipmentService) Print(ctx context.Context, id uint64) (*model.Shipment, error) {
	return s.PrintWithCarrier(ctx, id, 0)
}

// PrintWithCarrier 云打印；carrierAccountID>0 时用指定物流账号（再次打印可选账号）。
func (s *ShipmentService) PrintWithCarrier(ctx context.Context, id, carrierAccountID uint64) (*model.Shipment, error) {
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
	// 快递助手推送/打单：云打印与面单存档无效
	if isKdzsShipment(shipment) || !isSFManagedShipment(shipment) {
		return nil, fmt.Errorf("%w: 快递助手发货单不支持本系统打印与面单存档", ErrBadRequest)
	}

	carrier, err := s.resolvePrintCarrier(shipment, carrierAccountID)
	if err != nil {
		return nil, err
	}
	if shipment.CarrierAccountID == 0 {
		shipment.CarrierAccountID = carrier.ID
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

func (s *ShipmentService) resolvePrintCarrier(shipment *model.Shipment, overrideCarrierID uint64) (*model.CarrierAccount, error) {
	id := shipment.CarrierAccountID
	if overrideCarrierID > 0 {
		id = overrideCarrierID
	}
	if id == 0 {
		return nil, fmt.Errorf("%w: 请选择物流账号后再打印", ErrBadRequest)
	}
	carrier, err := s.carrier.GetRaw(id)
	if err != nil {
		return nil, fmt.Errorf("%w: 物流账号无效或不存在", ErrBadRequest)
	}
	if !carrier.Enabled {
		return nil, fmt.Errorf("%w: 物流账号已停用", ErrBadRequest)
	}
	return carrier, nil
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

func (s *ShipmentService) printDocOpt(_ *model.Shipment, carrier *model.CarrierAccount) *sf.PrintDocOptions {
	opt := &sf.PrintDocOptions{}
	if carrier != nil {
		opt.PrintLogo = carrier.PrintLogo
	}
	return opt
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

// markShipmentShipped 记录首次取得运单号的发货时间；已有值不覆盖。
// 注意：再次打印只更新 PrintedAt，不得调用本函数改写发货时间。
func markShipmentShipped(shipment *model.Shipment) {
	if shipment == nil || shipment.ShippedAt != nil {
		return
	}
	if strings.TrimSpace(shipment.MailNo) == "" {
		return
	}
	now := time.Now()
	shipment.ShippedAt = &now
}

func markShipmentPrinted(shipment *model.Shipment) {
	if shipment == nil {
		return
	}
	now := time.Now()
	shipment.PrintedAt = &now
	// 不在此处写 ShippedAt：发货时间=取号时间，与打印次数无关
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
		// force=true：再次打印也强制覆盖存档 PDF
		s.scheduleArchiveLabelPDF(shipment.ID, shipment.CarrierAccountID, shipment.MailNo, tpl, "", true)
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
		s.scheduleArchiveLabelPDF(shipment.ID, shipment.CarrierAccountID, shipment.MailNo, tpl, "", true)
	}
	return nil
}

// scheduleArchiveLabelPDF 后台重试拉取云打印 PDF 并写入 label_pdf_url。
// force：为 true 时即使已有存档也重新拉取覆盖（再次打印用）。
// 首次下单/插件打印后立刻调 COM_RECE_CLOUD_PRINT_WAYBILLS 常得到空 apiResultData，需稍后再试。
func (s *ShipmentService) scheduleArchiveLabelPDF(shipmentID, carrierAccountID uint64, mailNo, tpl, custom string, force bool) {
	if s.store == nil || shipmentID == 0 || strings.TrimSpace(mailNo) == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		// 首次出单：稍等避开丰桥空包；再次打印(force)：立刻重拉，失败再短间隔重试
		delays := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 60 * time.Second}
		if force {
			delays = []time.Duration{0, 2 * time.Second, 5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second}
		}
		for i, wait := range delays {
			if wait > 0 {
				select {
				case <-ctx.Done():
					log.Printf("archive label pdf aborted %s: %v", mailNo, ctx.Err())
					return
				case <-time.After(wait):
				}
			} else if ctx.Err() != nil {
				log.Printf("archive label pdf aborted %s: %v", mailNo, ctx.Err())
				return
			}
			var existing model.Shipment
			if err := s.db().Select("id", "label_pdf_url", "mail_no").First(&existing, shipmentID).Error; err != nil {
				log.Printf("archive label pdf load %d: %v", shipmentID, err)
				return
			}
			if !force && strings.TrimSpace(existing.LabelPdfURL) != "" {
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
			log.Printf("archive label pdf ok %s -> %s (force=%v)", mailNo, url, force)
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
// carrierAccountID>0 时用指定物流账号（再次打印可选账号）。
func (s *ShipmentService) FetchPrintPluginData(ctx context.Context, id uint64, overrideTpl, overrideCustom string, carrierAccountID uint64) (map[string]interface{}, error) {
	shipment, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if shipment.MailNo == "" {
		return nil, fmt.Errorf("%w: waybill not created", ErrBadRequest)
	}
	if isKdzsShipment(shipment) || !isSFManagedShipment(shipment) {
		return nil, fmt.Errorf("%w: 快递助手发货单不支持本系统打印与面单存档", ErrBadRequest)
	}
	carrier, err := s.resolvePrintCarrier(shipment, carrierAccountID)
	if err != nil {
		return nil, err
	}
	if shipment.CarrierAccountID == 0 {
		shipment.CarrierAccountID = carrier.ID
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
	if carrier.PrintLogo {
		doc["isPrintLogo"] = "true"
	}
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
		s.scheduleArchiveLabelPDF(shipment.ID, carrier.ID, shipment.MailNo, tpl, "", true)

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
		s.scheduleArchiveLabelPDF(shipment.ID, carrier.ID, shipment.MailNo, tpl, "", true)
		if err != nil {
			out["parsedDataError"] = err.Error()
		}
	}
	return out, nil
}

// FetchLabelPDF 代理下载顺丰云打印 PDF（带 token），供浏览器/本机打印组件打开。
// carrierAccountID>0 时用指定物流账号。
func (s *ShipmentService) FetchLabelPDF(ctx context.Context, id, carrierAccountID uint64) ([]byte, string, error) {
	shipment, err := s.Get(id)
	if err != nil {
		return nil, "", err
	}
	if shipment.MailNo == "" {
		return nil, "", fmt.Errorf("%w: waybill not created", ErrBadRequest)
	}
	if isKdzsShipment(shipment) || !isSFManagedShipment(shipment) {
		return nil, "", fmt.Errorf("%w: 快递助手发货单不支持本系统打印与面单存档", ErrBadRequest)
	}
	carrier, err := s.resolvePrintCarrier(shipment, carrierAccountID)
	if err != nil {
		return nil, "", err
	}
	if shipment.CarrierAccountID == 0 {
		shipment.CarrierAccountID = carrier.ID
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

func (s *ShipmentService) Cancel(ctx context.Context, token string, id uint64) (*model.Shipment, error) {
	shipment, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if shipment.Status == model.ShipmentStatusCancelled {
		_ = s.unshipOrderCore(ctx, token, shipment)
		return shipment, nil
	}
	if isKdzsShipment(shipment) {
		return nil, fmt.Errorf("%w: 快递助手发货单请在快递助手侧取消运单，本系统不支持取消/打印", ErrBadRequest)
	}
	mailNo := strings.TrimSpace(shipment.MailNo)
	// 草稿未向顺丰下单：仅作废本地发货单
	if shipment.Status == model.ShipmentStatusDraft || (mailNo == "" && strings.TrimSpace(shipment.SFOrderID) == "") {
		shipment.Status = model.ShipmentStatusCancelled
		shipment.ErrorMessage = ""
		if err := s.db().Save(shipment).Error; err != nil {
			return nil, err
		}
		_ = s.unshipOrderCore(ctx, token, shipment)
		return s.Get(shipment.ID)
	}

	// 有物流账号且已取号：向顺丰取消；手动填单号（无账号）仅本地作废
	if shipment.CarrierAccountID > 0 && (mailNo != "" || strings.TrimSpace(shipment.SFOrderID) != "") {
		carrier, err := s.carrier.GetRaw(shipment.CarrierAccountID)
		if err != nil {
			return nil, fmt.Errorf("%w: 物流账号无效，无法取消顺丰单", ErrBadRequest)
		}
		client := newSFClient(carrier)
		// 取消必须用下单时写入的 sfOrderId；勿按当前序号重算（会变成 -2 导致取消失败）
		sfOrderID := firstNonEmptyTrim(shipment.SFOrderID, sfCustomerOrderBase(shipment), shipmentOrderID(shipment.ID))
		if err := client.CancelOrder(ctx, sfOrderID, mailNo, 2); err != nil {
			return nil, fmt.Errorf("取消顺丰快递单失败: %w", err)
		}
	}

	shipment.Status = model.ShipmentStatusCancelled
	shipment.ErrorMessage = ""
	if err := s.db().Save(shipment).Error; err != nil {
		return nil, err
	}
	if err := s.unshipOrderCore(ctx, token, shipment); err != nil {
		return nil, fmt.Errorf("快递单已取消，但回退订单发货失败: %w", err)
	}
	return s.Get(shipment.ID)
}

// unshipOrderCore 取消快递后清除订单中心该运单对应商品发货明细，重算待发/部分发货。
func (s *ShipmentService) unshipOrderCore(ctx context.Context, token string, shipment *model.Shipment) error {
	if s.orderCore == nil || shipment == nil {
		return nil
	}
	orderID := shipment.OrderCoreOrderID
	mailNo := strings.TrimSpace(shipment.MailNo)
	if orderID == 0 || mailNo == "" {
		return nil
	}
	_, err := s.orderCore.Unship(ctx, token, orderID, ordercore.UnshipRequest{
		ExpressNo: mailNo,
		Remark:    fmt.Sprintf("发货中心取消快递单 #%d", shipment.ID),
	})
	return err
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
	expressNo := strings.TrimSpace(in.ExpressNo)

	// 幂等：同订单同运单号已有未取消发货单时复用，避免网关失败/重试产生重复单
	if existing, ok := s.findActiveKdzsShipment(in.OrderID, expressNo); ok {
		shippedAt, err := s.shipOrderCore(ctx, token, in.OrderID, firstNonEmptyTrim(expressCompany, existing.ExpressCompany), expressNo, existing.Items, false)
		if err != nil {
			return nil, fmt.Errorf("发货单已存在，回写订单中心失败: %w", err)
		}
		updates := map[string]any{}
		if shippedAt != nil && (existing.ShippedAt == nil || !existing.ShippedAt.Equal(*shippedAt)) {
			updates["shipped_at"] = *shippedAt
			updates["printed_at"] = nil
		} else if existing.PrintedAt != nil {
			updates["printed_at"] = nil
		}
		if in.GroupID != nil && (existing.GroupID == nil || *existing.GroupID != *in.GroupID) {
			updates["group_id"] = *in.GroupID
		}
		if len(updates) > 0 {
			_ = s.db().Model(existing).Updates(updates).Error
		}
		return s.Get(existing.ID)
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
	// 先回写订单中心（同号已同步则幂等成功），成功后再建发货中心记录，避免孤儿单
	shippedAt, err := s.shipOrderCore(ctx, token, in.OrderID, expressCompany, expressNo, items, false)
	if err != nil {
		return nil, fmt.Errorf("回写订单中心失败: %w", err)
	}
	now := time.Now()
	if shippedAt == nil {
		shippedAt = &now
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
		GroupID:          in.GroupID,
		ReceiverName:     strings.TrimSpace(order.ReceiverName),
		ReceiverMobile:   strings.TrimSpace(order.ReceiverMobile),
		ReceiverProvince: strings.TrimSpace(order.ReceiverProvince),
		ReceiverCity:     strings.TrimSpace(order.ReceiverCity),
		ReceiverCounty:   strings.TrimSpace(order.ReceiverCounty),
		ReceiverAddress:  strings.TrimSpace(order.ReceiverAddress),
		MailNo:           expressNo,
		ShipVia:          model.ShipViaKdzs,
		ExpressCompany:   expressCompany,
		Status:           model.ShipmentStatusPrinted,
		ShippedAt:        shippedAt,
		// 快递助手打单不在本系统打印，不记 printedAt（避免把确认时间误当成打印时间）
		CargoName:        cargoName,
		ParcelQty:        1,
		Items:            items,
	}
	if err := s.db().Create(&shipment).Error; err != nil {
		return nil, fmt.Errorf("订单中心已发货，创建发货单失败: %w", err)
	}
	_ = s.ApplyShipPlanProgressFromGoods(order.Goods)
	return s.Get(shipment.ID)
}

// CreateShipmentGroup 创建拆分发货主单。
func (s *ShipmentService) CreateShipmentGroup(in *dto.CreateShipmentGroupDTO) (*model.ShipmentGroup, error) {
	if in == nil {
		return nil, ErrBadRequest
	}
	shipVia := strings.TrimSpace(in.ShipVia)
	if shipVia == "" {
		shipVia = model.ShipViaKdzs
	}
	g := model.ShipmentGroup{
		TenantID:         s.tenantID,
		OrderCoreOrderID: in.OrderID,
		OrderNo:          strings.TrimSpace(in.OrderNo),
		SourceRef:        strings.TrimSpace(in.SourceRef),
		ShipVia:          shipVia,
		Status:           model.ShipmentStatusPrinted,
	}
	if err := s.db().Create(&g).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

// ConfirmKdzsSplitShip 拆分发货确认：建组 + 多运单回写订单中心。
// 同一运单号可对应多行商品（整单按包裹履约时前端会拆成多行同号），按运单号合并为一票。
func (s *ShipmentService) ConfirmKdzsSplitShip(ctx context.Context, token string, in *dto.ConfirmKdzsSplitShipDTO) (*model.ShipmentGroup, error) {
	if in == nil || in.OrderID == 0 || len(in.Lines) == 0 {
		return nil, ErrBadRequest
	}
	defaultCompany := strings.TrimSpace(in.ExpressCompany)
	if defaultCompany == "" {
		defaultCompany = "快递"
	}

	type pkgGoods struct {
		OrderItemID uint64
		Qty         int
		Title       string
		SkuName     string
		OuterID     string
		Company     string
	}
	type pkgBucket struct {
		ExpressNo string
		Goods     []pkgGoods
	}
	orderKeys := make([]string, 0)
	buckets := map[string]*pkgBucket{}
	qtyByItem := map[uint64]int{}

	for i, line := range in.Lines {
		no := strings.TrimSpace(line.ExpressNo)
		if no == "" {
			return nil, fmt.Errorf("%w: 第 %d 行运单号不能为空", ErrBadRequest, i+1)
		}
		emptyPkg := line.OrderItemID == 0 && line.Qty <= 0
		freeForm := line.OrderItemID == 0 && line.Qty > 0 // 整单拆分：仅规格名称，无原商品对应
		if !emptyPkg && !freeForm {
			if line.OrderItemID == 0 {
				return nil, fmt.Errorf("%w: 第 %d 行缺少商品行 ID", ErrBadRequest, i+1)
			}
			if line.Qty <= 0 {
				return nil, fmt.Errorf("%w: 第 %d 行发货数量须大于 0", ErrBadRequest, i+1)
			}
		}
		if freeForm {
			sku := firstNonEmptyTrim(line.SkuName, line.Title)
			if sku == "" {
				return nil, fmt.Errorf("%w: 第 %d 行规格名称不能为空", ErrBadRequest, i+1)
			}
		}
		key := strings.ToUpper(no)
		b, ok := buckets[key]
		if !ok {
			b = &pkgBucket{ExpressNo: no}
			buckets[key] = b
			orderKeys = append(orderKeys, key)
		}
		if emptyPkg {
			continue
		}
		b.Goods = append(b.Goods, pkgGoods{
			OrderItemID: line.OrderItemID,
			Qty:         line.Qty,
			Title:       strings.TrimSpace(line.Title),
			SkuName:     strings.TrimSpace(line.SkuName),
			OuterID:     strings.TrimSpace(line.OuterID),
			Company:     strings.TrimSpace(line.ExpressCompany),
		})
		if line.OrderItemID > 0 {
			qtyByItem[line.OrderItemID] += line.Qty
		}
	}

	// 有商品的包裹先确认，无明细追加包裹后确认（依赖订单中心「已发完可追加空运单」）
	sort.SliceStable(orderKeys, func(i, j int) bool {
		gi, gj := len(buckets[orderKeys[i]].Goods), len(buckets[orderKeys[j]].Goods)
		if (gi == 0) != (gj == 0) {
			return gi > 0
		}
		return i < j
	})

	orderNo := strings.TrimSpace(in.Order.OrderNo)
	sourceRef := firstNonEmptyTrim(strings.TrimSpace(in.Order.SysTid), orderNo, strings.TrimSpace(in.Order.SourceTid))
	group, err := s.CreateShipmentGroup(&dto.CreateShipmentGroupDTO{
		OrderID:   in.OrderID,
		OrderNo:   orderNo,
		SourceRef: sourceRef,
		ShipVia:   model.ShipViaKdzs,
	})
	if err != nil {
		return nil, err
	}

	goodsByID := map[uint64]dto.OrderGoodsDTO{}
	for _, g := range in.Order.Goods {
		if g.OrderItemID > 0 {
			goodsByID[g.OrderItemID] = g
		}
	}

	created := make([]*model.Shipment, 0, len(orderKeys))
	for _, key := range orderKeys {
		b := buckets[key]
		company := defaultCompany
		snapGoods := make([]dto.OrderGoodsDTO, 0, len(b.Goods))
		// 同运单同商品合并数量；整单拆分（orderItemId=0）按规格名分行不合并错位
		merged := map[uint64]*dto.OrderGoodsDTO{}
		mergeOrder := make([]uint64, 0)
		freeIdx := uint64(0)
		for _, g := range b.Goods {
			if c := firstNonEmptyTrim(g.Company); c != "" {
				company = c
			}
			base := goodsByID[g.OrderItemID]
			skuName := firstNonEmptyTrim(g.SkuName, g.Title, base.SkuName, base.Title)
			title := firstNonEmptyTrim(g.Title, base.Title)
			outerID := firstNonEmptyTrim(g.OuterID, base.OuterID)
			mergeKey := g.OrderItemID
			if mergeKey == 0 {
				freeIdx++
				mergeKey = ^uint64(0) - freeIdx // 伪 key，避免多规格挤成一行
				row := dto.OrderGoodsDTO{
					OrderItemID: 0,
					Title:       title,
					SkuName:     skuName,
					Num:         g.Qty,
					OuterID:     outerID,
					Price:       0,
				}
				merged[mergeKey] = &row
				mergeOrder = append(mergeOrder, mergeKey)
				continue
			}
			if cur, ok := merged[mergeKey]; ok {
				cur.Num += g.Qty
				continue
			}
			row := dto.OrderGoodsDTO{
				OrderItemID: g.OrderItemID,
				Title:       title,
				SkuName:     skuName,
				Num:         g.Qty,
				OuterID:     outerID,
				Price:       base.Price,
			}
			merged[mergeKey] = &row
			mergeOrder = append(mergeOrder, mergeKey)
		}
		for _, id := range mergeOrder {
			snapGoods = append(snapGoods, *merged[id])
		}
		snap := in.Order
		snap.Goods = snapGoods
		sh, err := s.ConfirmKdzsShip(ctx, token, &dto.ConfirmKdzsShipDTO{
			OrderID:        in.OrderID,
			ExpressNo:      b.ExpressNo,
			ExpressCompany: company,
			Order:          snap,
			GroupID:        &group.ID,
		})
		if err != nil {
			return nil, fmt.Errorf("运单 %s 确认失败: %w", b.ExpressNo, err)
		}
		created = append(created, sh)
	}
	_ = qtyByItem
	_ = s.ApplyShipPlanProgressFromSplitLines(in.Lines)
	group.Shipments = make([]model.Shipment, 0, len(created))
	for _, sh := range created {
		if sh != nil {
			group.Shipments = append(group.Shipments, *sh)
		}
	}
	return group, nil
}

// GetShipmentGroup 查发货组及组内运单。
func (s *ShipmentService) GetShipmentGroup(id uint64) (*model.ShipmentGroup, error) {
	var g model.ShipmentGroup
	err := s.db().
		Where("tenant_id = ? AND id = ?", s.tenantID, id).
		Preload("Shipments", func(db *gorm.DB) *gorm.DB {
			return db.Where("status <> ?", model.ShipmentStatusCancelled).Order("id ASC").Preload("Items")
		}).
		First(&g).Error
	if err != nil {
		return nil, err
	}
	for i := range g.Shipments {
		s.rewriteShipmentURLs(&g.Shipments[i])
	}
	return &g, nil
}

func (s *ShipmentService) findActiveKdzsShipment(orderCoreOrderID uint64, expressNo string) (*model.Shipment, bool) {
	if orderCoreOrderID == 0 || strings.TrimSpace(expressNo) == "" {
		return nil, false
	}
	var list []model.Shipment
	err := s.db().
		Where("tenant_id = ? AND order_core_order_id = ? AND mail_no = ? AND status <> ?",
			s.tenantID, orderCoreOrderID, strings.TrimSpace(expressNo), model.ShipmentStatusCancelled).
		Order("id ASC").
		Limit(1).
		Preload("Items").
		Find(&list).Error
	if err != nil || len(list) == 0 {
		return nil, false
	}
	return &list[0], true
}

// shipOrderCore 回写订单中心发货，并返回该运单在订单中心的发货时间（优先快递助手同步时间）。
// callback=true：顺丰等渠道需回传电商平台；快递助手侧已打单发货传 false，避免重复回传。
func (s *ShipmentService) shipOrderCore(ctx context.Context, token string, orderID uint64, expressCompany, expressNo string, shipmentItems []model.ShipmentItem, callback bool) (*time.Time, error) {
	if s.orderCore == nil || orderID == 0 || strings.TrimSpace(expressNo) == "" {
		return nil, nil
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
	if len(shipmentItems) > 0 && len(shipItems) == 0 {
		return nil, fmt.Errorf("发货明细缺少订单商品行 ID；请从待发货重新保存拆分并勾选子行后下单")
	}
	cb := callback
	raw, err := s.orderCore.Ship(ctx, token, orderID, ordercore.ShipRequest{
		ExpressCompany: expressCompany,
		ExpressNo:      strings.TrimSpace(expressNo),
		Callback:       &cb,
		Items:          shipItems,
	})
	if err != nil {
		return nil, err
	}
	return extractOrderShipmentShippedAt(raw, expressNo), nil
}

func extractOrderShipmentShippedAt(raw json.RawMessage, expressNo string) *time.Time {
	if len(raw) == 0 {
		return nil
	}
	expressNo = strings.TrimSpace(expressNo)
	var o struct {
		ShippedAt json.RawMessage `json:"shippedAt"`
		Shipments []struct {
			ExpressNo string          `json:"expressNo"`
			ShippedAt json.RawMessage `json:"shippedAt"`
		} `json:"shipments"`
	}
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil
	}
	for _, sh := range o.Shipments {
		if strings.TrimSpace(sh.ExpressNo) == expressNo {
			if t := parseJSONTime(sh.ShippedAt); t != nil {
				return t
			}
		}
	}
	return parseJSONTime(o.ShippedAt)
}

func parseJSONTime(raw json.RawMessage) *time.Time {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var t time.Time
	if err := json.Unmarshal(raw, &t); err == nil && !t.IsZero() {
		out := t
		return &out
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return parseFlexibleTime(s)
	}
	return nil
}

func parseFlexibleTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.000",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return &t
		}
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
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
