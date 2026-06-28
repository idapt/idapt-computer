package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

const (
	ExitOK          = 0
	ExitError       = 1
	ExitAuth        = 2
	ExitForbidden   = 3
	ExitNotFound    = 4
	ExitValidation  = 5
	ExitSpendingCap = 6
	ExitRateLimit   = 10
)

const (
	ErrTypeInvalidRequest     = "invalid_request"
	ErrTypeUnauthorized       = "unauthorized"
	ErrTypeForbidden          = "forbidden"
	ErrTypeNotFound           = "not_found"
	ErrTypeConflict           = "conflict"
	ErrTypeRateLimit          = "rate_limit"
	ErrTypeServiceUnavailable = "service_unavailable"
	ErrTypeInternalError      = "internal_error"

	ErrCodeSpendingCapExceeded = "spending_cap_exceeded"
	ErrCodeModelNotAvailableForTier = "model_not_available_for_tier"
)

type APIError struct {
	StatusCode int
	Type       string // coarse error category (error.type)
	Code       string // optional finer sub-code (error.code), may be empty
	Message    string
	Retryable  bool
	RetryAfter int    // seconds, from Retry-After header
	Reason     string // server-supplied reason tag (e.g. "per_period_cap")
	Hint       string // human-facing hint (e.g. "Upgrade your plan")
	RequestID  string // x-request-id, for support correlation
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		if e.Type != "" {
			msg = fmt.Sprintf("API error %d: %s", e.StatusCode, e.Type)
		} else {
			msg = fmt.Sprintf("API error %d", e.StatusCode)
		}
	}
	if e.RequestID != "" && e.StatusCode >= 500 {
		msg += fmt.Sprintf(" (request id: %s)", e.RequestID)
	}
	return msg
}

func (e *APIError) ExitCode() int {
	switch {
	case e.StatusCode == 401:
		return ExitAuth
	case e.StatusCode == 402:
		return ExitSpendingCap
	case e.StatusCode == 403:
		return ExitForbidden
	case e.StatusCode == 404:
		return ExitNotFound
	case e.StatusCode == 422 || e.StatusCode == 400:
		return ExitValidation
	case e.StatusCode == 429:
		return ExitRateLimit
	default:
		return ExitError
	}
}

func IsAuthError(err error) bool {
	if e, ok := err.(*APIError); ok {
		return e.StatusCode == 401
	}
	return false
}

func IsNotFoundError(err error) bool {
	if e, ok := err.(*APIError); ok {
		return e.StatusCode == 404
	}
	return false
}

func IsRateLimitError(err error) bool {
	if e, ok := err.(*APIError); ok {
		return e.StatusCode == 429
	}
	return false
}

func IsServiceUnavailableError(err error) bool {
	if e, ok := err.(*APIError); ok {
		return e.StatusCode == 503 || e.Type == ErrTypeServiceUnavailable
	}
	return false
}

func IsSpendingCapError(err error) bool {
	if e, ok := err.(*APIError); ok {
		return e.StatusCode == 402 &&
			(e.Code == ErrCodeSpendingCapExceeded || e.Type == ErrCodeSpendingCapExceeded)
	}
	return false
}

func parseErrorResponse(resp *http.Response) *APIError {
	apiErr := &APIError{StatusCode: resp.StatusCode}
	apiErr.RequestID = resp.Header.Get("x-request-id")

	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil {
			apiErr.RetryAfter = secs
		}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil || len(body) == 0 {
		apiErr.Message = http.StatusText(resp.StatusCode)
		return apiErr
	}

	var envelope struct {
		Error struct {
			Type      string `json:"type"`
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
			Reason    string `json:"reason"`
			Hint      string `json:"hint"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil && envelope.Error.Message != "" {
		apiErr.Type = envelope.Error.Type
		apiErr.Code = envelope.Error.Code
		apiErr.Message = envelope.Error.Message
		apiErr.Retryable = envelope.Error.Retryable
		apiErr.Reason = envelope.Error.Reason
		apiErr.Hint = envelope.Error.Hint
		if apiErr.StatusCode == http.StatusServiceUnavailable ||
			envelope.Error.Type == ErrTypeServiceUnavailable {
			apiErr.Retryable = true
		}
		return apiErr
	}

	var simple struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &simple) == nil && simple.Error != "" {
		apiErr.Message = simple.Error
		return apiErr
	}

	msg := string(body)
	if len(msg) > 256 {
		msg = msg[:256] + "..."
	}
	apiErr.Message = msg
	return apiErr
}
