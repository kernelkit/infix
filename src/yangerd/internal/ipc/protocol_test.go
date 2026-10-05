package ipc

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	payload := []byte(`{"method":"get","path":"/test"}`)
	var buf bytes.Buffer

	if err := WriteFrame(&buf, payload); err != nil {
		t.Fatal(err)
	}

	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("mismatch: %s vs %s", got, payload)
	}
}

func TestFrameVersionMismatch(t *testing.T) {
	var buf bytes.Buffer
	buf.Write([]byte{99, 0, 0, 0, 2, '{', '}'})

	_, err := ReadFrame(&buf)
	if err == nil {
		t.Fatal("expected version mismatch error")
	}
}

func TestFrameOversized(t *testing.T) {
	var buf bytes.Buffer
	huge := make([]byte, MaxPayload+1)
	if err := WriteFrame(&buf, huge); err == nil {
		t.Fatal("expected oversized payload error")
	}
}

func TestRequestResponseRoundTrip(t *testing.T) {
	var buf bytes.Buffer

	req := &Request{Method: "get", Path: "/ietf-system:system-state"}
	data, _ := json.Marshal(req)
	WriteFrame(&buf, data)

	got, err := ReadRequest(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != "get" || got.Path != "/ietf-system:system-state" {
		t.Fatalf("unexpected request: %+v", got)
	}
}

func TestResponseRoundTrip(t *testing.T) {
	var buf bytes.Buffer

	resp := &Response{
		Status: "ok",
		Data:   json.RawMessage(`{"hostname":"r1"}`),
	}
	WriteResponse(&buf, resp)

	got, err := ReadResponse(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "ok" || string(got.Data) != `{"hostname":"r1"}` {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestEmptyFrame(t *testing.T) {
	var buf bytes.Buffer
	WriteFrame(&buf, []byte{})

	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %d bytes", len(got))
	}
}
