package dto

// ErrorCode represents standardized machine-readable error codes returned in
// API error responses. Clients can switch on these codes to handle specific
// error conditions programmatically.
//
// Possible values:
//   - invalid_request: The request is malformed, missing required fields, or violates domain rules. Do not retry.
//   - internal: An unexpected server-side error occurred. Safe to retry with back-off.
type ErrorCode string

const (
	// ErrorCodeInvalidRequest indicates a client error: malformed payload,
	// missing required headers, or a domain validation failure. The request
	// should not be retried without modification.
	ErrorCodeInvalidRequest ErrorCode = "invalid_request"
	// ErrorCodeInternal indicates a transient server-side error. The request
	// may succeed if retried with exponential back-off.
	ErrorCodeInternal ErrorCode = "internal"
)

// ErrorDetail provides field-level error information, typically for validation failures.
type ErrorDetail struct {
	// Field is the request field that caused the error (omitted when not field-specific).
	Field string `json:"field,omitempty" example:"fileName"`
	// Message is a human-readable description of the error.
	Message string `json:"message" example:"fileName is required"`
}

// ErrorResponse is the top-level envelope for all API error responses.
// Every non-2xx response body conforms to this schema.
type ErrorResponse struct {
	// Error contains the structured error payload.
	Error ErrorPayload `json:"error"`
}

// ErrorPayload carries the structured error information returned to API clients.
type ErrorPayload struct {
	// Code is the machine-readable error classification.
	Code ErrorCode `json:"code" example:"invalid_request" enums:"invalid_request,internal"`
	// Message is a human-readable summary of the error suitable for display.
	Message string `json:"message" example:"Validation failed"`
	// RequestID is the correlation ID for tracing this request across logs and observability tools.
	RequestID string `json:"requestId,omitempty" example:"req_6f1a2c9d5e7b3a1c"`
	// Details contains field-level error information when applicable.
	Details []ErrorDetail `json:"details,omitempty"`
}
