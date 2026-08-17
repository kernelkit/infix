// Package ipc implements the yangerd IPC protocol: a versioned,
// length-prefixed JSON framing over AF_UNIX SOCK_STREAM.
//
// Wire format:
//
//	+--------+--------+--------+--------+--------+--- ... ---+
//	| ver(1) | length (uint32 big-endian, bytes)  | JSON body |
//	+--------+--------+--------+--------+--------+--- ... ---+
package ipc

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const (
	// Version is the current protocol version.
	Version byte = 1

	// MaxPayload is the maximum JSON body size (4 MiB).
	MaxPayload = 4 << 20

	headerSize = 5 // 1 byte version + 4 bytes length
)

// Request is the IPC request from a client.
type Request struct {
	Method string            `json:"method"`
	Path   string            `json:"path,omitempty"`
	Filter map[string]string `json:"filter,omitempty"`
}

// Response is the IPC response to a client.
type Response struct {
	Status  string `json:"status"`
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`

	// Used by "get" responses.
	Data json.RawMessage `json:"data,omitempty"`

	// Used by "health" responses.
	Subsystems map[string]json.RawMessage `json:"subsystems,omitempty"`
	Models     map[string]json.RawMessage `json:"models,omitempty"`
}

// WriteFrame writes a versioned, length-prefixed frame to w.
func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) > MaxPayload {
		return fmt.Errorf("payload size %d exceeds maximum %d", len(payload), MaxPayload)
	}
	hdr := [headerSize]byte{Version}
	binary.BigEndian.PutUint32(hdr[1:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// ReadFrame reads a versioned, length-prefixed frame from r.
func ReadFrame(r io.Reader) ([]byte, error) {
	var hdr [headerSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	if hdr[0] != Version {
		return nil, fmt.Errorf("protocol version mismatch: got %d, want %d", hdr[0], Version)
	}
	length := binary.BigEndian.Uint32(hdr[1:])
	if length > MaxPayload {
		return nil, fmt.Errorf("payload size %d exceeds maximum %d", length, MaxPayload)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// WriteResponse marshals a Response and writes it as a framed message.
func WriteResponse(w io.Writer, resp *Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return WriteFrame(w, data)
}

// ReadRequest reads and unmarshals a framed Request.
func ReadRequest(r io.Reader) (*Request, error) {
	data, err := ReadFrame(r)
	if err != nil {
		return nil, err
	}
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request JSON: %w", err)
	}
	return &req, nil
}

// ReadResponse reads and unmarshals a framed Response.
func ReadResponse(r io.Reader) (*Response, error) {
	data, err := ReadFrame(r)
	if err != nil {
		return nil, err
	}
	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("invalid response JSON: %w", err)
	}
	return &resp, nil
}
