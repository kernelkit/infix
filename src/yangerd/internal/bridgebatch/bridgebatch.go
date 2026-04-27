// Package bridgebatch manages a persistent `bridge -json -batch -`
// subprocess for querying bridge FDB, VLAN, MDB, and STP state.
// Identical design to ipbatch: mutex-serialized queries, dead/alive
// state management, and exponential backoff restart.
//
// Like ipbatch, `bridge -batch -` produces NO stdout for commands
// that fail.  Query uses a read timeout to detect this and kills
// the subprocess so restartLoop can recover.
package bridgebatch

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
var ErrBatchDead = errors.New("bridge batch process is dead")

const (
	canaryCommand = "vlan show"

	queryTimeout     = 5 * time.Second
	reconnectInitial = 100 * time.Millisecond
	reconnectMax     = 30 * time.Second
	reconnectFactor  = 2.0
)

// BridgeBatch wraps a persistent `bridge -json -batch -` subprocess.
type BridgeBatch struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan []byte
	stderr io.ReadCloser
	mu     sync.Mutex
	alive  atomic.Bool
	log    *slog.Logger
	ctx    context.Context
	cancel context.CancelFunc
}

// New spawns the bridge batch subprocess.
func New(ctx context.Context, log *slog.Logger) (*BridgeBatch, error) {
	ctx, cancel := context.WithCancel(ctx)
	b := &BridgeBatch{
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

func (b *BridgeBatch) start() error {
	cmd := exec.CommandContext(b.ctx, "bridge", "-json", "-batch", "-")
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
		return fmt.Errorf("start bridge batch: %w", err)
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

func (b *BridgeBatch) readLines(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 4*1024*1024), 4*1024*1024)
	for scanner.Scan() {
		line := make([]byte, len(scanner.Bytes()))
		copy(line, scanner.Bytes())
		b.lines <- line
	}
	b.alive.Store(false)
}

// Query sends a command to the bridge batch process and returns the
// JSON response.
func (b *BridgeBatch) Query(command string) (json.RawMessage, error) {
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
			return nil, fmt.Errorf("bridge batch process exited")
		}
		return json.RawMessage(line), nil
	case <-time.After(queryTimeout):
		b.log.Warn("bridge batch query timeout, killing subprocess", "cmd", command)
		b.alive.Store(false)
		if b.cmd != nil && b.cmd.Process != nil {
			b.cmd.Process.Kill()
		}
		return nil, fmt.Errorf("timeout waiting for response to: %s", command)
	}
}

// Close terminates the subprocess and cancels the restart loop.
func (b *BridgeBatch) Close() {
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

// Status returns "running" or "restarting".
func (b *BridgeBatch) Status() string {
	if b.alive.Load() {
		return "running"
	}
	return "restarting"
}

func (b *BridgeBatch) drainStderr() {
	scanner := bufio.NewScanner(b.stderr)
	for scanner.Scan() {
		b.log.Warn("bridge batch stderr", "line", scanner.Text())
	}
}

func (b *BridgeBatch) restartLoop() {
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

		b.log.Info("bridge batch: subprocess died, restarting", "delay", delay)
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
			b.log.Warn("bridge batch: restart failed", "err", err)
			delay = time.Duration(math.Min(
				float64(delay)*reconnectFactor,
				float64(reconnectMax)))
			continue
		}

		if _, err := b.Query(canaryCommand); err != nil {
			b.log.Warn("bridge batch: canary query failed", "err", err)
			b.alive.Store(false)
			delay = time.Duration(math.Min(
				float64(delay)*reconnectFactor,
				float64(reconnectMax)))
			continue
		}

		b.log.Info("bridge batch: restarted successfully")
		delay = reconnectInitial
	}
}
