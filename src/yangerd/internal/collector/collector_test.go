package collector

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

type panicky struct{}

func (panicky) Name() string                                    { return "panicky" }
func (panicky) Interval() time.Duration                         { return time.Hour }
func (panicky) Collect(ctx context.Context, t *tree.Tree) error { panic("bad read") }

func TestCollectRecoversPanic(t *testing.T) {
	err := collect(context.Background(), tree.New(), panicky{})
	if err == nil || !strings.Contains(err.Error(), "bad read") {
		t.Fatalf("got %v, want the panic as an error", err)
	}
}
