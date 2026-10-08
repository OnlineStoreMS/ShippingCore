package admin

import (
	"net/http"
	"strconv"
	"strings"

	"shippingcore/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// InternalAuth 服务间鉴权（AgentsCenter 拉默认快递助手账号等）。
func InternalAuth(token string) gin.HandlerFunc {
	want := strings.TrimSpace(token)
	return func(c *gin.Context) {
		got := strings.TrimSpace(c.GetHeader("X-Internal-Token"))
		if want == "" || got == "" || got != want {
			response.Fail(c, http.StatusUnauthorized, "unauthorized")
			c.Abort()
			return
		}
		c.Next()
	}
}

// InternalDefaultKdzsLogin GET /api/v1/internal/kdzs/default-login?tenantId=
func (h *KdzsPrintAgentHandler) InternalDefaultKdzsLogin(c *gin.Context) {
	tenantID, _ := strconv.ParseUint(strings.TrimSpace(c.Query("tenantId")), 10, 64)
	if tenantID == 0 {
		response.Fail(c, http.StatusBadRequest, "tenantId 必填")
		return
	}
	mobile, password, code, name, err := h.svc.DefaultPrintLogin(tenantID)
	if err != nil {
		writePrintAgentErr(c, err)
		return
	}
	response.OK(c, gin.H{
		"tenantId":    tenantID,
		"mobile":      mobile,
		"password":    password,
		"accountCode": code,
		"accountName": name,
	})
}
