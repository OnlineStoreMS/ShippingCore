package admin

import (
	"net/http"
	"strconv"

	"shippingcore/internal/dto"
	"shippingcore/internal/integrations/ordercore"
	"shippingcore/internal/integrations/storesyncagent"
	"shippingcore/internal/pkg/authcontext"
	"shippingcore/internal/pkg/httputil"
	"shippingcore/internal/pkg/response"
	"shippingcore/internal/service"

	"github.com/gin-gonic/gin"
)

func (h *Handlers) kdzs(c *gin.Context) *service.KdzsService {
	return h.Kdzs.ForTenant(authcontext.TenantID(c))
}

func (h *Handlers) ListKdzsAccounts(c *gin.Context) {
	items, err := h.kdzs(c).ListAccounts(c.Request.Context(), authcontext.BearerToken(c))
	if err != nil {
		response.Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	response.OK(c, gin.H{"items": items, "total": len(items)})
}

func (h *Handlers) ListKdzsAccountDetails(c *gin.Context) {
	items, err := h.kdzs(c).ListAccountDetails(c.Request.Context(), authcontext.BearerToken(c))
	if err != nil {
		response.Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	response.OK(c, gin.H{"items": items, "total": len(items)})
}

func (h *Handlers) SyncKdzsAccounts(c *gin.Context) {
	stats, err := h.kdzs(c).SyncAccountsFromSSA(c.Request.Context(), authcontext.BearerToken(c), true)
	if err != nil {
		response.Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	response.OK(c, stats)
}

func (h *Handlers) CreateKdzsAccount(c *gin.Context) {
	var in storesyncagent.KdzsAccountInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.kdzs(c).CreateAccount(c.Request.Context(), authcontext.BearerToken(c), in)
	if err != nil {
		response.Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	response.Created(c, item)
}

func (h *Handlers) UpdateKdzsAccount(c *gin.Context) {
	id := c.Param("id")
	var in storesyncagent.KdzsAccountUpdateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.kdzs(c).UpdateAccount(c.Request.Context(), authcontext.BearerToken(c), id, in)
	if err != nil {
		response.Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	response.OK(c, item)
}

func (h *Handlers) DeleteKdzsAccount(c *gin.Context) {
	id := c.Param("id")
	if err := h.kdzs(c).DeleteAccount(c.Request.Context(), authcontext.BearerToken(c), id); err != nil {
		response.Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	response.OK(c, gin.H{"ok": true})
}

func (h *Handlers) SetDefaultKdzsAccount(c *gin.Context) {
	var req storesyncagent.AccountIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.kdzs(c).SetDefaultAccount(c.Request.Context(), authcontext.BearerToken(c), req.AccountID); err != nil {
		response.Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	response.OK(c, gin.H{"ok": true})
}

func (h *Handlers) SwitchKdzsAccount(c *gin.Context) {
	var req storesyncagent.AccountIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.kdzs(c).SwitchAccount(c.Request.Context(), authcontext.BearerToken(c), req.AccountID); err != nil {
		response.Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	response.OK(c, gin.H{"ok": true})
}

func (h *Handlers) GetBatchPrintURL(c *gin.Context) {
	platform := c.Query("platform")
	if platform == "" {
		response.Fail(c, http.StatusBadRequest, "platform is required")
		return
	}
	data, err := h.kdzs(c).GetBatchPrintURL(c.Request.Context(), authcontext.BearerToken(c), platform)
	if err != nil {
		response.Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", wrapRawJSON(data))
}

func (h *Handlers) QueryPrintWaybills(c *gin.Context) {
	var body storesyncagent.PrintWaybillQueryRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	data, err := h.kdzs(c).QueryPrintWaybills(c.Request.Context(), authcontext.BearerToken(c), body)
	if err != nil {
		response.Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", wrapRawJSON(data))
}

func (h *Handlers) SyncKdzsPrintAssets(c *gin.Context) {
	stats, err := h.kdzs(c).SyncPrintAssets(c.Request.Context(), authcontext.BearerToken(c))
	if err != nil {
		response.Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	response.OK(c, stats)
}

func (h *Handlers) ListExpressTemplates(c *gin.Context) {
	page, pageSize := httputil.ParsePage(c)
	list, total, err := h.kdzs(c).ListExpressTemplates(page, pageSize, c.Query("platform"))
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.OK(c, response.PageResult(list, total, page, pageSize))
}

func (h *Handlers) ListWaybillAuths(c *gin.Context) {
	page, pageSize := httputil.ParsePage(c)
	list, total, err := h.kdzs(c).ListWaybillAuths(page, pageSize)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.OK(c, response.PageResult(list, total, page, pageSize))
}

func (h *Handlers) ListPendingOMSOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	q := ordercore.OrderQuery{
		Page:           page,
		PageSize:       pageSize,
		ShipStatus:     c.DefaultQuery("shipStatus", "wait_ship"),
		AllocType:      c.DefaultQuery("allocType", "self_ship"),
		Platform:       c.Query("platform"),
		Keyword:        c.Query("keyword"),
		PlatformSysTid: c.Query("platformSysTid"),
		SourceChannel:  c.Query("sourceChannel"),
	}
	data, err := h.shipment(c).ListPendingOMSOrders(c.Request.Context(), authcontext.BearerToken(c), q)
	if err != nil {
		response.Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", wrapRawJSON(data))
}

func (h *Handlers) ConfirmKdzsShip(c *gin.Context) {
	var in dto.ConfirmKdzsShipDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	data, err := h.shipment(c).ConfirmKdzsShip(c.Request.Context(), authcontext.BearerToken(c), &in)
	if err != nil {
		response.Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	response.OK(c, data)
}
