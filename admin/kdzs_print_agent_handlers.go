package admin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"shippingcore/internal/pkg/authcontext"
	"shippingcore/internal/pkg/response"
	"shippingcore/internal/service"

	"github.com/gin-gonic/gin"
)

type KdzsPrintAgentHandler struct {
	svc *service.KdzsPrintAgentService
}

func NewKdzsPrintAgentHandler(svc *service.KdzsPrintAgentService) *KdzsPrintAgentHandler {
	return &KdzsPrintAgentHandler{svc: svc}
}

func (h *KdzsPrintAgentHandler) adminSvc(c *gin.Context) *service.KdzsPrintAgentService {
	return h.svc.ForTenant(authcontext.TenantID(c))
}

func writePrintAgentErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrPairCodeInvalid):
		response.Fail(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrDeviceAuth):
		response.Fail(c, http.StatusUnauthorized, err.Error())
	case errors.Is(err, service.ErrDeviceOffline):
		response.Fail(c, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrDeviceNotFound), errors.Is(err, service.ErrNotFound):
		response.Fail(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrNoTask):
		response.OK(c, gin.H{"task": nil})
	case errors.Is(err, service.ErrBadRequest):
		response.Fail(c, http.StatusBadRequest, err.Error())
	default:
		msg := err.Error()
		if strings.Contains(msg, "ErrBadRequest") || strings.HasPrefix(msg, "bad request") {
			response.Fail(c, http.StatusBadRequest, msg)
			return
		}
		response.Fail(c, http.StatusInternalServerError, msg)
	}
}

func deviceCreds(c *gin.Context) (key, secret string) {
	key = strings.TrimSpace(c.GetHeader("X-KDZS-Device-Key"))
	secret = strings.TrimSpace(c.GetHeader("X-KDZS-Device-Secret"))
	if key == "" {
		key = strings.TrimSpace(c.Query("deviceKey"))
	}
	if secret == "" {
		secret = strings.TrimSpace(c.Query("deviceSecret"))
	}
	return key, secret
}

// CreatePairOffer POST /mobile/kdzs-print/pair-sessions （扩展用，无需登录）
func (h *KdzsPrintAgentHandler) CreatePairOffer(c *gin.Context) {
	var body struct {
		DeviceName string `json:"deviceName"`
	}
	_ = c.ShouldBindJSON(&body)
	res, err := h.svc.CreatePairOffer(body.DeviceName)
	if err != nil {
		writePrintAgentErr(c, err)
		return
	}
	response.OK(c, res)
}

// ClaimPair POST /admin/kdzs-print/pair-claim （手机登录后输入电脑配对码）
func (h *KdzsPrintAgentHandler) ClaimPair(c *gin.Context) {
	var body struct {
		PairCode string `json:"pairCode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	dto, err := h.adminSvc(c).ClaimPair(authcontext.UserID(c), body.PairCode)
	if err != nil {
		writePrintAgentErr(c, err)
		return
	}
	response.OK(c, dto)
}

// ListDevices GET /admin/kdzs-print/devices
func (h *KdzsPrintAgentHandler) ListDevices(c *gin.Context) {
	list, err := h.adminSvc(c).ListDevices()
	if err != nil {
		writePrintAgentErr(c, err)
		return
	}
	response.OK(c, gin.H{"list": list, "total": len(list)})
}

// RenameDevice PUT /admin/kdzs-print/devices/:id
func (h *KdzsPrintAgentHandler) RenameDevice(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var body struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	dto, err := h.adminSvc(c).RenameDevice(id, body.Name)
	if err != nil {
		writePrintAgentErr(c, err)
		return
	}
	response.OK(c, dto)
}

// UnbindDevice DELETE /admin/kdzs-print/devices/:id
func (h *KdzsPrintAgentHandler) UnbindDevice(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.adminSvc(c).UnbindDevice(id); err != nil {
		writePrintAgentErr(c, err)
		return
	}
	response.OK(c, gin.H{"ok": true})
}

// CreateTask POST /admin/kdzs-print/tasks
func (h *KdzsPrintAgentHandler) CreateTask(c *gin.Context) {
	var in service.CreatePrintTaskInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	dto, err := h.adminSvc(c).CreateTask(authcontext.UserID(c), &in)
	if err != nil {
		writePrintAgentErr(c, err)
		return
	}
	response.OK(c, dto)
}

// ListTasks GET /admin/kdzs-print/tasks
func (h *KdzsPrintAgentHandler) ListTasks(c *gin.Context) {
	list, err := h.adminSvc(c).ListRecentTasks(20)
	if err != nil {
		writePrintAgentErr(c, err)
		return
	}
	response.OK(c, gin.H{"list": list, "total": len(list)})
}

// CompletePair 已废弃：配对改为电脑出码、手机认领。
func (h *KdzsPrintAgentHandler) CompletePair(c *gin.Context) {
	response.Fail(c, http.StatusGone, "请升级扩展：由电脑生成配对码，手机输入绑定")
}

// Heartbeat POST /mobile/kdzs-print/heartbeat
func (h *KdzsPrintAgentHandler) Heartbeat(c *gin.Context) {
	key, secret := deviceCreds(c)
	dto, err := h.svc.Heartbeat(key, secret)
	if err != nil {
		writePrintAgentErr(c, err)
		return
	}
	response.OK(c, dto)
}

// ClaimTask POST /mobile/kdzs-print/tasks/claim
func (h *KdzsPrintAgentHandler) ClaimTask(c *gin.Context) {
	key, secret := deviceCreds(c)
	dto, err := h.svc.ClaimNext(key, secret)
	if err != nil {
		if errors.Is(err, service.ErrNoTask) {
			response.OK(c, gin.H{"task": nil})
			return
		}
		writePrintAgentErr(c, err)
		return
	}
	response.OK(c, gin.H{"task": dto})
}

// ReportTask POST /mobile/kdzs-print/tasks/:id/report
func (h *KdzsPrintAgentHandler) ReportTask(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var in service.ReportTaskInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	key, secret := deviceCreds(c)
	dto, err := h.svc.ReportTask(key, secret, id, &in)
	if err != nil {
		writePrintAgentErr(c, err)
		return
	}
	response.OK(c, dto)
}
