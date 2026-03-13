package middleware

import (
	"strings"

	"github.com/AmithSAI007/prj-apex-ingestion-service/pkg/utils"
	"github.com/gin-gonic/gin"
)

const (
	// RequestIDHeader is the HTTP header used to propagate request correlation IDs.
	// If the client sends this header, its value is preserved; otherwise, a new ID is generated.
	RequestIDHeader = "X-Request-ID"
	// TraceIDKey is the Gin context key where the trace ID is stored for handler access.
	TraceIDKey = "trace_id"
)

// RequestContext returns a Gin middleware that ensures every request has a
// correlation ID. If the client sends an X-Request-ID header, that value is
// used; otherwise, a new "req_<random>" ID is generated. The ID is:
//   - Stored in the Gin context under TraceIDKey.
//   - Stored in the Go request context for downstream service/store layers.
//   - Echoed back in the X-Request-ID response header.
func RequestContext() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		requestID := strings.TrimSpace(ctx.GetHeader(RequestIDHeader))
		if requestID == "" {
			requestID = "req_" + utils.GenerateTraceID()
		}
		ctx.Set(TraceIDKey, requestID)
		ctx.Request = ctx.Request.WithContext(utils.ContextWithTraceID(ctx.Request.Context(), requestID))
		ctx.Writer.Header().Set(RequestIDHeader, requestID)
		ctx.Next()
	}
}
