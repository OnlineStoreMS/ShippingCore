package router

import (
	"shippingcore/admin"
	adminmw "shippingcore/admin/middleware"
	"shippingcore/internal/config"
	"shippingcore/internal/integrations/ordercore"
	"shippingcore/internal/integrations/storesyncagent"
	jwtmgr "shippingcore/internal/pkg/jwt"
	"shippingcore/internal/repo"
	"shippingcore/internal/service"
	"shippingcore/internal/storage"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func Setup(db *gorm.DB, cfg *config.Config) *gin.Engine {
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), corsMiddleware(cfg))

	store, err := storage.New(&cfg.Storage)
	if err != nil {
		panic("storage init: " + err.Error())
	}
	if cfg.Storage.Driver != "minio" {
		r.Static("/uploads", cfg.Storage.LocalPath)
	}

	repos := repo.New(db)
	carrierSvc := service.NewCarrierService(repos)
	shipperSvc := service.NewShipperService(repos)
	ssAgent := storesyncagent.NewClient(cfg.Integrations.StoreSyncAgentAPIURL)
	orderCore := ordercore.NewClient(cfg.Integrations.OrderCoreAPIURL)
	shipmentSvc := service.NewShipmentService(repos, carrierSvc, shipperSvc, ssAgent, orderCore, store)
	kdzsSvc := service.NewKdzsService(repos, ssAgent)
	h := admin.NewHandlers(carrierSvc, shipperSvc, shipmentSvc, kdzsSvc)
	uploadH := admin.NewUploadHandler(store)
	photoH := admin.NewPhotoUploadHandler(store)
	kdzsHandoffH := admin.NewKdzsHelperHandoffHandler()
	kdzsPrintAgentSvc := service.NewKdzsPrintAgentService(repos)
	kdzsPrintAgentH := admin.NewKdzsPrintAgentHandler(kdzsPrintAgentSvc)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "shippingcore"})
	})

	v1 := r.Group("/api/v1")
	adminGroup := v1.Group("/admin")
	jwtMgr := jwtmgr.NewManager(cfg.Auth.JWTSecret)
	adminGroup.Use(adminmw.AdminAuth(&cfg.Auth, jwtMgr))
	admin.RegisterRoutes(adminGroup, h)
	adminGroup.POST("/upload", uploadH.Upload)
	adminGroup.POST("/photo-upload-sessions", photoH.CreateSession)
	adminGroup.GET("/photo-upload-sessions/:token", photoH.GetSession)
	adminGroup.POST("/kdzs/helper-handoff-sessions", kdzsHandoffH.CreateSession)

	adminGroup.POST("/kdzs-print/pair-claim", kdzsPrintAgentH.ClaimPair)
	adminGroup.GET("/kdzs-print/devices", kdzsPrintAgentH.ListDevices)
	adminGroup.PUT("/kdzs-print/devices/:id", kdzsPrintAgentH.RenameDevice)
	adminGroup.DELETE("/kdzs-print/devices/:id", kdzsPrintAgentH.UnbindDevice)
	adminGroup.POST("/kdzs-print/tasks", kdzsPrintAgentH.CreateTask)
	adminGroup.GET("/kdzs-print/tasks", kdzsPrintAgentH.ListTasks)

	mobile := v1.Group("/mobile")
	mobile.GET("/photo-upload/:token", photoH.MobileGet)
	mobile.POST("/photo-upload/:token", photoH.MobileUpload)
	mobile.GET("/kdzs-helper-handoff/:token", kdzsHandoffH.MobileGet)
	mobile.POST("/kdzs-print/pair-sessions", kdzsPrintAgentH.CreatePairOffer)
	mobile.POST("/kdzs-print/pair", kdzsPrintAgentH.CompletePair) // 旧扩展兼容提示
	mobile.POST("/kdzs-print/heartbeat", kdzsPrintAgentH.Heartbeat)
	mobile.POST("/kdzs-print/tasks/claim", kdzsPrintAgentH.ClaimTask)
	mobile.POST("/kdzs-print/tasks/:id/report", kdzsPrintAgentH.ReportTask)

	return r
}

func corsMiddleware(cfg *config.Config) gin.HandlerFunc {
	origins := cfg.CORS.AllowOrigins
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := origin == ""
		for _, o := range origins {
			if o == origin || o == "*" {
				allowed = true
				break
			}
		}
		if allowed && origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
		}
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization,X-KDZS-Device-Key,X-KDZS-Device-Secret")
		c.Header("Access-Control-Allow-Credentials", "true")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
