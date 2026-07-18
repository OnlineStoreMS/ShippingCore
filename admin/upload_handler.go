package admin

import (
	"net/http"
	"path/filepath"
	"strings"

	"shippingcore/internal/pkg/response"
	"shippingcore/internal/storage"

	"github.com/gin-gonic/gin"
)

const maxImageSize = 10 << 20 // 10MB

type UploadHandler struct {
	store storage.Storage
}

func NewUploadHandler(store storage.Storage) *UploadHandler {
	return &UploadHandler{store: store}
}

func (h *UploadHandler) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "file required")
		return
	}
	if file.Size > maxImageSize {
		response.Fail(c, http.StatusBadRequest, "image too large (max 10MB)")
		return
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
	default:
		response.Fail(c, http.StatusBadRequest, "unsupported image type")
		return
	}
	subdir := c.DefaultPostForm("subdir", "products")
	subdir = strings.Trim(subdir, "/")
	if subdir == "" {
		subdir = "products"
	}
	url, err := h.store.Upload(file, subdir)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.OK(c, gin.H{"url": url})
}
