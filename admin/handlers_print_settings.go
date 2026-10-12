package admin

import (
	"net/http"

	"shippingcore/internal/pkg/authcontext"
	"shippingcore/internal/pkg/httputil"
	"shippingcore/internal/pkg/response"
	"shippingcore/internal/service"

	"github.com/gin-gonic/gin"
)

func (h *Handlers) printSettings(c *gin.Context) *service.PrintSettingsService {
	return h.PrintSettings.ForTenant(authcontext.TenantID(c))
}

func (h *Handlers) GetPrintSettings(c *gin.Context) {
	data, err := h.printSettings(c).Get()
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, data)
}

func (h *Handlers) UpdatePrintSettings(c *gin.Context) {
	var in service.UpdatePrintSettingsInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	data, err := h.printSettings(c).Update(&in)
	if err != nil {
		httputil.HandleServiceError(c, err)
		return
	}
	response.OK(c, data)
}
