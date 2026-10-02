package ptpmonitor

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

// An instance removed and re-added across scans is stopped and started
// cleanly: every start has its cancel func before run sees it.
func TestScanStopStart(t *testing.T) {
	dir := t.TempDir()
	oldConf, oldSock := confDir, sockDir
	confDir, sockDir = dir, dir
	t.Cleanup(func() { confDir, sockDir = oldConf, oldSock })

	conf := filepath.Join(dir, "ptp4l-0.conf")
	m := New(tree.New(), slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	var started []*instance
	defer func() {
		cancel()
		for _, inst := range started {
			<-inst.done
		}
	}()

	for i := 0; i < 20; i++ {
		os.WriteFile(conf, []byte("[global]\n[e1]\n"), 0644)
		m.scan(ctx)
		m.mu.Lock()
		inst := m.instances[0]
		m.mu.Unlock()
		if inst == nil || inst.cancel == nil {
			t.Fatal("instance started without a cancel func")
		}
		started = append(started, inst)

		os.Remove(conf)
		m.scan(ctx)
	}

	m.mu.Lock()
	n := len(m.instances)
	m.mu.Unlock()
	if n != 0 {
		t.Fatalf("instances left after removal: %d", n)
	}
}
