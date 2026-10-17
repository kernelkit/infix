package unixgram

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestDialRoundTripAndCleanup(t *testing.T) {
	dir := t.TempDir()
	server := filepath.Join(dir, "srv")
	local := filepath.Join(dir, "client", "c1")

	srv, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: server, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	// A stale file from a killed process must not block the bind
	os.MkdirAll(filepath.Dir(local), 0755)
	os.WriteFile(local, nil, 0644)

	conn, err := Dial(local, server, 0666)
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(local); err != nil || fi.Mode().Perm() != 0666 {
		t.Fatalf("local socket mode = %v, %v", fi, err)
	}

	if _, err := conn.Write([]byte("PING")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	n, from, err := srv.ReadFromUnix(buf)
	if err != nil || string(buf[:n]) != "PING" {
		t.Fatalf("server got %q, %v", buf[:n], err)
	}
	srv.WriteToUnix([]byte("PONG"), from)
	if n, err = conn.Read(buf); err != nil || string(buf[:n]) != "PONG" {
		t.Fatalf("client got %q, %v", buf[:n], err)
	}

	conn.Close()
	if _, err := os.Stat(local); !os.IsNotExist(err) {
		t.Fatalf("local socket not removed on Close: %v", err)
	}
}

func TestDialFailureLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "c1")

	if _, err := Dial(local, filepath.Join(dir, "nobody"), 0); err == nil {
		t.Fatal("expected dial to a missing server to fail")
	}
	if _, err := os.Stat(local); !os.IsNotExist(err) {
		t.Fatalf("local socket left behind: %v", err)
	}
}

func TestCleanDirOnce(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "stale"), nil, 0644)

	CleanDir(dir)
	if _, err := os.Stat(filepath.Join(dir, "stale")); !os.IsNotExist(err) {
		t.Fatal("stale file survived the first clean")
	}

	os.WriteFile(filepath.Join(dir, "live"), nil, 0644)
	CleanDir(dir)
	if _, err := os.Stat(filepath.Join(dir, "live")); err != nil {
		t.Fatal("second clean must not remove files created since")
	}
}
