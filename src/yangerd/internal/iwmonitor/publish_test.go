package iwmonitor

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// A refresh that started before a disconnect must not publish over it:
// the last word on an interface is the operation that began last.
func TestClearNotOverwrittenByInflightRefresh(t *testing.T) {
	m := New(slog.Default())

	var pubMu sync.Mutex
	var pubs []string
	m.SetOnUpdate(func(_ string, data json.RawMessage) {
		pubMu.Lock()
		pubs = append(pubs, string(data))
		pubMu.Unlock()
	})

	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	m.meshState = func(string) meshState {
		once.Do(func() {
			close(started)
			<-release
		})
		return meshState{iftype: "station"}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		m.refreshInterface(context.Background(), "wtest0")
	}()
	<-started
	go func() {
		defer wg.Done()
		m.clearInterface("wtest0")
	}()
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	pubMu.Lock()
	defer pubMu.Unlock()
	if len(pubs) != 2 {
		t.Fatalf("published %d times, want 2: %v", len(pubs), pubs)
	}
	if pubs[1] != "{}" {
		t.Fatalf("in-flight refresh overwrote the clear: %v", pubs)
	}
}
