package ipc

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
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
}

// NewServer creates a Server that serves data from the given Tree.
// While ready is false, all requests receive a 503 "starting" response.
func NewServer(t *tree.Tree, ready *atomic.Bool) *Server {
	return &Server{
		tree:  t,
		ready: ready,
	}
}

// Listen creates and binds a Unix domain socket at path.
// A stale socket file is removed before binding.
func (s *Server) Listen(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	if err := os.Chmod(path, 0660); err != nil {
		ln.Close()
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
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(conn)
		}()
	}
}

// Addr returns the listener address, or empty string if not listening.
func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

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

	data := s.tree.Get(key)
	if data == nil {
		WriteResponse(conn, &Response{
			Status:  "error",
			Code:    404,
			Message: "path not found: " + path,
		})
		return
	}

	envelope := map[string]json.RawMessage{key: data}
	body, err := json.Marshal(envelope)
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
