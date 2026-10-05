// Package inotify is a thin Linux inotify watcher with the fsnotify
// shape the monitors use: Add, Remove, Events and Errors.  Events carry
// the full path, a directory watch reports its entries as dir/name.
package inotify

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Op is a set of event types.
type Op uint32

const (
	Create Op = 1 << iota
	Write
	Remove
	Rename
	Chmod
)

// Has reports whether op contains every bit of want.
func (op Op) Has(want Op) bool { return op&want == want }

func (op Op) String() string {
	var parts []string
	for _, f := range []struct {
		op   Op
		name string
	}{{Create, "CREATE"}, {Write, "WRITE"}, {Remove, "REMOVE"}, {Rename, "RENAME"}, {Chmod, "CHMOD"}} {
		if op.Has(f.op) {
			parts = append(parts, f.name)
		}
	}
	if len(parts) == 0 {
		return "NONE"
	}
	return strings.Join(parts, "|")
}

// Event is one change on a watched path.
type Event struct {
	Name string
	Op   Op
}

// Has reports whether the event has every bit of op.
func (e Event) Has(op Op) bool { return e.Op.Has(op) }

// ErrEventOverflow is sent on Errors when the kernel dropped events.
var ErrEventOverflow = errors.New("inotify queue overflow")

// ErrNonExistentWatch is returned by Remove for a path not watched.
var ErrNonExistentWatch = errors.New("can't remove non-existent watch")

const mask = unix.IN_CREATE | unix.IN_MODIFY | unix.IN_DELETE | unix.IN_DELETE_SELF |
	unix.IN_MOVED_FROM | unix.IN_MOVED_TO | unix.IN_MOVE_SELF | unix.IN_ATTRIB

// Watcher delivers inotify events on Events until Close.
type Watcher struct {
	Events chan Event
	Errors chan error

	fd      int
	closeR  int
	closeW  int
	mu      sync.Mutex
	paths   map[int]string // wd -> path
	watches map[string]int // path -> wd
	closing chan struct{}  // closed by Close, before readLoop is told
	done    chan struct{}  // closed when readLoop has exited
	once    sync.Once
}

// NewWatcher creates an inotify instance and starts reading it.
func NewWatcher() (*Watcher, error) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, fmt.Errorf("inotify_init: %w", err)
	}

	var p [2]int
	if err := unix.Pipe2(p[:], unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("pipe: %w", err)
	}

	w := &Watcher{
		Events:  make(chan Event),
		Errors:  make(chan error),
		fd:      fd,
		closeR:  p[0],
		closeW:  p[1],
		paths:   make(map[int]string),
		watches: make(map[string]int),
		closing: make(chan struct{}),
		done:    make(chan struct{}),
	}
	go w.readLoop()
	return w, nil
}

// Add watches path.  Adding a path already watched is a no-op.
func (w *Watcher) Add(path string) error {
	path = filepath.Clean(path)
	w.mu.Lock()
	defer w.mu.Unlock()

	wd, err := unix.InotifyAddWatch(w.fd, path, mask)
	if err != nil {
		return &os.PathError{Op: "inotify_add_watch", Path: path, Err: err}
	}
	if old, ok := w.paths[wd]; ok && old != path {
		delete(w.watches, old)
	}
	w.paths[wd] = path
	w.watches[path] = wd
	return nil
}

// Remove stops watching path.
func (w *Watcher) Remove(path string) error {
	path = filepath.Clean(path)
	w.mu.Lock()
	defer w.mu.Unlock()

	wd, ok := w.watches[path]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNonExistentWatch, path)
	}
	delete(w.watches, path)
	delete(w.paths, wd)

	// The kernel already dropped the watch of a deleted path, so
	// EINVAL here is expected and not an error.
	if _, err := unix.InotifyRmWatch(w.fd, uint32(wd)); err != nil && err != unix.EINVAL {
		return &os.PathError{Op: "inotify_rm_watch", Path: path, Err: err}
	}
	return nil
}

// Close stops the watcher and closes Events and Errors.
func (w *Watcher) Close() error {
	w.once.Do(func() {
		close(w.closing)
		unix.Write(w.closeW, []byte{0})
		<-w.done
		unix.Close(w.closeW)
		unix.Close(w.closeR)
		unix.Close(w.fd)
	})
	return nil
}

func (w *Watcher) readLoop() {
	defer close(w.done)
	defer close(w.Errors)
	defer close(w.Events)

	buf := make([]byte, 64*1024)
	fds := []unix.PollFd{
		{Fd: int32(w.fd), Events: unix.POLLIN},
		{Fd: int32(w.closeR), Events: unix.POLLIN},
	}

	for {
		if _, err := unix.Poll(fds, -1); err != nil {
			if err == unix.EINTR {
				continue
			}
			w.sendError(fmt.Errorf("poll: %w", err))
			return
		}
		if fds[1].Revents != 0 {
			return
		}
		if fds[0].Revents == 0 {
			continue
		}

		for {
			n, err := unix.Read(w.fd, buf)
			if err == unix.EAGAIN || err == unix.EINTR {
				break
			}
			if err != nil {
				w.sendError(fmt.Errorf("read: %w", err))
				return
			}
			if !w.dispatch(buf[:n]) {
				return
			}
		}
	}
}

// dispatch decodes one read worth of events.  It returns false when the
// watcher was closed while an event was waiting to be received.
func (w *Watcher) dispatch(buf []byte) bool {
	for len(buf) >= unix.SizeofInotifyEvent {
		raw := (*unix.InotifyEvent)(unsafe.Pointer(&buf[0]))
		size := unix.SizeofInotifyEvent + int(raw.Len)
		if size > len(buf) {
			return true
		}
		name := strings.TrimRight(string(buf[unix.SizeofInotifyEvent:size]), "\x00")
		buf = buf[size:]

		if raw.Mask&unix.IN_Q_OVERFLOW != 0 {
			if !w.sendError(ErrEventOverflow) {
				return false
			}
			continue
		}

		w.mu.Lock()
		path, ok := w.paths[int(raw.Wd)]
		if raw.Mask&(unix.IN_IGNORED|unix.IN_DELETE_SELF|unix.IN_MOVE_SELF) != 0 && ok {
			delete(w.paths, int(raw.Wd))
			delete(w.watches, path)
		}
		w.mu.Unlock()
		if !ok || raw.Mask&unix.IN_IGNORED != 0 {
			continue
		}

		if name != "" {
			path = filepath.Join(path, name)
		}
		op := opFromMask(raw.Mask)
		if op == 0 {
			continue
		}

		select {
		case w.Events <- Event{Name: path, Op: op}:
		case <-w.closed():
			return false
		}
	}
	return true
}

func opFromMask(m uint32) Op {
	var op Op
	if m&(unix.IN_CREATE|unix.IN_MOVED_TO) != 0 {
		op |= Create
	}
	if m&unix.IN_MODIFY != 0 {
		op |= Write
	}
	if m&(unix.IN_DELETE|unix.IN_DELETE_SELF) != 0 {
		op |= Remove
	}
	if m&(unix.IN_MOVED_FROM|unix.IN_MOVE_SELF) != 0 {
		op |= Rename
	}
	if m&unix.IN_ATTRIB != 0 {
		op |= Chmod
	}
	return op
}

func (w *Watcher) sendError(err error) bool {
	select {
	case w.Errors <- err:
		return true
	case <-w.closed():
		return false
	}
}

// closed yields a channel that is readable once Close has been called.
func (w *Watcher) closed() <-chan struct{} {
	return w.closing
}
