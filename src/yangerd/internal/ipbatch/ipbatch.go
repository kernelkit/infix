// Package ipbatch manages a persistent `ip -json [-s] [-d] -force -batch -`
// subprocess.  Commands sent via Query are serialized by a mutex and
// paired with the single JSON-array line the subprocess writes to
// stdout.  The caller chooses global flags via functional options:
// WithStats adds -s (statistics) and WithDetails adds -d (details).
//
// IMPORTANT: When -s is present, `link show` commands produce multiple
// lines of output — breaking the one-command-one-line protocol used by
// Query.  Address queries must therefore use a separate IPBatch instance
// that omits -s (use WithDetails only).
//
// IMPORTANT: `ip -force -batch -` produces NO stdout for commands that
// fail (e.g. "link show dev <nonexistent>").  Query uses a read timeout
// to detect this and kills the subprocess so restartLoop can recover.
//
// On subprocess death the manager enters a dead state and attempts
// automatic restart with exponential backoff.
package ipbatch

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// ErrBatchDead is returned by Query when the subprocess is not running.
// Callers should treat it as transient and retry on the next event.
var ErrBatchDead = errors.New("ip batch process is dead")

const (
	canaryCommand = "link show lo"

	queryTimeout     = 5 * time.Second
	reconnectInitial = 100 * time.Millisecond
	reconnectMax     = 30 * time.Second
	reconnectFactor  = 2.0
)

// Option configures an IPBatch instance.
type Option func(*IPBatch)

// WithStats adds -s (statistics) to the ip command.
func WithStats() Option { return func(b *IPBatch) { b.stats = true } }

// WithDetails adds -d (details) to the ip command.
func WithDetails() Option { return func(b *IPBatch) { b.details = true } }

// IPBatch wraps a persistent `ip -json -force -batch -` subprocess.
type IPBatch struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan []byte
	stderr io.ReadCloser
	mu     sync.Mutex // serializes queries
	alive  atomic.Bool
	log    *slog.Logger
	ctx    context.Context
	cancel context.CancelFunc

	stats   bool
	details bool
}

// New spawns the ip batch subprocess.  The returned IPBatch is ready
// for Query calls.  A background goroutine drains stderr.
func New(ctx context.Context, log *slog.Logger, opts ...Option) (*IPBatch, error) {
	ctx, cancel := context.WithCancel(ctx)
	b := &IPBatch{
		log:    log,
		ctx:    ctx,
		cancel: cancel,
	}
	for _, o := range opts {
		o(b)
	}
	if err := b.start(); err != nil {
		cancel()
		return nil, err
	}
	go b.restartLoop()
	return b, nil
}

func (b *IPBatch) start() error {
	args := []string{"-json"}
	if b.stats {
		args = append(args, "-s")
	}
	if b.details {
		args = append(args, "-d")
	}
	args = append(args, "-force", "-batch", "-")

	cmd := exec.CommandContext(b.ctx, "ip", args...)
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
		return fmt.Errorf("start ip batch: %w", err)
	}
	b.mu.Lock()
	b.cmd = cmd
	b.stdin = stdin
	b.lines = make(chan []byte, 8)
	b.stderr = stderr
	b.alive.Store(true)
	b.mu.Unlock()
	go b.readLines(stdout)
	go b.drainStderr()
	return nil
}

func (b *IPBatch) readLines(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 4*1024*1024), 4*1024*1024)
	for scanner.Scan() {
		line := make([]byte, len(scanner.Bytes()))
		copy(line, scanner.Bytes())
		b.lines <- line
	}
	b.alive.Store(false)
}

// Query sends a command to the ip batch process and returns the JSON
// response.  Commands are newline-terminated (e.g. "link show dev eth0").
// Each command produces exactly one line of JSON array output.  If the
// subprocess produces no output (e.g. querying a non-existent device),
// Query times out and kills the subprocess for recovery.
func (b *IPBatch) Query(command string) (json.RawMessage, error) {
	if !b.alive.Load() {
		return nil, ErrBatchDead
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.alive.Load() {
		return nil, ErrBatchDead
	}

	if _, err := fmt.Fprintf(b.stdin, "%s\n", command); err != nil {
		b.alive.Store(false)
		return nil, fmt.Errorf("write command: %w", err)
	}

	select {
	case line, ok := <-b.lines:
		if !ok {
			b.alive.Store(false)
			return nil, fmt.Errorf("ip batch process exited")
		}
		b.log.Debug("ipbatch query", "cmd", command, "respLen", len(line))
		return json.RawMessage(line), nil
	case <-time.After(queryTimeout):
		b.log.Warn("ip batch query timeout, killing subprocess", "cmd", command)
		b.alive.Store(false)
		if b.cmd != nil && b.cmd.Process != nil {
			b.cmd.Process.Kill()
		}
		return nil, fmt.Errorf("timeout waiting for response to: %s", command)
	}
}

// Close terminates the subprocess and cancels the restart loop.
func (b *IPBatch) Close() {
	b.cancel()
	b.mu.Lock()
	if b.stdin != nil {
		b.stdin.Close()
	}
	if b.cmd != nil && b.cmd.Process != nil {
		b.cmd.Process.Kill()
	}
	b.alive.Store(false)
	b.mu.Unlock()
}

// Status returns "running", "restarting", or "failed".
func (b *IPBatch) Status() string {
	if b.alive.Load() {
		return "running"
	}
	return "restarting"
}

func (b *IPBatch) drainStderr() {
	scanner := bufio.NewScanner(b.stderr)
	for scanner.Scan() {
		b.log.Warn("ip batch stderr", "line", scanner.Text())
	}
}

// restartLoop runs in the background and respawns the subprocess when
// it dies.  Uses exponential backoff: 100ms initial, 30s max, 2x factor.
// After a successful restart, a canary query validates the new process.
func (b *IPBatch) restartLoop() {
	delay := reconnectInitial
	for {
		select {
		case <-b.ctx.Done():
			return
		default:
		}

		if b.alive.Load() {
			select {
			case <-b.ctx.Done():
				return
			case <-time.After(200 * time.Millisecond):
				continue
			}
		}

		b.log.Info("ip batch: subprocess died, restarting", "delay", delay)
		select {
		case <-b.ctx.Done():
			return
		case <-time.After(delay):
		}

		b.mu.Lock()
		if b.cmd != nil && b.cmd.Process != nil {
			b.cmd.Process.Kill()
			b.cmd.Wait()
		}
		b.mu.Unlock()

		if err := b.start(); err != nil {
			b.log.Warn("ip batch: restart failed", "err", err)
			delay = time.Duration(math.Min(
				float64(delay)*reconnectFactor,
				float64(reconnectMax)))
			continue
		}

		if _, err := b.Query(canaryCommand); err != nil {
			b.log.Warn("ip batch: canary query failed", "err", err)
			b.alive.Store(false)
			delay = time.Duration(math.Min(
				float64(delay)*reconnectFactor,
				float64(reconnectMax)))
			continue
		}

		b.log.Info("ip batch: restarted successfully")
		delay = reconnectInitial
	}
}
