package ipc

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

// Server listens on an AF_UNIX SOCK_STREAM socket and serves
// YANG operational data from an in-memory Tree.
type Server struct {
	tree     *tree.Tree
	listener net.Listener
	ready    *atomic.Bool
	wg       sync.WaitGroup

	// timeout bounds a whole request/response exchange, the C
	// client in statd uses the same.
	timeout time.Duration

	// debounce is how long "watch" collects a burst of changes.
	debounce time.Duration

	mu    sync.Mutex
	conns map[net.Conn]struct{}
}

// NewServer creates a Server that serves data from the given Tree.
// While ready is false, all requests receive a 503 "starting" response.
func NewServer(t *tree.Tree, ready *atomic.Bool) *Server {
	return &Server{
		tree:     t,
		ready:    ready,
		timeout:  5 * time.Second,
		debounce: 100 * time.Millisecond,
		conns:    make(map[net.Conn]struct{}),
	}
}

// Listen creates and binds a Unix domain socket at path, root only.
// A stale socket file is removed before binding.
func (s *Server) Listen(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	old := syscall.Umask(0177)
	ln, err := net.Listen("unix", path)
	syscall.Umask(old)
	if err != nil {
		return err
	}
	s.listener = ln
	return nil
}

// Serve accepts connections until ctx is cancelled.  Each connection
// is handled in its own goroutine.
func (s *Server) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		s.listener.Close()
		s.mu.Lock()
		for conn := range s.conns {
			conn.Close()
		}
		s.mu.Unlock()
	}()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// Listener closed by context cancellation — normal shutdown.
			select {
			case <-ctx.Done():
				s.wg.Wait()
				return nil
			default:
				return err
			}
		}
		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(conn)
			s.mu.Lock()
			delete(s.conns, conn)
			s.mu.Unlock()
		}()
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ipc: panic serving request: %v\n%s", r, debug.Stack())
		}
	}()
	conn.SetDeadline(time.Now().Add(s.timeout))

	req, err := ReadRequest(conn)
	if err != nil {
		log.Printf("ipc: read request: %v", err)
		return
	}

	if !s.ready.Load() {
		WriteResponse(conn, &Response{
			Status:  "starting",
			Code:    503,
			Message: "yangerd is starting up",
		})
		return
	}

	switch req.Method {
	case "get":
		s.handleGet(conn, req)
	case "health":
		s.handleHealth(conn)
	case "watch":
		s.handleWatch(conn, req)
	default:
		WriteResponse(conn, &Response{
			Status:  "error",
			Code:    400,
			Message: "unknown method: " + req.Method,
		})
	}
}

func (s *Server) handleGet(conn net.Conn, req *Request) {
	path := req.Path
	if path == "" || path == "/" {
		s.handleDump(conn)
		return
	}

	key := path
	if key[0] == '/' {
		key = key[1:]
	}

	body, err := envelope(key, s.tree.Get(key))
	if err != nil {
		WriteResponse(conn, &Response{
			Status:  "error",
			Code:    500,
			Message: "marshal error: " + err.Error(),
		})
		return
	}

	WriteResponse(conn, &Response{
		Status: "ok",
		Data:   body,
	})
}

// envelope wraps the data of a key the way libyang wants it.
func envelope(key string, data json.RawMessage) (json.RawMessage, error) {
	if data == nil {
		// An absent subtree is a normal answer for operational data --
		// the feature is simply not active (e.g. NTP unconfigured).
		// Answer ok with an empty object rather than an error, so every
		// client gets "no data" without special-casing.  Deliberately
		// NOT {"<key>": {}}: that would make libyang instantiate the
		// container, which for presence containers is real data.
		return json.RawMessage(`{}`), nil
	}
	return json.Marshal(map[string]json.RawMessage{key: data})
}

// handleWatch keeps the connection and writes the stored data of each
// key that changes, starting with all of them, until the client hangs
// up.  Values a tree provider computes per request are left out.
func (s *Server) handleWatch(conn net.Conn, req *Request) {
	conn.SetDeadline(time.Time{})

	w := s.tree.Watch(req.Keys...)
	defer s.tree.Unwatch(w)

	gone := make(chan struct{})
	go func() {
		io.Copy(io.Discard, conn)
		close(gone)
	}()

	for {
		select {
		case <-gone:
			return
		case <-w.C:
		}
		select {
		case <-gone:
			return
		case <-time.After(s.debounce):
		}
		for _, key := range w.Take() {
			body, err := envelope(key, s.tree.GetCached(key))
			if err != nil {
				log.Printf("ipc: watch %s: %v", key, err)
				continue
			}
			conn.SetWriteDeadline(time.Now().Add(s.timeout))
			if WriteResponse(conn, &Response{Status: "ok", Key: key, Data: body}) != nil {
				return
			}
		}
	}
}

func (s *Server) handleDump(conn net.Conn) {
	keys := s.tree.Keys()
	blobs := s.tree.GetMulti(keys)

	all := make(map[string]json.RawMessage, len(keys))
	for i, k := range keys {
		if i < len(blobs) {
			all[k] = blobs[i]
		}
	}

	body, err := json.Marshal(all)
	if err != nil {
		WriteResponse(conn, &Response{
			Status:  "error",
			Code:    500,
			Message: "marshal error: " + err.Error(),
		})
		return
	}

	WriteResponse(conn, &Response{
		Status: "ok",
		Data:   body,
	})
}

func (s *Server) handleHealth(conn net.Conn) {
	keys := s.tree.Keys()
	models := make(map[string]json.RawMessage, len(keys))

	for _, k := range keys {
		info, ok := s.tree.Info(k)
		if !ok {
			continue
		}
		entry := struct {
			LastUpdated string `json:"last_updated"`
			SizeBytes   int    `json:"size_bytes"`
		}{
			LastUpdated: info.LastUpdated.UTC().Format(time.RFC3339),
			SizeBytes:   info.SizeBytes,
		}
		b, _ := json.Marshal(entry)
		models[k] = b
	}

	WriteResponse(conn, &Response{
		Status: "ok",
		Models: models,
	})
}
