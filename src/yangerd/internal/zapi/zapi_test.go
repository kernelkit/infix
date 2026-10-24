package zapi

import (
	"bytes"
	"testing"
)

func TestEncodeDecodeHeader(t *testing.T) {
	raw := EncodeHeader(42, 0, CmdHello)
	hdr, err := DecodeHeader(raw)
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Length != 42 {
		t.Errorf("Length = %d, want 42", hdr.Length)
	}
	if hdr.Command != CmdHello {
		t.Errorf("Command = %d, want %d", hdr.Command, CmdHello)
	}
	if hdr.VrfID != 0 {
		t.Errorf("VrfID = %d, want 0", hdr.VrfID)
	}
}

func TestDecodeHeaderBadMarker(t *testing.T) {
	raw := EncodeHeader(10, 0, CmdHello)
	raw[2] = 0x00
	_, err := DecodeHeader(raw)
	if err == nil {
		t.Fatal("expected error for bad marker")
	}
}

func TestDecodeHeaderBadVersion(t *testing.T) {
	raw := EncodeHeader(10, 0, CmdHello)
	raw[3] = 5
	_, err := DecodeHeader(raw)
	if err == nil {
		t.Fatal("expected error for bad version")
	}
}

func TestBuildMessage(t *testing.T) {
	body := EncodeHello()
	msg := BuildMessage(CmdHello, DefaultVrf, body)
	if len(msg) != HeaderSize+len(body) {
		t.Errorf("message len = %d, want %d", len(msg), HeaderSize+len(body))
	}
	hdr, err := DecodeHeader(msg[:HeaderSize])
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Command != CmdHello {
		t.Errorf("Command = %d, want %d", hdr.Command, CmdHello)
	}
	if int(hdr.Length) != len(msg) {
		t.Errorf("Length = %d, want %d", hdr.Length, len(msg))
	}
}

func TestEncodeRedistributeAdd(t *testing.T) {
	body := EncodeRedistributeAdd(AFIIPv4, RouteStatic)
	if len(body) != 4 {
		t.Fatalf("body len = %d, want 4", len(body))
	}
	if body[0] != AFIIPv4 {
		t.Errorf("afi = %d, want %d", body[0], AFIIPv4)
	}
	if body[1] != uint8(RouteStatic) {
		t.Errorf("routeType = %d, want %d", body[1], RouteStatic)
	}
}

func TestReadMessage(t *testing.T) {
	body := []byte{0x01, 0x02, 0x03}
	msg := BuildMessage(CmdRouterIDUpdate, DefaultVrf, body)
	r := bytes.NewReader(msg)

	hdr, gotBody, err := ReadMessage(r)
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Command != CmdRouterIDUpdate {
		t.Errorf("Command = %d, want %d", hdr.Command, CmdRouterIDUpdate)
	}
	if !bytes.Equal(gotBody, body) {
		t.Errorf("body = %v, want %v", gotBody, body)
	}
}
