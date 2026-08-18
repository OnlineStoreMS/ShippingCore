package admin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"shippingcore/internal/dto"
	"shippingcore/internal/integrations/storesyncagent"
	"shippingcore/internal/pkg/authcontext"
	"shippingcore/internal/pkg/httputil"
	"shippingcore/internal/pkg/response"
	"shippingcore/internal/service"

	"github.com/gin-gonic/gin"
)

type Handlers struct {
	Carrier  *service.CarrierService
	Shipper  *service.ShipperService
	Shipment *service.ShipmentService
	Kdzs     *service.KdzsService
}

func NewHandlers(carrier *service.CarrierService, shipper *service.ShipperService, shipment *service.ShipmentService, kdzs *service.KdzsService) *Handlers {
	return &Handlers{Carrier: carrier, Shipper: shipper, Shipment: shipment, Kdzs: kdzs}
}

func (h *Handlers) carrier(c *gin.Context) *service.CarrierService {
	return h.Carrier.ForTenant(authcontext.TenantID(c))
}

func (h *Handlers) shipper(c *gin.Context) *service.ShipperService {
	return h.Shipper.ForTenant(authcontext.TenantID(c))
}

func (h *Handlers) shipment(c *gin.Context) *service.ShipmentService {
	return h.Shipment.ForTenant(authcontext.TenantID(c))
}

func firstQuery(c *gin.Context, keys ...string) string {
	for _, k := range keys {
		if v := c.Query(k); v != "" {
			return v
		}
	}
	return ""
}

func parseQueryTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			if layout == "2006-01-02" {
				// 仅日期：起始按当日 00:00；截止由调用方传 23:59:59 或次日，这里按当天 00:00 解析
				day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
				return &day
			}
			return &t
		}
	}
	return nil
}

// parseQueryTimeEnd 解析截止时间；纯日期按含当日（23:59:59）。
func parseQueryTimeEnd(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			if layout == "2006-01-02" {
				end := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, time.Local)
				return &end
			}
			return &t
		}
	}
	return nil
}

// ── Carrier accounts ──

func (h *Handlers) ListCarrierAccounts(c *gin.Context) {
	page, pageSize := httputil.ParsePage(c)
	list, total, err := h.carrier(c).List(c.Query("keyword"), page, pageSize)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.OK(c, response.PageResult(list, total, page, pageSize))
}

func (h *Handlers) GetCarrierAccount(c *gin.Context) {
	id, err := httputil.ParseID(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := h.carrier(c).Get(id)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, item)
}

func (h *Handlers) CreateCarrierAccount(c *gin.Context) {
	var in dto.CarrierAccountDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.carrier(c).Create(&in)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.Created(c, item)
}

func (h *Handlers) UpdateCarrierAccount(c *gin.Context) {
	id, err := httputil.ParseID(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	var in dto.CarrierAccountDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.carrier(c).Update(id, &in)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, item)
}

func (h *Handlers) DeleteCarrierAccount(c *gin.Context) {
	id, err := httputil.ParseID(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.carrier(c).Delete(id); err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// ── Shipper profiles ──

func (h *Handlers) ListShipperProfiles(c *gin.Context) {
	page, pageSize := httputil.ParsePage(c)
	list, total, err := h.shipper(c).List(c.Query("keyword"), page, pageSize)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.OK(c, response.PageResult(list, total, page, pageSize))
}

func (h *Handlers) GetShipperProfile(c *gin.Context) {
	id, err := httputil.ParseID(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := h.shipper(c).Get(id)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, item)
}

func (h *Handlers) CreateShipperProfile(c *gin.Context) {
	var in dto.ShipperProfileDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.shipper(c).Create(&in)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.Created(c, item)
}

func (h *Handlers) UpdateShipperProfile(c *gin.Context) {
	id, err := httputil.ParseID(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	var in dto.ShipperProfileDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.shipper(c).Update(id, &in)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, item)
}

func (h *Handlers) DeleteShipperProfile(c *gin.Context) {
	id, err := httputil.ParseID(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.shipper(c).Delete(id); err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handlers) SetDefaultShipperProfile(c *gin.Context) {
	id, err := httputil.ParseID(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := h.shipper(c).SetDefault(id)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, item)
}

func (h *Handlers) CheckPickupTime(c *gin.Context) {
	var in dto.CheckPickupTimeRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.carrier(c).CheckPickupTime(c.Request.Context(), in)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, item)
}

func (h *Handlers) QueryDeliverTm(c *gin.Context) {
	var in dto.QueryDeliverTmRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.carrier(c).QueryDeliverTm(c.Request.Context(), in)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, item)
}

// ── Shipments ──

func (h *Handlers) ListShipments(c *gin.Context) {
	page, pageSize := httputil.ParsePage(c)
	list, total, err := h.shipment(c).List(service.ShipmentListQuery{
		Status:         c.Query("status"),
		Keyword:        firstQuery(c, "keyword", "q"),
		MailNo:         c.Query("mail_no"),
		SourceRef:      firstQuery(c, "source_ref", "sourceRef"),
		SourceTid:      firstQuery(c, "source_tid", "sourceTid"),
		Receiver:       c.Query("receiver"),
		Platform:       c.Query("platform"),
		Goods:          firstQuery(c, "goods", "goods_name"),
		PrintedAtStart: parseQueryTime(firstQuery(c, "printedAtStart", "printed_at_start")),
		PrintedAtEnd:   parseQueryTimeEnd(firstQuery(c, "printedAtEnd", "printed_at_end")),
		ShippedAtStart: parseQueryTime(firstQuery(c, "shippedAtStart", "shipped_at_start")),
		ShippedAtEnd:   parseQueryTimeEnd(firstQuery(c, "shippedAtEnd", "shipped_at_end")),
		Page:           page,
		PageSize:       pageSize,
	})
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.OK(c, response.PageResult(list, total, page, pageSize))
}

func (h *Handlers) GetShipment(c *gin.Context) {
	id, err := httputil.ParseID(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := h.shipment(c).Get(id)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, item)
}

func (h *Handlers) SearchPromiseTm(c *gin.Context) {
	id, err := httputil.ParseID(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := h.shipment(c).SearchPromiseTm(c.Request.Context(), id)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, item)
}

func (h *Handlers) CreateShipmentFromOrder(c *gin.Context) {
	var in dto.CreateShipmentFromOrderDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.shipment(c).CreateFromOrder(&in)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.Created(c, item)
}

func (h *Handlers) CreateShipmentWaybill(c *gin.Context) {
	id, err := httputil.ParseID(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := h.shipment(c).CreateWaybill(c.Request.Context(), authcontext.BearerToken(c), id)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, item)
}

func parseOptionalCarrierAccountID(c *gin.Context) uint64 {
	raw := strings.TrimSpace(c.Query("carrierAccountId"))
	if raw == "" {
		raw = strings.TrimSpace(c.PostForm("carrierAccountId"))
	}
	if raw == "" {
		return 0
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func (h *Handlers) PrintShipment(c *gin.Context) {
	id, err := httputil.ParseID(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	carrierID := parseOptionalCarrierAccountID(c)
	if carrierID == 0 {
		var body struct {
			CarrierAccountID uint64 `json:"carrierAccountId"`
		}
		_ = c.ShouldBindJSON(&body)
		carrierID = body.CarrierAccountID
	}
	item, err := h.shipment(c).PrintWithCarrier(c.Request.Context(), id, carrierID)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, item)
}

func (h *Handlers) DownloadShipmentLabel(c *gin.Context) {
	id, err := httputil.ParseID(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	pdf, filename, err := h.shipment(c).FetchLabelPDF(c.Request.Context(), id, parseOptionalCarrierAccountID(c))
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	if filename == "" {
		filename = "sf-label.pdf"
	}
	c.Header("Content-Disposition", `inline; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/pdf", pdf)
}

func (h *Handlers) GetShipmentPrintPluginData(c *gin.Context) {
	id, err := httputil.ParseID(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	data, err := h.shipment(c).FetchPrintPluginData(
		c.Request.Context(),
		id,
		c.Query("templateCode"),
		c.Query("customTemplateCode"),
		parseOptionalCarrierAccountID(c),
	)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, data)
}

func (h *Handlers) CancelShipment(c *gin.Context) {
	id, err := httputil.ParseID(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := h.shipment(c).Cancel(c.Request.Context(), authcontext.BearerToken(c), id)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, item)
}

func (h *Handlers) DeleteShipmentsByOrderCore(c *gin.Context) {
	var in dto.DeleteShipmentsByOrderCoreDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	n, err := h.shipment(c).DeleteByOrderCore(in.OrderCoreOrderID, in.SourceRef)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": n})
}

func (h *Handlers) SyncShipmentShippedAt(c *gin.Context) {
	var in dto.SyncShippedAtDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	n, err := h.shipment(c).SyncShippedAtByMailNo(in.OrderCoreOrderID, in.MailNo, in.ShippedAt)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"updated": n})
}

func (h *Handlers) UpsertKdzsFromSync(c *gin.Context) {
	var in dto.UpsertKdzsFromSyncDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.shipment(c).UpsertKdzsFromSync(&in)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, item)
}

// ── Pending orders proxy ──

func (h *Handlers) ListPendingOrders(c *gin.Context) {
	var q storesyncagent.OrderQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if q.PageNo <= 0 {
		if page, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil {
			q.PageNo = page
		}
	}
	if q.PageSize <= 0 {
		if pageSize, err := strconv.Atoi(c.DefaultQuery("pageSize", "20")); err == nil {
			q.PageSize = pageSize
		}
	}
	data, err := h.shipment(c).ListPendingOrders(c.Request.Context(), authcontext.BearerToken(c), q)
	if err != nil {
		response.Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", wrapRawJSON(data))
}

func (h *Handlers) DecryptPendingOrders(c *gin.Context) {
	var req dto.DecryptPendingOrdersDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	data, err := h.shipment(c).DecryptPendingOrders(c.Request.Context(), authcontext.BearerToken(c), req)
	if err != nil {
		response.Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", wrapRawJSON(data))
}

func wrapRawJSON(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte(`{"code":200,"message":"success","data":null}`)
	}
	return []byte(`{"code":200,"message":"success","data":` + string(raw) + `}`)
}
