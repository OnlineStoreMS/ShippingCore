package admin

import "github.com/gin-gonic/gin"

func RegisterRoutes(g *gin.RouterGroup, h *Handlers) {
	g.GET("/carrier-accounts", h.ListCarrierAccounts)
	g.POST("/carrier-accounts", h.CreateCarrierAccount)
	g.GET("/carrier-accounts/:id", h.GetCarrierAccount)
	g.PUT("/carrier-accounts/:id", h.UpdateCarrierAccount)
	g.DELETE("/carrier-accounts/:id", h.DeleteCarrierAccount)

	g.GET("/shipper-profiles", h.ListShipperProfiles)
	g.POST("/shipper-profiles", h.CreateShipperProfile)
	g.GET("/shipper-profiles/:id", h.GetShipperProfile)
	g.PUT("/shipper-profiles/:id", h.UpdateShipperProfile)
	g.DELETE("/shipper-profiles/:id", h.DeleteShipperProfile)
	g.POST("/shipper-profiles/:id/set-default", h.SetDefaultShipperProfile)

	g.GET("/shipments", h.ListShipments)
	g.GET("/shipments/:id", h.GetShipment)
	g.POST("/shipments/from-order", h.CreateShipmentFromOrder)
	g.POST("/shipments/:id/create-waybill", h.CreateShipmentWaybill)
	g.POST("/shipments/:id/print", h.PrintShipment)
	g.POST("/shipments/:id/cancel", h.CancelShipment)

	g.GET("/pending-orders", h.ListPendingOrders)
	g.POST("/pending-orders/decrypt", h.DecryptPendingOrders)
}
