package apperror

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PermanentError struct {
	Err error
}

type TransientError struct {
	Err error
}

type IdempotencyError struct {
	Err error
}

func NewPermanentError(err error) *PermanentError {
	return &PermanentError{Err: err}
}
func (e *PermanentError) Error() string {
	return e.Err.Error()
}
func (e *PermanentError) Unwrap() error {
	return e.Err
}

func NewTransientError(err error) *TransientError {
	return &TransientError{Err: err}
}
func (e *TransientError) Error() string {
	return e.Err.Error()
}
func (e *TransientError) Unwrap() error {
	return e.Err
}

func NewIdempotencyError(err error) *IdempotencyError {
	return &IdempotencyError{Err: err}
}
func (e *IdempotencyError) Error() string {
	return e.Err.Error()
}
func (e *IdempotencyError) Unwrap() error {
	return e.Err
}

var permanentCodes = map[codes.Code]bool{
	codes.PermissionDenied: true,
	codes.Unauthenticated:  true,
	codes.InvalidArgument:  true,
	codes.NotFound:         true,
}

func IsPermanent(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	return permanentCodes[status.Code(err)]
}

func ClassifyError(msg string, err error) error {
	if IsPermanent(err) {
		return NewPermanentError(fmt.Errorf("%s: %w", msg, err))
	}
	return NewTransientError(fmt.Errorf("%s: %w", msg, err))
}
