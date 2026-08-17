package ipc

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Client connects to a yangerd Unix socket and issues IPC requests.
type Client struct {
	addr    string
	timeout time.Duration
}

// NewClient returns a Client that connects to the given socket path
// with per-request timeout.
func NewClient(socketPath string, timeout time.Duration) *Client {
	return &Client{
		addr:    socketPath,
		timeout: timeout,
	}
}

// Get queries a YANG subtree by path.  Path "/" returns all models.
func (c *Client) Get(path string) (*Response, error) {
	return c.call(&Request{Method: "get", Path: path})
}

// Health returns per-model freshness metadata.
func (c *Client) Health() (*Response, error) {
	return c.call(&Request{Method: "health"})
}

func (c *Client) call(req *Request) (*Response, error) {
	conn, err := net.DialTimeout("unix", c.addr, c.timeout)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", c.addr, err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(c.timeout))

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if err := WriteFrame(conn, payload); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}
	resp, err := ReadResponse(conn)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return resp, nil
}
