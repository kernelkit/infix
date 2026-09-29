package lldpmonitor

import (
	"strings"
	"testing"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

// A brace inside a peer's free-text description must not stall the
// framing: the event after it still triggers a refresh.
func TestReadEventsBraceInDescription(t *testing.T) {
	m := New(tree.New(), nil)
	m.refresh = make(chan struct{}, 8)

	stream := `{
  "lldp-added": {
    "lldp": [{"interface": [{"name": "e1",
      "chassis": [{"descr": [{"value": "switch {rack 4"}]}]}]}]
  }
}

{"lldp-deleted": {"lldp": []}}
`
	err := m.readEvents(strings.NewReader(stream))
	if err == nil || !strings.Contains(err.Error(), "exited") {
		t.Fatalf("readEvents at EOF = %v", err)
	}
	if n := len(m.refresh); n != 2 {
		t.Fatalf("refreshes triggered = %d, want 2", n)
	}
}
