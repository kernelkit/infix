package handlers

import (
	"strings"
	"testing"
	"time"
)

func TestFormatLastChange(t *testing.T) {
	if got := formatLastChange(""); got != "" {
		t.Errorf("empty: got %q", got)
	}
	if got := formatLastChange("garbage"); got != "garbage" {
		t.Errorf("unparsable: got %q", got)
	}

	stamp := time.Now().Add(-90 * time.Second).UTC().Format("2006-01-02T15:04:05+00:00")
	got := formatLastChange(stamp)
	if !strings.HasSuffix(got, "(1m ago)") {
		t.Errorf("age: got %q", got)
	}
}
