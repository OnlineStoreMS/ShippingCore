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
	g.POST("/sf/check-pickup-time", h.CheckPickupTime)
	g.POST("/sf/query-deliver-tm", h.QueryDeliverTm)

	g.GET("/shipments", h.ListShipments)
	g.POST("/shipments/delete-by-ordercore", h.DeleteShipmentsByOrderCore)
	g.POST("/shipments/sync-shipped-at", h.SyncShipmentShippedAt)
	g.POST("/shipments/upsert-kdzs-from-sync", h.UpsertKdzsFromSync)
	g.GET("/shipments/:id", h.GetShipment)
	g.GET("/shipments/:id/promise-tm", h.SearchPromiseTm)
	g.POST("/shipments/from-order", h.CreateShipmentFromOrder)
	g.POST("/shipments/:id/create-waybill", h.CreateShipmentWaybill)
	g.POST("/shipments/:id/print", h.PrintShipment)
	g.GET("/shipments/:id/label", h.DownloadShipmentLabel)
	g.GET("/shipments/:id/print-plugin-data", h.GetShipmentPrintPluginData)
	g.POST("/shipments/:id/cancel", h.CancelShipment)

	g.GET("/pending-orders", h.ListPendingOrders)
	g.POST("/pending-orders/decrypt", h.DecryptPendingOrders)
	g.GET("/pending-oms-orders", h.ListPendingOMSOrders)

	g.GET("/kdzs/accounts", h.ListKdzsAccounts)
	g.GET("/kdzs/account-details", h.ListKdzsAccountDetails)
	g.POST("/kdzs/accounts/sync", h.SyncKdzsAccounts)
	g.POST("/kdzs/accounts", h.CreateKdzsAccount)
	g.PUT("/kdzs/accounts/:id", h.UpdateKdzsAccount)
	g.DELETE("/kdzs/accounts/:id", h.DeleteKdzsAccount)
	g.POST("/kdzs/accounts/default", h.SetDefaultKdzsAccount)
	g.POST("/kdzs/accounts/switch", h.SwitchKdzsAccount)

	g.POST("/sync/kdzs-print-assets", h.SyncKdzsPrintAssets)
	g.GET("/express-templates", h.ListExpressTemplates)
	g.GET("/waybill-auths", h.ListWaybillAuths)
	g.GET("/kdzs/batch-print-url", h.GetBatchPrintURL)
	g.POST("/kdzs/print-waybills", h.QueryPrintWaybills)

	g.POST("/shipments/confirm-kdzs-ship", h.ConfirmKdzsShip)
	g.POST("/shipments/confirm-kdzs-split-ship", h.ConfirmKdzsSplitShip)
	g.POST("/shipment-groups", h.CreateShipmentGroup)
	g.GET("/shipment-groups/:id", h.GetShipmentGroup)
}
