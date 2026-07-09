package modelgateway

import (
	"context"
	"errors"
	"fmt"
)

type ErrorCode string

const (
	ErrorCodeTimeout             ErrorCode = "timeout"
	ErrorCodeInvalidResponse     ErrorCode = "invalid_response"
	ErrorCodeUnsupportedProvider ErrorCode = "unsupported_provider"
	ErrorCodeConfiguration       ErrorCode = "configuration_error"
	ErrorCodeProviderFailure     ErrorCode = "provider_failure"
)

type Error struct {
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	Retryable bool      `json:"retryable"`
	Cause     error     `json:"-"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NewError(code ErrorCode, message string, retryable bool, cause error) *Error {
	return &Error{
		Code:      code,
		Message:   message,
		Retryable: retryable,
		Cause:     cause,
	}
}

func NormalizeError(err error) *Error {
	if err == nil {
		return nil
	}
	var gatewayErr *Error
	if errors.As(err, &gatewayErr) {
		return gatewayErr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewError(ErrorCodeTimeout, "model gateway timed out", true, err)
	}
	return NewError(ErrorCodeProviderFailure, fmt.Sprintf("model gateway failed: %v", err), true, err)
}

func IsRetryableError(err error) bool {
	normalized := NormalizeError(err)
	return normalized != nil && normalized.Retryable
}
