package iwmonitor

import (
	"testing"

	"github.com/kernelkit/infix/src/yangerd/internal/wpactrl"
)

func TestChannelEvent(t *testing.T) {
	for name, want := range map[string]bool{
		"AP-ENABLED":              true,
		"AP-CSA-FINISHED":         true,
		"CTRL-EVENT-CONNECTED":    true,
		"MESH-GROUP-STARTED":      true,
		"AP-STA-CONNECTED":        false,
		"CTRL-EVENT-SCAN-RESULTS": false,
	} {
		if got := channelEvent(wpactrl.Event{Name: name}); got != want {
			t.Errorf("channelEvent(%s) = %v, want %v", name, got, want)
		}
	}
}
