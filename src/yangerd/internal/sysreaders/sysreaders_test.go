package sysreaders

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestReadHostname(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostname")
	write(t, path, "infix-00-00-00\n")

	raw, err := ReadHostname(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"hostname":"infix-00-00-00"}` {
		t.Fatalf("got %s", raw)
	}
}

// Static resolvers come from resolv.conf.head, DHCP ones from the
// per-interface resolvconf files, tagged with the interface they were
// learned on.  A server listed twice is reported once, it is the key.
func TestReadDNSResolver(t *testing.T) {
	dir := t.TempDir()
	head := filepath.Join(dir, "resolv.conf.head")
	ifaces := filepath.Join(dir, "interfaces")
	write(t, head, "nameserver 1.1.1.1\nsearch example.com\noptions timeout:2 attempts:3\n")
	write(t, filepath.Join(ifaces, "e1.conf"),
		"search lan # e1\nnameserver 192.168.1.1 # e1\nnameserver 1.1.1.1 # e1\n")
	write(t, filepath.Join(ifaces, "e2-ipv6.conf"), "nameserver fe80::1\n")

	raw, err := readDNSResolver(head, ifaces)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		DNS struct {
			Server  []map[string]string `json:"server"`
			Search  []string            `json:"search"`
			Options map[string]int      `json:"options"`
		} `json:"infix-system:dns-resolver"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}

	wantServers := []map[string]string{
		{"address": "1.1.1.1", "origin": "static"},
		{"address": "192.168.1.1", "origin": "dhcp", "interface": "e1"},
		{"address": "fe80::1", "origin": "dhcp", "interface": "e2"},
	}
	if !reflect.DeepEqual(out.DNS.Server, wantServers) {
		t.Errorf("servers = %v, want %v", out.DNS.Server, wantServers)
	}
	if !reflect.DeepEqual(out.DNS.Search, []string{"example.com", "lan"}) {
		t.Errorf("search = %v", out.DNS.Search)
	}
	if out.DNS.Options["timeout"] != 2 || out.DNS.Options["attempts"] != 3 {
		t.Errorf("options = %v", out.DNS.Options)
	}
}

func TestReadDNSResolverNothingConfigured(t *testing.T) {
	dir := t.TempDir()
	raw, err := readDNSResolver(filepath.Join(dir, "none"), filepath.Join(dir, "none.d"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"infix-system:dns-resolver":{"server":[]}}` {
		t.Fatalf("got %s", raw)
	}
}
