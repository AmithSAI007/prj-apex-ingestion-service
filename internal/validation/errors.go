package validation

import "errors"

var (
	ErrPathInjection       = errors.New("potential path injection detected in GCS object path")
	ErrInvalidObjectPath   = errors.New("GCS object name doesn't match {userId}/{videoId}/{fileName} pattern")
	ErrInvalidUUID         = errors.New("value is not a valid UUID")
	ErrUnexpectedEventType = errors.New("CloudEvent type is not object.finalized")
	ErrStaleEvent          = errors.New("CloudEvent timestamp exceeds the allowed age threshold")
	ErrUnexpectedBucket    = errors.New("Event came from wrong bucket")
	ErrFileTooSmall        = errors.New("file below minimum size threshold")
	ErrFileTooLarge        = errors.New("file exceeds maximum size threshold")
	ErrUnsupportedFormat   = errors.New("magic bytes doesn't match expected video format")
	ErrInvalidTimestamp    = errors.New("invalid timestamp format in CloudEvent")
	ErrInvalidJSON         = errors.New("invalid JSON in request body")
)
