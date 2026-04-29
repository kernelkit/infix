// Package collector defines the Collector interface and the RunAll
// scheduler that drives periodic data collection into the Tree.
package collector

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

// Collector gathers operational data and writes it to the Tree.
type Collector interface {
	Name() string
	Interval() time.Duration
	Collect(ctx context.Context, t *tree.Tree) error
}

// RunAll starts one goroutine per Collector, each ticking at the
// collector's configured interval.  A failed Collect is logged and
// retried on the next tick.  All goroutines exit when ctx is cancelled.
func RunAll(ctx context.Context, wg *sync.WaitGroup, t *tree.Tree, collectors []Collector, pokeCh <-chan struct{}) {
	for _, c := range collectors {
		wg.Add(1)
		go runOne(ctx, wg, t, c, pokeCh)
	}
}

func runOne(ctx context.Context, wg *sync.WaitGroup, t *tree.Tree, c Collector, pokeCh <-chan struct{}) {
	defer wg.Done()

	if err := c.Collect(ctx, t); err != nil {
		log.Printf("collector %s: initial: %v", c.Name(), err)
	}

	ticker := time.NewTicker(c.Interval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.Collect(ctx, t); err != nil {
				log.Printf("collector %s: %v", c.Name(), err)
			}
		case <-pokeCh:
			if err := c.Collect(ctx, t); err != nil {
				log.Printf("collector %s: poke: %v", c.Name(), err)
			}
		}
	}
}
