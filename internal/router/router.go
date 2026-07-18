package router

import (
	"shippingcore/admin"
	adminmw "shippingcore/admin/middleware"
	"shippingcore/internal/config"
	"shippingcore/internal/integrations/storesyncagent"
	jwtmgr "shippingcore/internal/pkg/jwt"
	"shippingcore/internal/repo"
	"shippingcore/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func Setup(db *gorm.DB, cfg *config.Config) *gin.Engine {
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), corsMiddleware(cfg))

	repos := repo.New(db)
	carrierSvc := service.NewCarrierService(repos)
	shipperSvc := service.NewShipperService(repos)
	ssAgent := storesyncagent.NewClient(cfg.Integrations.StoreSyncAgentAPIURL)
	shipmentSvc := service.NewShipmentService(repos, carrierSvc, shipperSvc, ssAgent)
	h := admin.NewHandlers(carrierSvc, shipperSvc, shipmentSvc)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "shippingcore"})
	})

	v1 := r.Group("/api/v1")
	adminGroup := v1.Group("/admin")
	jwtMgr := jwtmgr.NewManager(cfg.Auth.JWTSecret)
	adminGroup.Use(adminmw.AdminAuth(&cfg.Auth, jwtMgr))
	admin.RegisterRoutes(adminGroup, h)

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
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization")
		c.Header("Access-Control-Allow-Credentials", "true")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
