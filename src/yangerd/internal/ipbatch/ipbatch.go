// Package ipbatch manages a persistent `ip -json -s -d -force -batch -`
// subprocess.  Commands sent via Query are serialized by a mutex and
// paired with the single JSON-array line the subprocess writes to
// stdout.  The -s and -d global flags ensure link queries include
// statistics and details.  On subprocess death the manager enters a
// dead state and attempts automatic restart with exponential backoff.
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

	reconnectInitial = 100 * time.Millisecond
	reconnectMax     = 30 * time.Second
	reconnectFactor  = 2.0
)

// IPBatch wraps a persistent `ip -json -force -batch -` subprocess.
type IPBatch struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	stderr io.ReadCloser
	mu     sync.Mutex // serializes queries
	alive  atomic.Bool
	log    *slog.Logger
	ctx    context.Context
	cancel context.CancelFunc
}

// New spawns the ip batch subprocess.  The returned IPBatch is ready
// for Query calls.  A background goroutine drains stderr.
func New(ctx context.Context, log *slog.Logger) (*IPBatch, error) {
	ctx, cancel := context.WithCancel(ctx)
	b := &IPBatch{
		log:    log,
		ctx:    ctx,
		cancel: cancel,
	}
	if err := b.start(); err != nil {
		cancel()
		return nil, err
	}
	go b.restartLoop()
	return b, nil
}

func (b *IPBatch) start() error {
	cmd := exec.CommandContext(b.ctx, "ip", "-json", "-s", "-d", "-force", "-batch", "-")
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
	b.stdout = bufio.NewScanner(stdout)
	b.stdout.Buffer(make([]byte, 0, 4*1024*1024), 4*1024*1024) // 4 MiB max line
	b.stderr = stderr
	b.alive.Store(true)
	b.mu.Unlock()
	go b.drainStderr()
	return nil
}

// Query sends a command to the ip batch process and returns the JSON
// response.  Commands are newline-terminated (e.g. "link show dev eth0").
// Each command produces exactly one line of JSON array output.
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
	if !b.stdout.Scan() {
		b.alive.Store(false)
		if err := b.stdout.Err(); err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		return nil, fmt.Errorf("ip batch process exited")
	}
	raw := make([]byte, len(b.stdout.Bytes()))
	copy(raw, b.stdout.Bytes())
	return json.RawMessage(raw), nil
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
			// Wait for death or context cancellation.
			// Poll periodically since there's no notification channel.
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

		// Kill old process if lingering.
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

		// Canary query to validate the new process.
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
