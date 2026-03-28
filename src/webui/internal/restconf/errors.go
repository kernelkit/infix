// SPDX-License-Identifier: MIT

package restconf

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

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
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	re := &Error{StatusCode: resp.StatusCode}

	// Try to parse the standard RESTCONF error envelope.
	var envelope struct {
		Errors struct {
			Error []struct {
				ErrorType    string `json:"error-type"`
				ErrorTag     string `json:"error-tag"`
				ErrorMessage string `json:"error-message"`
			} `json:"error"`
		} `json:"ietf-restconf:errors"`
	}

	if json.Unmarshal(body, &envelope) == nil && len(envelope.Errors.Error) > 0 {
		first := envelope.Errors.Error[0]
		re.Type = first.ErrorType
		re.Tag = first.ErrorTag
		re.Message = first.ErrorMessage
	} else {
		re.Message = http.StatusText(resp.StatusCode)
	}

	return re
}
