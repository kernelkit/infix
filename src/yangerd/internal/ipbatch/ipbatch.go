// Package ipbatch manages a persistent `ip -json ... -force -batch -` or
// `bridge -json -force -batch -` subprocess.  Commands sent via Query are
// serialized by a mutex and paired with the single JSON-array line the
// subprocess writes to stdout.
//
// With -force a failing command (e.g. "link show dev <gone>") writes no
// stdout line, only "Command failed -:<N>" on stderr, where N is its
// line number in the batch.  Query counts the commands it sends and
// treats that stderr line as the answer, so a vanished device costs one
// round trip instead of a timeout and a subprocess restart.
//
// When -s is present, `link show` commands produce multiple lines of
// output, breaking the one-command-one-line protocol, so address queries
// must use a separate instance without WithStats.
package ipbatch

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/backoff"
)

// ErrBatchDead is returned by Query when the subprocess is not running.
// Callers should treat it as transient and retry on the next event.
var ErrBatchDead = errors.New("batch process is dead")

// ErrCommandFailed is returned by Query when the subprocess rejected the
// command, typically because the device it names does not exist.
var ErrCommandFailed = errors.New("batch command failed")

const queryTimeout = 5 * time.Second

var failedRe = regexp.MustCompile(`^Command failed -:(\d+)$`)

// Option configures an ip batch instance.
type Option func(*[]string)

// WithStats adds -s (statistics) to the ip command.
func WithStats() Option { return func(a *[]string) { *a = append(*a, "-s") } }

// WithDetails adds -d (details) to the ip command.
func WithDetails() Option { return func(a *[]string) { *a = append(*a, "-d") } }

// Batch wraps one persistent batch subprocess.
type Batch struct {
	argv   []string
	canary string
	log    *slog.Logger
	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex // serializes queries, guards the fields below
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan []byte
	failed chan int
	seq    int // commands written to the current subprocess

	alive atomic.Bool
	gen   atomic.Int64  // bumped per subprocess, so a stale reader cannot kill a new one
	died  chan struct{} // kicked when the subprocess goes away
}

// New starts `ip -json [opts] -force -batch -`.
func New(ctx context.Context, log *slog.Logger, opts ...Option) (*Batch, error) {
	args := []string{"-json"}
	for _, o := range opts {
		o(&args)
	}
	return Start(ctx, log, append([]string{"ip"}, append(args, "-force", "-batch", "-")...), "link show lo")
}

// NewBridge starts `bridge -json -force -batch -`.
func NewBridge(ctx context.Context, log *slog.Logger) (*Batch, error) {
	return Start(ctx, log, []string{"bridge", "-json", "-force", "-batch", "-"}, "vlan show dev lo")
}

// Start runs argv as a batch subprocess and keeps it running, restarting
// it with backoff when it dies.  canary is a command that must succeed,
// used to validate a restarted subprocess.
func Start(ctx context.Context, log *slog.Logger, argv []string, canary string) (*Batch, error) {
	ctx, cancel := context.WithCancel(ctx)
	b := &Batch{
		argv:   argv,
		canary: canary,
		log:    log,
		ctx:    ctx,
		cancel: cancel,
		died:   make(chan struct{}, 1),
	}
	if err := b.start(); err != nil {
		cancel()
		return nil, err
	}
	go b.restartLoop()
	return b, nil
}

func (b *Batch) start() error {
	cmd := exec.CommandContext(b.ctx, b.argv[0], b.argv[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s batch: %w", b.argv[0], err)
	}

	lines := make(chan []byte, 8)
	failed := make(chan int, 8)
	gen := b.gen.Add(1)
	b.mu.Lock()
	b.cmd = cmd
	b.stdin = stdin
	b.lines = lines
	b.failed = failed
	b.seq = 0
	b.alive.Store(true)
	b.mu.Unlock()

	go b.readLines(stdout, lines, gen)
	go b.readStderr(stderr, failed)
	return nil
}

func (b *Batch) readLines(r io.Reader, lines chan<- []byte, gen int64) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 4*1024*1024), 4*1024*1024)
	for scanner.Scan() {
		lines <- append([]byte(nil), scanner.Bytes()...)
	}
	close(lines)
	if b.gen.Load() == gen {
		b.markDead()
	}
}

func (b *Batch) readStderr(r io.Reader, failed chan<- int) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if m := failedRe.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			failed <- n
			continue
		}
		b.log.Debug(b.argv[0]+" batch stderr", "line", line)
	}
}

func (b *Batch) markDead() {
	b.alive.Store(false)
	select {
	case b.died <- struct{}{}:
	default:
	}
}

// Query sends a command to the batch process and returns its JSON
// response, ErrCommandFailed if the subprocess rejected it, or
// ErrBatchDead if the subprocess is gone.
func (b *Batch) Query(command string) (json.RawMessage, error) {
	if !b.alive.Load() {
		return nil, ErrBatchDead
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.alive.Load() {
		return nil, ErrBatchDead
	}

	if _, err := fmt.Fprintf(b.stdin, "%s\n", command); err != nil {
		b.markDead()
		return nil, fmt.Errorf("write command: %w", err)
	}
	b.seq++

	timeout := time.NewTimer(queryTimeout)
	defer timeout.Stop()
	for {
		select {
		case line, ok := <-b.lines:
			if !ok {
				return nil, ErrBatchDead
			}
			return json.RawMessage(line), nil
		case n := <-b.failed:
			if n != b.seq {
				continue // a stale report for an earlier command
			}
			return nil, fmt.Errorf("%w: %s", ErrCommandFailed, command)
		case <-timeout.C:
			b.log.Warn(b.argv[0]+" batch query timeout, killing subprocess", "cmd", command)
			b.markDead()
			if b.cmd.Process != nil {
				b.cmd.Process.Kill()
			}
			return nil, fmt.Errorf("timeout waiting for response to: %s", command)
		}
	}
}

// Close terminates the subprocess and cancels the restart loop.
func (b *Batch) Close() {
	b.cancel()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stdin != nil {
		b.stdin.Close()
	}
	if b.cmd != nil && b.cmd.Process != nil {
		b.cmd.Process.Kill()
	}
	b.alive.Store(false)
}

// restartLoop respawns the subprocess when it dies, with exponential
// backoff, validating each new process with the canary command.
func (b *Batch) restartLoop() {
	bo := backoff.Default()
	delay := bo.Initial
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-b.died:
		}

		for !b.alive.Load() {
			b.log.Info(b.argv[0]+" batch: subprocess died, restarting", "delay", delay)
			if backoff.Sleep(b.ctx, delay) != nil {
				return
			}
			delay = bo.Next(delay)

			b.mu.Lock()
			if b.cmd != nil && b.cmd.Process != nil {
				b.cmd.Process.Kill()
				b.cmd.Wait()
			}
			b.mu.Unlock()

			if err := b.start(); err != nil {
				b.log.Warn(b.argv[0]+" batch: restart failed", "err", err)
				continue
			}
			if _, err := b.Query(b.canary); err != nil {
				b.log.Warn(b.argv[0]+" batch: canary query failed", "err", err)
				b.markDead()
				continue
			}
			b.log.Info(b.argv[0] + " batch: restarted")
			delay = bo.Initial
		}
	}
}
