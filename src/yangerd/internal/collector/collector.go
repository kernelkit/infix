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

// Pokes asks running collectors for an immediate collection.  Each
// collector has its own channel, so a poke reaches the one it is meant
// for, and pokes that arrive while it is busy collapse into one.
type Pokes struct {
	chans map[string]chan struct{}
}

// Poke asks the named collector to collect now.
func (p *Pokes) Poke(name string) {
	if ch, ok := p.chans[name]; ok {
		poke(ch)
	}
}

// PokeAll asks every collector to collect now.
func (p *Pokes) PokeAll() {
	for _, ch := range p.chans {
		poke(ch)
	}
}

func poke(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// RunAll starts one goroutine per Collector, each ticking at the
// collector's configured interval.  A failed Collect is logged and
// retried on the next tick.  All goroutines exit when ctx is cancelled.
func RunAll(ctx context.Context, wg *sync.WaitGroup, t *tree.Tree, collectors []Collector) *Pokes {
	p := &Pokes{chans: make(map[string]chan struct{}, len(collectors))}
	for _, c := range collectors {
		ch := make(chan struct{}, 1)
		p.chans[c.Name()] = ch
		wg.Add(1)
		go runOne(ctx, wg, t, c, ch)
	}
	return p
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
