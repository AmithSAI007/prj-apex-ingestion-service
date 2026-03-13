// Package trace provides application-level trace ID generation and context
// propagation utilities. These trace IDs are separate from OpenTelemetry's
// distributed trace IDs and serve as human-readable correlation identifiers
// in logs and API error responses.
package utils

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/AmithSAI007/prj-apex-ingestion-service/pkg/constants"
)

// GenerateTraceID returns a cryptographically-random 16-character hex string
// suitable for use as a request correlation ID. Falls back to a zero-filled
// string if the system's random source is unavailable.
func GenerateTraceID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(buffer)
}

// ContextWithTraceID stores the given trace ID in the provided context under
// the CtxTraceIDKey and returns a new child context.
func ContextWithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, constants.CtxTraceIDKey, id)
}

// TraceIDFromContext extracts the application-level trace ID from a context.
// Returns an empty string if the context is nil or does not contain a trace ID.
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if value := ctx.Value(constants.CtxTraceIDKey); value != nil {
		if id, ok := value.(string); ok {
			return id
		}
	}
	return ""
}

// DataFromContext is a generic helper that extracts a string value from the
// context for the given key. It is used to retrieve user_id, tenant_id, and
// other request-scoped strings. Returns an empty string if the key is absent
// or the value is not a string.
func DataFromContext(ctx context.Context, key interface{}) string {
	if ctx == nil {
		return ""
	}
	if value := ctx.Value(key); value != nil {
		if id, ok := value.(string); ok {
			return id
		}
	}
	return ""
}
