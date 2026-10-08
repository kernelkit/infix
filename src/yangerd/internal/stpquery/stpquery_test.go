package stpquery

import (
	"testing"
	"time"
)

// The topology-change time only moves when the change count does.
func TestTCTimeStableUntilCountChanges(t *testing.T) {
	first := tcTime("br-test", 3, 10)
	time.Sleep(1100 * time.Millisecond)
	if again := tcTime("br-test", 3, 11); !again.Equal(first) {
		t.Fatalf("time moved without a new change: %v -> %v", first, again)
	}
	if next := tcTime("br-test", 4, 0); !next.After(first) {
		t.Fatalf("new change kept the old time: %v", next)
	}
}
