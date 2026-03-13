// Package api defines the HTTP route configuration and handler registry
// for the Apex Upload Platform API.
package api

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/swaggo/files"
	"github.com/swaggo/gin-swagger"

	"github.com/AmithSAI007/prj-apex-ingestion-service/api/handler"
	"github.com/AmithSAI007/prj-apex-ingestion-service/docs"
)

// HandlerRegistry holds the handler and middleware instances needed for route
// registration. It acts as the dependency injection point for the HTTP layer.
type HandlerRegistry struct {
	EventHandler *handler.EventHandler
}

// SetupRoutes registers all API endpoints on the given Gin router.
//
// Protected routes (under /api/v1/uploads) require JWT authentication via the
// AuthMiddleware. Public routes include Swagger UI, the raw Swagger JSON spec,
// and Prometheus metrics.
func SetupRoutes(router *gin.Engine, handlers *HandlerRegistry) {
	v1 := router.Group("/api/v1")

	v1.GET("/healthz", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{"status": "ok"})
	})

	v1.GET("/", handlers.EventHandler.HandleGCSObjectFinalizedEvent)

	// Swagger UI served at /api/v1/swagger/index.html.
	v1.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.URL("/docs/doc.json")))

	// Raw OpenAPI JSON spec for programmatic consumers.
	router.GET("/docs/doc.json", func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Content-Type", "application/json")
		ctx.Writer.WriteHeader(200)
		if _, err := ctx.Writer.Write([]byte(docs.SwaggerInfo.ReadDoc())); err != nil {
			ctx.AbortWithStatus(500)
		}
	})

	// Prometheus metrics endpoint.
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

}
