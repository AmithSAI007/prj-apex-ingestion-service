// Package constants defines typed context keys used across the application
// to store and retrieve request-scoped values such as user identity and
// trace correlation IDs. Using a dedicated type (CtxKey) prevents collisions
// with context keys from other packages.
package constants

// CtxKey is a private string type that prevents context-key collisions with
// other packages that might use plain string keys.
type CtxKey string

const (
	// CtxUserIDKey stores the authenticated user's ID in the request context.
	// Set by the auth middleware after successful JWT validation.
	CtxUserIDKey CtxKey = "user_id"

	// CtxTenantIDKey stores the tenant ID extracted from the request context.
	// Used for multi-tenant data isolation in Firestore queries.
	CtxTenantIDKey CtxKey = "tenant_id"

	// CtxTraceIDKey stores the application-level correlation ID (distinct from
	// OpenTelemetry trace IDs) for request logging and error responses.
	CtxTraceIDKey CtxKey = "trace_id"
)
