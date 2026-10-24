package ctrnet

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const podmanPS = `[
  {"Names": ["web"], "Networks": ["veth0b", "e3"], "State": "running"},
  {"Names": ["db"], "Networks": ["veth0b"], "State": "running"},
  {"Names": ["old"], "Networks": ["veth0b", "e9"], "State": "exited"},
  {"Networks": ["e3"]}
]`

const netnsList = `[{"name": "netns-1111"}, {"name": "netns-2222"}]`

// e3 and veth0b were renamed eth0/eth1 by podman; eth2 has an alias
// nobody asked for and lo has none.
const ns1Links = `[
  {"ifindex": 1, "ifname": "lo", "flags": ["LOOPBACK", "UP"], "link_type": "loopback", "operstate": "UNKNOWN"},
  {"ifindex": 7, "ifname": "eth0", "ifalias": "e3", "flags": ["BROADCAST", "UP"], "link_type": "ether",
   "address": "02:00:00:00:00:03", "operstate": "UP",
   "stats64": {"rx": {"bytes": 18446744073709551615}, "tx": {"bytes": 42}}},
  {"ifindex": 8, "ifname": "eth1", "ifalias": "veth0b", "flags": ["BROADCAST", "UP"], "link_type": "ether",
   "address": "02:00:00:00:00:0b", "operstate": "UP",
   "linkinfo": {"info_kind": "veth"}, "link": "if9"},
  {"ifindex": 9, "ifname": "eth2", "ifalias": "unrelated", "flags": ["UP"], "link_type": "ether", "operstate": "UP"}
]`

const ns1Addrs = `[
  {"ifindex": 7, "ifname": "eth0", "mtu": 1500,
   "addr_info": [{"family": "inet", "local": "10.0.0.2", "prefixlen": 24, "protocol": "static"}]},
  {"ifindex": 8, "ifname": "eth1", "mtu": 1500,
   "addr_info": [{"family": "inet", "local": "172.17.0.2", "prefixlen": 16, "protocol": "dhcp"}]}
]`

type fakeRunner struct {
	out   map[string]string
	errs  map[string]error
	calls []string
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	cmd := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, cmd)
	if err, ok := f.errs[cmd]; ok {
		return nil, err
	}
	out, ok := f.out[cmd]
	if !ok {
		return nil, errors.New("unexpected command: " + cmd)
	}
	return []byte(out), nil
}

func newRunner() *fakeRunner {
	return &fakeRunner{
		out: map[string]string{
			"podman ps -a --format json":          podmanPS,
			"ip -j netns list":                    netnsList,
			"ip -n netns-1111 -s -d -j link show": ns1Links,
			"ip -n netns-1111 -j addr show":       ns1Addrs,
			"ip -n netns-2222 -s -d -j link show": `[{"ifindex": 1, "ifname": "lo", "flags": ["UP"], "link_type": "loopback", "operstate": "UNKNOWN"}]`,
			"ip -n netns-2222 -j addr show":       `[]`,
		},
		errs: map[string]error{},
	}
}

func decode(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return m
}

func TestCollect(t *testing.T) {
	r := newRunner()
	got, namespaces, err := Collect(context.Background(), r.run)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if !reflect.DeepEqual(namespaces, []string{"netns-1111"}) {
		t.Fatalf("namespaces = %v, want only netns-1111", namespaces)
	}

	if len(got) != 2 {
		t.Fatalf("got %d interfaces, want 2: %v", len(got), got)
	}

	e3 := decode(t, got["e3"])
	if e3["name"] != "e3" || e3["description"] != "eth0" {
		t.Fatalf("e3 name/description = %v/%v", e3["name"], e3["description"])
	}
	if e3["type"] != "infix-if-type:ethernet" || e3["if-index"] != float64(7) {
		t.Fatalf("e3 type/if-index = %v/%v", e3["type"], e3["if-index"])
	}
	if e3["phys-address"] != "02:00:00:00:00:03" || e3["oper-status"] != "up" || e3["admin-status"] != "up" {
		t.Fatalf("e3 common leaves: %#v", e3)
	}
	stats := e3["statistics"].(map[string]any)
	if stats["in-octets"] != "18446744073709551615" || stats["out-octets"] != "42" {
		t.Fatalf("e3 statistics = %#v", stats)
	}
	ipv4 := e3["ietf-ip:ipv4"].(map[string]any)
	addr := ipv4["address"].([]any)[0].(map[string]any)
	if addr["ip"] != "10.0.0.2" || addr["prefix-length"] != float64(24) {
		t.Fatalf("e3 address = %#v", addr)
	}
	wantNet := map[string]any{"containers": []any{"web", "Unknown"}}
	if !reflect.DeepEqual(e3["infix-interfaces:container-network"], wantNet) {
		t.Fatalf("e3 container-network = %#v, want %#v", e3["infix-interfaces:container-network"], wantNet)
	}

	veth := decode(t, got["veth0b"])
	if veth["name"] != "veth0b" || veth["description"] != "eth1" {
		t.Fatalf("veth0b name/description = %v/%v", veth["name"], veth["description"])
	}
	if veth["type"] != "infix-if-type:veth" {
		t.Fatalf("veth0b type = %v", veth["type"])
	}
	if _, ok := veth["infix-interfaces:veth"]; ok {
		t.Fatalf("veth augment must not leak into a container interface: %#v", veth)
	}
	wantNet = map[string]any{"containers": []any{"web", "db", "old"}}
	if !reflect.DeepEqual(veth["infix-interfaces:container-network"], wantNet) {
		t.Fatalf("veth0b container-network = %#v, want %#v", veth["infix-interfaces:container-network"], wantNet)
	}

	for _, key := range []string{"higher-layer-if", "lower-layer-if"} {
		if _, ok := veth[key]; ok {
			t.Fatalf("%s must not be set on a container interface", key)
		}
	}

	// addr show runs once per netns with a match, never for a netns
	// without one.
	for _, c := range r.calls {
		if c == "ip -n netns-2222 -j addr show" {
			t.Fatalf("addr show run for a netns without matching links: %v", r.calls)
		}
	}
	count := 0
	for _, c := range r.calls {
		if c == "ip -n netns-1111 -j addr show" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("addr show for netns-1111 run %d times, want 1", count)
	}
}

func TestCollectNoContainers(t *testing.T) {
	r := newRunner()
	r.out["podman ps -a --format json"] = `[]`

	got, _, err := Collect(context.Background(), r.run)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
	if len(r.calls) != 1 {
		t.Fatalf("ip must not run without container networks: %v", r.calls)
	}
}

func TestCollectPodmanMissing(t *testing.T) {
	r := newRunner()
	r.errs["podman ps -a --format json"] = errors.New("executable file not found")

	if _, _, err := Collect(context.Background(), r.run); err == nil {
		t.Fatal("expected an error when podman cannot run")
	}
}

func TestCollectNetnsVanished(t *testing.T) {
	r := newRunner()
	r.errs["ip -n netns-1111 -s -d -j link show"] = errors.New("Cannot open network namespace")

	got, _, err := Collect(context.Background(), r.run)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want nothing from a vanished netns", got)
	}
}

func TestCollectEmptyNetnsList(t *testing.T) {
	r := newRunner()
	r.out["ip -j netns list"] = ``

	got, _, err := Collect(context.Background(), r.run)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}
