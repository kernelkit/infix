package backoff

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestNextGrowsToMax(t *testing.T) {
	b := &Backoff{Initial: 100 * time.Millisecond, Max: 300 * time.Millisecond, Factor: 2}
	got := []time.Duration{b.Next(0)}
	for i := 0; i < 3; i++ {
		got = append(got, b.Next(got[len(got)-1]))
	}
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 300 * time.Millisecond, 300 * time.Millisecond}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("step %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// Retry keeps calling fn until the context ends, and returns the
// context error rather than fn's.
func TestRetryRestartsUntilCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := Retry(ctx, slog.Default(), "test", func(ctx context.Context) error {
		calls++
		if calls == 3 {
			cancel()
		}
		return errors.New("boom")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}
