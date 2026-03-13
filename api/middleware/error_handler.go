package middleware

import (
	"net/http"

	"github.com/AmithSAI007/prj-apex-ingestion-service/api/dto"
	"github.com/AmithSAI007/prj-apex-ingestion-service/pkg/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ErrorHandler returns a Gin middleware that provides two layers of protection:
//  1. Panic recovery: catches panics from downstream handlers and returns a
//     structured 500 JSON error response instead of crashing the server.
//  2. Context error handling: after the handler chain completes, checks for
//     any accumulated Gin context errors and returns a 400 JSON response.
//
// This middleware should be registered early in the middleware stack (after
// RequestContext) so that panic recovery and error formatting cover all
// downstream handlers.
func ErrorHandler(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {

		// Deferred panic recovery: catches panics and returns a 500 error.
		defer func() {
			if r := recover(); r != nil {
				traceID := utils.TraceIDFromContext(c.Request.Context())
				logger.Error("Recovered from panic", zap.Any("error", r), zap.String("trace_id", traceID))
				c.AbortWithStatusJSON(
					http.StatusInternalServerError,
					dto.ErrorResponse{
						Error: dto.ErrorPayload{
							Code:      dto.ErrorCodeInternal,
							Message:   "Internal Server Error",
							RequestID: traceID,
						},
					},
				)
			}
		}()

		c.Next()

		// After the handler chain: check for any Gin context errors that
		// were not explicitly handled by the endpoint handler.
		if len(c.Errors) > 0 {
			traceID := utils.TraceIDFromContext(c.Request.Context())
			for _, e := range c.Errors {
				logger.Error("Request error", zap.Error(e.Err), zap.String("trace_id", traceID))
			}
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{
				Error: dto.ErrorPayload{
					Code:      dto.ErrorCodeInvalidArgument,
					Message:   "Bad Request",
					RequestID: traceID,
				},
			})
		}

	}
}
