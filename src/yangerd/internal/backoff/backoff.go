// Package backoff provides exponential backoff retry logic with
// context-aware sleep, shared across reactive monitors.
package backoff

import (
	"context"
	"math"
	"time"
)

// Backoff implements exponential backoff with a configurable initial
// delay, maximum delay, and growth factor.
type Backoff struct {
	Initial time.Duration
	Max     time.Duration
	Factor  float64
}

// Default returns a Backoff with the standard yangerd parameters:
// 100ms initial, 30s max, factor 2.
func Default() *Backoff {
	return &Backoff{
		Initial: 100 * time.Millisecond,
		Max:     30 * time.Second,
		Factor:  2.0,
	}
}

// Next returns the next delay value after current.  If current is
// zero, Initial is returned.
func (b *Backoff) Next(current time.Duration) time.Duration {
	if current <= 0 {
		return b.Initial
	}
	next := time.Duration(math.Min(float64(current)*b.Factor, float64(b.Max)))
	if next <= 0 {
		return b.Initial
	}
	return next
}

// Sleep waits for duration d or until ctx is cancelled, whichever
// comes first.  Returns ctx.Err() if the context was cancelled.
func Sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
