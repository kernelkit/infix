// Package ipbatch manages a persistent `ip -json ... -force -batch -` or
// `bridge -json -force -batch -` subprocess.  Commands sent via Query are
// serialized by a mutex and each is paired with the JSON line the
// subprocess writes for it.
//
// Pairing cannot rely on one line per command: depending on the iproute2
// version a failing command writes nothing, "[]" or "[{}]" before its
// "Command failed -:<N>" (N is the line number in the batch), and with
// stdout and stderr on separate pipes the two race.  So the subprocess
// gets both on one pipe, which keeps its write order, and every command
// is followed by a sentinel that always fails.  A query is answered by
// the lines that arrive before the sentinel's failure: a failure for the
// command's own line number means ErrCommandFailed, otherwise the last
// JSON line is the answer.
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
	"os"
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

const (
	queryTimeout = 5 * time.Second

	// sentinel is an object neither ip nor bridge knows, so it fails
	// with only a "Command failed" line and never any JSON.
	sentinel = "sentinel"
)

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

	mu    sync.Mutex // serializes queries, guards the fields below
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan []byte
	quit  chan struct{} // closed when this subprocess is replaced
	seq   int           // lines written to the current subprocess

	// restartMu serializes Refresh and the restart loop, so a process
	// one of them just started is never killed by the other.
	restartMu sync.Mutex

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

// start spawns a new subprocess and makes it current.  The previous
// one, if any, is left to the caller to terminate.
func (b *Batch) start() error {
	cmd := exec.CommandContext(b.ctx, b.argv[0], b.argv[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("output pipe: %w", err)
	}
	cmd.Stdout = w
	cmd.Stderr = w
	err = cmd.Start()
	w.Close()
	if err != nil {
		r.Close()
		return fmt.Errorf("start %s batch: %w", b.argv[0], err)
	}

	lines := make(chan []byte, 8)
	quit := make(chan struct{})
	gen := b.gen.Add(1)
	b.mu.Lock()
	if b.quit != nil {
		close(b.quit)
	}
	b.cmd = cmd
	b.stdin = stdin
	b.lines = lines
	b.quit = quit
	b.seq = 0
	b.alive.Store(true)
	b.mu.Unlock()

	go b.readLines(r, lines, quit, gen)
	return nil
}

// readLines feeds the merged stdout and stderr of one subprocess to
// lines until it exits or is replaced.
func (b *Batch) readLines(r io.ReadCloser, lines chan<- []byte, quit <-chan struct{}, gen int64) {
	defer r.Close()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		select {
		case lines <- append([]byte(nil), scanner.Bytes()...):
		case <-quit:
			return
		}
	}
	close(lines)
	if b.gen.Load() == gen {
		b.markDead()
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

	if _, err := fmt.Fprintf(b.stdin, "%s\n%s\n", command, sentinel); err != nil {
		b.markDead()
		return nil, fmt.Errorf("write command: %w", err)
	}
	b.seq += 2
	cmdNo, endNo := b.seq-1, b.seq

	var answer json.RawMessage
	failed := false
	timeout := time.NewTimer(queryTimeout)
	defer timeout.Stop()
	for {
		select {
		case line, ok := <-b.lines:
			if !ok {
				return nil, ErrBatchDead
			}
			if m := failedRe.FindSubmatch(line); m != nil {
				n, _ := strconv.Atoi(string(m[1]))
				switch n {
				case endNo:
					if failed {
						return nil, fmt.Errorf("%w: %s", ErrCommandFailed, command)
					}
					return answer, nil
				case cmdNo:
					failed = true
				}
				continue
			}
			if len(line) > 0 && (line[0] == '[' || line[0] == '{') {
				answer = json.RawMessage(line)
			}
			// Anything else is an error message, e.g. "Device "x" does
			// not exist.", and the failure line that follows says so.
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

// Refresh replaces the subprocess with a fresh one and validates it with
// the canary.  iproute2 caches name-to-index lookups for the life of the
// process, so once a name is reused by a new interface only a new
// process resolves it right.
func (b *Batch) Refresh() error {
	b.restartMu.Lock()
	defer b.restartMu.Unlock()

	b.mu.Lock()
	cmd, stdin := b.cmd, b.stdin
	b.mu.Unlock()

	if err := b.start(); err != nil {
		b.markDead()
		return err
	}
	reap(cmd, stdin)

	_, err := b.Query(b.canary)
	return err
}

// reap terminates a subprocess that is no longer current.
func reap(cmd *exec.Cmd, stdin io.Closer) {
	if stdin != nil {
		stdin.Close()
	}
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
		go cmd.Wait()
	}
}

// Alive tells whether the subprocess is running and answering.
func (b *Batch) Alive() bool {
	return b.alive.Load()
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

			if b.restart() {
				b.log.Info(b.argv[0] + " batch: restarted")
				delay = bo.Initial
			}
		}
	}
}

// restart replaces the dead subprocess, unless a Refresh got there
// first.  It reports whether a live subprocess is in place.
func (b *Batch) restart() bool {
	b.restartMu.Lock()
	defer b.restartMu.Unlock()

	if b.alive.Load() {
		return true
	}

	b.mu.Lock()
	cmd, stdin := b.cmd, b.stdin
	b.mu.Unlock()

	if err := b.start(); err != nil {
		b.log.Warn(b.argv[0]+" batch: restart failed", "err", err)
		return false
	}
	reap(cmd, stdin)

	if _, err := b.Query(b.canary); err != nil {
		b.log.Warn(b.argv[0]+" batch: canary query failed", "err", err)
		b.markDead()
		return false
	}
	return true
}
