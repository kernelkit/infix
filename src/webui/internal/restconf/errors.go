// SPDX-License-Identifier: MIT

package restconf

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
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

// IsAccessDenied reports whether the device refused the request under
// NACM, as opposed to rejecting the credentials.
func IsAccessDenied(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.StatusCode == http.StatusForbidden || e.Tag == "access-denied"
	}
	var a *AuthError
	return errors.As(err, &a) && a.Code == http.StatusForbidden
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

// rcError is one entry of an ietf-restconf errors list.
type rcError struct {
	Type    string `json:"error-type"`
	Tag     string `json:"error-tag"`
	Path    string `json:"error-path"`
	Message string `json:"error-message"`
}

type rcErrors struct {
	Error []rcError `json:"error"`
}

// parseError reads a RESTCONF error response body and returns an *Error.
// Both the plain ietf-restconf:errors envelope and the ietf-yang-patch
// status report, which nests one errors list per failed edit, are
// understood.
func parseError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))

	re := &Error{StatusCode: resp.StatusCode}

	var envelope struct {
		Errors rcErrors `json:"ietf-restconf:errors"`
		Patch  struct {
			Errors     rcErrors `json:"errors"`
			EditStatus struct {
				Edit []struct {
					Errors rcErrors `json:"errors"`
				} `json:"edit"`
			} `json:"edit-status"`
		} `json:"ietf-yang-patch:yang-patch-status"`
	}

	var errs []rcError
	if json.Unmarshal(body, &envelope) == nil {
		errs = append(errs, envelope.Errors.Error...)
		errs = append(errs, envelope.Patch.Errors.Error...)
		for _, e := range envelope.Patch.EditStatus.Edit {
			errs = append(errs, e.Errors.Error...)
		}
	}
	if len(errs) == 0 {
		re.Message = http.StatusText(resp.StatusCode)
		return re
	}

	var parts []string
	for _, e := range errs {
		msg := strings.TrimPrefix(unwrap(e.Message), "Validation failure: ")
		if msg == "" {
			msg = e.Tag
		}
		if node := leafName(e.Path); node != "" {
			msg = node + ": " + msg
			if strings.Contains(e.Path, "[") {
				msg += " (path: " + e.Path + ")"
			}
		}
		parts = append(parts, msg)
	}
	re.Type = errs[0].Type
	re.Tag = errs[0].Tag
	re.Message = strings.Join(parts, "; ")
	return re
}

// srFraming finds the message in rousette's framing of a sysrepo error:
//
//	Internal server error due to sysrepo exception: <where> failed:
//	SR_ERR_OPERATION_FAILED <msg> (SR_ERR_OPERATION_FAILED) [NETCONF: ...]
var srFraming = regexp.MustCompile(`(?s)SR_ERR_[A-Z_]+ (.*?) \(SR_ERR_[A-Z_]+\)`)

// unwrap returns the message a user can act on from a sysrepo error.
func unwrap(msg string) string {
	if m := srFraming.FindStringSubmatch(msg); m != nil {
		return m[1]
	}
	return msg
}

// leafName returns the last node name of a data path, without module
// prefix or list keys: /ietf-system:system/hostname gives "hostname".
func leafName(path string) string {
	var b strings.Builder
	var quote rune
	depth := 0
	for _, r := range path {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case depth > 0 && (r == '\'' || r == '"'):
			quote = r
		case r == '[':
			depth++
		case r == ']':
			depth--
		case depth == 0:
			b.WriteRune(r)
		}
	}
	name := b.String()
	name = name[strings.LastIndex(name, "/")+1:]
	return name[strings.LastIndex(name, ":")+1:]
}
