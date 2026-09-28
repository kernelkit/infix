// Package backoff provides exponential backoff retry logic with
// context-aware sleep, shared across reactive monitors.
package backoff

import (
	"context"
	"log/slog"
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

// Retry runs fn until ctx is cancelled, restarting it with exponential
// backoff each time it returns.  A run that lasted longer than the
// maximum delay counts as healthy and resets the delay, so a source
// that flaps after hours of service is retried quickly, while one that
// dies at once backs off to Max.  This is the restart loop every
// reactive monitor needs; name labels the log line.
func Retry(ctx context.Context, log *slog.Logger, name string, fn func(ctx context.Context) error) error {
	bo := Default()
	delay := bo.Initial

	for {
		started := time.Now()
		err := fn(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Since(started) > bo.Max {
			delay = bo.Initial
		}

		log.Warn(name+": exited, restarting", "err", err, "delay", delay)
		if err := Sleep(ctx, delay); err != nil {
			return err
		}
		delay = bo.Next(delay)
	}
}
