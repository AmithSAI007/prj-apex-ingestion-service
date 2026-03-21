package validation

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
)

var (
	dangerousPathPatterns = []string{
		"..",
		"/",
		"\\",
		"\x00",
		"%2f",
		"%2F",
		"%00",
		"%2e%2e",
	}
	uuidRegex       = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
	videoSignatures = []magicSignature{
		{Format: "mp4", Offset: 4, Magic: []byte("ftyp")},
		{Format: "mkv", Offset: 0, Magic: []byte{0x1A, 0x45, 0xDF, 0xA3}},
		{Format: "avi", Offset: 0, Magic: []byte("RIFF")},
		{Format: "flv", Offset: 0, Magic: []byte("FLV")},
		{Format: "ts", Offset: 0, Magic: []byte{0x47}},
		{Format: "mpeg", Offset: 0, Magic: []byte{0x00, 0x00, 0x01, 0xBA}},
	}
)

const FinalizedEventType string = "OBJECT_FINALIZE"

type magicSignature struct {
	Format string
	Offset int
	Magic  []byte
}

type Validator struct {
	logger *zap.Logger
}

func NewValidator(logger *zap.Logger) *Validator {
	return &Validator{
		logger: logger,
	}
}

func (v *Validator) validateUUID(value string) bool {
	return uuidRegex.MatchString(value)
}

func (v *Validator) validateNoPathInjection(name string) error {
	for _, pattern := range dangerousPathPatterns {
		if strings.Contains(name, pattern) {
			return ErrPathInjection
		}
	}
	return nil
}

func (v *Validator) ValidatePathSegment(field, value string) error {
	if !v.validateUUID(value) {
		return fmt.Errorf("invalid %s: %w", field, ErrInvalidUUID)
	}
	if err := v.validateNoPathInjection(value); err != nil {
		return fmt.Errorf("invalid %s: %w", field, err)
	}
	return nil
}

func (v *Validator) ParseObjectPath(name string) (UserID, VideoID, fileName string, err error) {
	parts := strings.SplitN(name, "/", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("object name must have the format userId/videoId/fileName: %w", ErrInvalidObjectPath)
	}
	userId, videoId, fileName := parts[0], parts[1], parts[2]
	return userId, videoId, fileName, nil

}

func (v *Validator) ValidateEventType(eventType string) error {
	if eventType != FinalizedEventType {
		return ErrUnexpectedEventType
	}
	return nil
}

func (v *Validator) ValidateEventAge(timestamp string) error {
	parsedTime, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return ErrInvalidTimestamp
	}

	if time.Since(parsedTime) > 24*time.Hour {
		return ErrStaleEvent
	}

	return nil
}

func (v *Validator) ValidateBucket(bucket string, expectedBucketName string) error {
	if bucket != expectedBucketName {
		return ErrUnexpectedBucket
	}
	return nil
}

func (v *Validator) ValidateFileSize(size int64, maxSize int64, minSize int64) error {
	if size < minSize {
		return ErrFileTooSmall
	}
	if size > maxSize {
		return ErrFileTooLarge
	}
	return nil
}

func (v *Validator) ValidateMagicBytes(header []byte, allowedFormats []string) (string, error) {
	if len(header) < 12 {
		return "", fmt.Errorf("%w: file header too short (%d bytes)", ErrUnsupportedFormat, len(header))
	}

	detectedFormat := ""
	for _, sig := range videoSignatures {
		end := sig.Offset + len(sig.Magic)
		if end > len(header) {
			continue
		}

		if bytes.Equal(header[sig.Offset:end], sig.Magic) {
			detectedFormat = sig.Format

			if sig.Format == "mkv" {
				detectedFormat = v.detectMatroskaVariant(header)
			}

			if sig.Format == "avi" {
				if len(header) >= 12 && bytes.Equal(header[8:12], []byte("AVI ")) {
					continue
				}
			}

			if sig.Format == "mp4" {
				detectedFormat = v.detectMp4Variant(header)
			}
			break
		}
	}

	if detectedFormat == "" {
		return "", fmt.Errorf("%w: no matching video format found in header", ErrUnsupportedFormat)
	}

	if !v.isFormatAllowed(detectedFormat, allowedFormats) {
		return detectedFormat, fmt.Errorf("%w: detected format '%s' is not in allowed formats %v", ErrUnsupportedFormat, detectedFormat, allowedFormats)
	}

	return detectedFormat, nil

}

func (v *Validator) detectMatroskaVariant(header []byte) string {
	docTypeId := []byte{0x42, 0x82} // DocType ID in EBML

	for i := 0; i < len(header)-10; i++ {
		if bytes.Equal(header[i:i+2], docTypeId) {
			valueStart := i + 3
			remaining := header[valueStart:]

			if bytes.HasPrefix(remaining, []byte("matroska")) {
				return "mkv"
			}
			if bytes.HasPrefix(remaining, []byte("webm")) {
				return "webm"
			}
			break
		}
	}

	return "mkv"
}

func (v *Validator) isFormatAllowed(format string, allowedFormats []string) bool {
	for _, allowed := range allowedFormats {
		if format == allowed {
			return true
		}
	}
	return false
}

func (v *Validator) detectMp4Variant(header []byte) string {
	if len(header) < 12 {
		return "mp4"
	}

	brand := string(header[8:12])

	switch brand {
	case "qt  ":
		return "mov"
	default:
		return "mp4"
	}
}
