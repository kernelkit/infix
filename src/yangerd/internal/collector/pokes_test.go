package collector

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

type countingCollector struct {
	name  string
	count atomic.Int32
}

func (c *countingCollector) Name() string            { return c.name }
func (c *countingCollector) Interval() time.Duration { return time.Hour }
func (c *countingCollector) Collect(context.Context, *tree.Tree) error {
	c.count.Add(1)
	return nil
}

func waitCount(t *testing.T, c *countingCollector, want int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c.count.Load() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s collected %d times, want %d", c.name, c.count.Load(), want)
}

// A poke reaches the collector it names, and PokeAll reaches every one.
func TestPokesReachTheirCollector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()

	a := &countingCollector{name: "a"}
	b := &countingCollector{name: "b"}
	pokes := RunAll(ctx, &wg, tree.New(), []Collector{a, b})
	waitCount(t, a, 1)
	waitCount(t, b, 1)

	pokes.Poke("b")
	waitCount(t, b, 2)
	time.Sleep(50 * time.Millisecond)
	if a.count.Load() != 1 {
		t.Fatalf("poke for b ran a")
	}

	pokes.PokeAll()
	waitCount(t, a, 2)
	waitCount(t, b, 3)
}
