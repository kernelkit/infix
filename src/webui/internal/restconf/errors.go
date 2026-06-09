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

// IsDataMissing reports whether err carries the RESTCONF "data-missing"
// error-tag, returned when an operation targets a leaf or container that
// isn't present in the datastore (e.g. a reset on a leaf that was never
// set).  Callers use this to swallow no-op failures so the UI stays
// consistent regardless of whether the leaf was already absent.
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
