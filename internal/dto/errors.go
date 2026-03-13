package dto

// ErrorCode represents standardized machine-readable error codes returned in
// API error responses. Clients can switch on these codes to handle specific
// error conditions programmatically.
type ErrorCode string

const (
	// ErrorCodeInvalidArgument indicates malformed or invalid request parameters (HTTP 400).
	ErrorCodeInvalidArgument ErrorCode = "invalid_argument"
	// ErrorCodeUnauthorized indicates missing or invalid authentication credentials (HTTP 401).
	ErrorCodeUnauthorized ErrorCode = "unauthorized"
	// ErrorCodeForbidden indicates the caller lacks permission for the requested action (HTTP 403).
	ErrorCodeForbidden ErrorCode = "forbidden"
	// ErrorCodeNotFound indicates the requested resource does not exist (HTTP 404).
	ErrorCodeNotFound ErrorCode = "not_found"
	// ErrorCodeGone indicates the resource existed but has been permanently removed or expired (HTTP 410).
	ErrorCodeGone ErrorCode = "gone"
	// ErrorCodeConflict indicates a state conflict, such as duplicate idempotency key with different parameters (HTTP 409).
	ErrorCodeConflict ErrorCode = "conflict"
	// ErrorCodeRateLimited indicates the caller has exceeded the allowed request rate (HTTP 429).
	ErrorCodeRateLimited ErrorCode = "rate_limited"
	// ErrorCodeNotImplemented indicates the requested feature is not yet available (HTTP 501).
	ErrorCodeNotImplemented ErrorCode = "not_implemented"
	// ErrorCodeInternal indicates an unexpected server-side error (HTTP 500).
	ErrorCodeInternal ErrorCode = "internal"
	// ErrorCodeInvalidEventType indicates the event type in the request is not supported (HTTP 400).
	ErrorCodeInvalidEventType ErrorCode = "invalid_event_type"
	// ErrorCodeInvalidPath indicates the GCS object path contains invalid characters or path traversal (HTTP 400).
	ErrorCodeInvalidPath ErrorCode = "invalid_path"
	// ErrorCodeRequestTimeout indicates the CloudEvent timestamp exceeds the allowed age threshold (HTTP 408).
	ErrorCodeRequestTimeout ErrorCode = "request_timeout"
	// ErrorCodeUnsupportedMediaType indicates the file format is not supported (HTTP 415).
	ErrorCodeUnsupportedMediaType ErrorCode = "unsupported_media_type"
)

// ErrorDetail provides field-level error information, typically for validation failures.
type ErrorDetail struct {
	// Field is the request field that caused the error (omitted when not field-specific).
	Field string `json:"field,omitempty" example:"fileName"`
	// Message is a human-readable description of the error.
	Message string `json:"message" example:"fileName is required"`
}

// ErrorResponse is the top-level envelope for all API error responses.
type ErrorResponse struct {
	Error ErrorPayload `json:"error"`
}

// ErrorPayload carries the structured error information returned to API clients.
type ErrorPayload struct {
	// Code is the machine-readable error classification.
	Code ErrorCode `json:"code" example:"invalid_argument"`
	// Message is a human-readable summary of the error.
	Message string `json:"message" example:"Validation failed"`
	// RequestID is the correlation ID for tracing this request in logs and observability tools.
	RequestID string `json:"requestId,omitempty" example:"req_6f1a2c9d5e7b3a1c"`
	// Details contains field-level error information when applicable.
	Details []ErrorDetail `json:"details,omitempty"`
}
