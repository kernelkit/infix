package mcd

import (
	"net"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// Shape as mcd 3.1 prints `show interfaces json` (ipc.c show_igmp_iface):
// "vid" only on a bridge VLAN, "timeout" only for another device's
// querier.  br0 runs mcd per VLAN, br1 on its VLAN interface br1.20.
const showInterfacesJSON = `[
  {"interface": "br0", "family": "ipv4", "vid": 10, "state": "up", "querier": "10.0.0.1",
   "interval": 125, "router-timeout": 255, "mrd": false, "version": 3},
  {"interface": "br0", "family": "ipv4", "vid": 20, "state": "up", "querier": "10.0.0.2",
   "timeout": 240, "interval": 60, "router-timeout": 255, "mrd": false, "version": 2},
  {"interface": "br0", "family": "ipv6", "vid": 10, "state": "proxy", "querier": "::",
   "interval": 125, "router-timeout": 255, "mrd": false, "version": 2},
  {"interface": "br0", "family": "ipv4", "vid": 30, "state": "disabled", "querier": "0.0.0.0",
   "interval": 125, "router-timeout": 255, "mrd": false, "version": 3},
  {"interface": "br1.20", "family": "ipv4", "state": "up", "querier": "192.168.20.1",
   "interval": 125, "router-timeout": 255, "mrd": false, "version": 3},
  {"interface": "br2", "family": "ipv4", "state": "proxy", "querier": "0.0.0.0",
   "interval": 30, "router-timeout": 255, "mrd": false, "version": 3}
]`

func queriers(t *testing.T) []Querier {
	t.Helper()
	qs, err := Parse([]byte(showInterfacesJSON))
	if err != nil {
		t.Fatal(err)
	}
	return qs
}

func TestApply(t *testing.T) {
	tests := []struct {
		name   string
		bridge string
		vid    int
		upper  string
		want   map[string]any
	}{
		{
			name: "local IGMP querier and MLD proxy on a VLAN", bridge: "br0", vid: 10,
			want: map[string]any{
				"querier": "auto", "query-interval": 125,
				"active-querier": []any{
					map[string]any{"family": "ipv4", "address": "10.0.0.1", "local": true, "version": 3, "query-interval": 125},
					map[string]any{"family": "ipv6", "address": "::", "local": true, "version": 2, "query-interval": 125},
				},
			},
		},
		{
			name: "another device won the election", bridge: "br0", vid: 20,
			want: map[string]any{
				"querier": "auto", "query-interval": 60,
				"active-querier": []any{
					map[string]any{"family": "ipv4", "address": "10.0.0.2", "local": false, "version": 2, "query-interval": 60, "timeout": 240},
				},
			},
		},
		{
			name: "querier not active", bridge: "br0", vid: 30,
			want: map[string]any{"querier": "auto", "query-interval": 125},
		},
		{
			name: "mcd on the VLAN interface", bridge: "br1", vid: 20, upper: "br1.20",
			want: map[string]any{
				"querier": "auto", "query-interval": 125,
				"active-querier": []any{
					map[string]any{"family": "ipv4", "address": "192.168.20.1", "local": true, "version": 3, "query-interval": 125},
				},
			},
		},
		{
			name: "proxy querier on a bridge", bridge: "br2",
			want: map[string]any{
				"querier": "proxy", "query-interval": 30,
				"active-querier": []any{
					map[string]any{"family": "ipv4", "address": "0.0.0.0", "local": true, "version": 3, "query-interval": 30},
				},
			},
		},
		{
			name: "no querier", bridge: "br3",
			want: map[string]any{"querier": "off"},
		},
	}

	qs := queriers(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := map[string]any{}
			Apply(got, qs, tt.bridge, tt.vid, tt.upper)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %v\nwant %v", got, tt.want)
			}
		})
	}
}

func TestRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcd.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	cmd := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 768)
		n, _ := conn.Read(buf)
		cmd <- string(buf[:n])
		conn.Write([]byte(showInterfacesJSON))
	}()

	qs, err := Read(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got := <-cmd; got != "show interfaces json" {
		t.Errorf("sent %q", got)
	}
	if !reflect.DeepEqual(qs, queriers(t)) {
		t.Errorf("got %+v", qs)
	}

	if _, err := Read(filepath.Join(t.TempDir(), "none.sock"), time.Second); err == nil {
		t.Error("want an error without mcd")
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := Parse([]byte("no mcd")); err == nil {
		t.Error("want an error")
	}
}
