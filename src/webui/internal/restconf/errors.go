// SPDX-License-Identifier: MIT

package restconf

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// IsNotFound reports whether err is a RESTCONF error with HTTP status 404.
// Used to distinguish "data resource absent" (expected for delete-or-create
// flows) from real failures.
func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.StatusCode == http.StatusNotFound
}

// IsDataMissing reports whether err is a RESTCONF "data-missing" tag error
// (RFC 8040 §7.6.2). Returned by DELETE on a leaf that is already absent.
// Useful when the caller is explicitly trying to reach an "absent" state
// and treats already-absent the same as just-deleted.
func IsDataMissing(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Tag == "data-missing"
}

// AuthError is returned when RESTCONF rejects credentials (401/403).
type AuthError struct {
	Code int
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("authentication failed (HTTP %d)", e.Code)
}

// Error represents a RESTCONF error response.
type Error struct {
	StatusCode int
	Type       string
	Tag        string
	Message    string
}

func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("restconf %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("restconf %d: %s", e.StatusCode, e.Tag)
}

// parseError reads a RESTCONF error response body and returns an *Error.
func parseError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))

	re := &Error{StatusCode: resp.StatusCode}

	// Try to parse the standard RESTCONF error envelope.
	var envelope struct {
		Errors struct {
			Error []struct {
				ErrorType    string `json:"error-type"`
				ErrorTag     string `json:"error-tag"`
				ErrorPath    string `json:"error-path"`
				ErrorMessage string `json:"error-message"`
				ErrorInfo    any    `json:"error-info"`
			} `json:"error"`
		} `json:"ietf-restconf:errors"`
	}

	if json.Unmarshal(body, &envelope) == nil && len(envelope.Errors.Error) > 0 {
		var parts []string
		for _, e := range envelope.Errors.Error {
			msg := e.ErrorMessage
			if msg == "" {
				msg = e.ErrorTag
			}
			if e.ErrorPath != "" {
				msg += " (path: " + e.ErrorPath + ")"
			}
			parts = append(parts, msg)
		}
		re.Type = envelope.Errors.Error[0].ErrorType
		re.Tag = envelope.Errors.Error[0].ErrorTag
		re.Message = strings.Join(parts, "; ")
	} else {
		re.Message = http.StatusText(resp.StatusCode)
	}

	return re
}
