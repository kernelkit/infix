package lldpmonitor

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

type remote struct {
	TimeMark         int    `json:"time-mark"`
	RemoteIndex      int    `json:"remote-index"`
	ChassisIDSubtype string `json:"chassis-id-subtype"`
	ChassisID        string `json:"chassis-id"`
	PortIDSubtype    string `json:"port-id-subtype"`
	PortID           string `json:"port-id"`
}
type port struct {
	Name          string   `json:"name"`
	DestMAC       string   `json:"dest-mac-address"`
	RemoteSystems []remote `json:"remote-systems-data"`
}

// outShape is the stored (unwrapped) subtree: the IPC layer adds the
// module envelope when serving the key.
type outShape struct {
	Port []port `json:"port"`
}

// json0 format: "lldp" is an array, interface entries carry a "name"
// field, rid/age are string attributes, chassis/port/id are arrays.
const showNeighborsJSON0 = `{
  "lldp": [{
    "interface": [
      {
        "name": "eth0",
        "via": "LLDP",
        "rid": "7",
        "age": "0 day, 00:05:30",
        "chassis": [{
          "id": [{"type": "mac", "value": "aa:bb:cc:dd:ee:ff"}],
          "name": [{"value": "switch1"}]
        }],
        "port": [{
          "id": [{"type": "ifname", "value": "swp1"}]
        }]
      },
      {
        "name": "eth1",
        "via": "LLDP",
        "rid": "9",
        "age": "1 day, 02:30:15",
        "chassis": [{
          "id": [{"type": "local", "value": "Chassis ID 007"}]
        }],
        "port": [{
          "id": [{"type": "mac", "value": "02:01:02:03:04:05"}]
        }]
      }
    ]
  }]
}`

// Older keyed json format: "lldp" is an object, interfaces are keyed by
// name, chassis/port are objects.
const showNeighborsJSON = `{
  "lldp": {
    "interface": [
      {
        "eth0": {
          "rid": 7,
          "age": "0 day, 00:05:30",
          "chassis": {"id": {"type": "mac", "value": "aa:bb:cc:dd:ee:ff"}},
          "port": {"id": {"type": "ifname", "value": "swp1"}}
        }
      }
    ]
  }
}`

func decode(t *testing.T, raw json.RawMessage) outShape {
	t.Helper()
	var out outShape
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	return out
}

func TestTransformNeighborsJSON0(t *testing.T) {
	out := decode(t, transformNeighbors([]byte(showNeighborsJSON0)))

	if len(out.Port) != 2 {
		t.Fatalf("port count = %d, want 2", len(out.Port))
	}

	byIf := make(map[string]port)
	for _, p := range out.Port {
		if p.DestMAC != lldpMulticastMAC {
			t.Fatalf("dest-mac-address = %q, want %q", p.DestMAC, lldpMulticastMAC)
		}
		byIf[p.Name] = p
	}

	eth0, ok := byIf["eth0"]
	if !ok || len(eth0.RemoteSystems) != 1 {
		t.Fatalf("eth0 missing or wrong neighbor count: %#v", byIf)
	}
	want := remote{
		TimeMark:         330,
		RemoteIndex:      7,
		ChassisIDSubtype: "mac-address",
		ChassisID:        "aa:bb:cc:dd:ee:ff",
		PortIDSubtype:    "interface-name",
		PortID:           "swp1",
	}
	if !reflect.DeepEqual(eth0.RemoteSystems[0], want) {
		t.Fatalf("eth0 remote\n got: %#v\nwant: %#v", eth0.RemoteSystems[0], want)
	}

	eth1 := byIf["eth1"]
	if len(eth1.RemoteSystems) != 1 {
		t.Fatalf("eth1 neighbor count = %d", len(eth1.RemoteSystems))
	}
	r := eth1.RemoteSystems[0]
	if r.ChassisIDSubtype != "local" || r.ChassisID != "Chassis ID 007" {
		t.Errorf("eth1 chassis = %s/%s", r.ChassisIDSubtype, r.ChassisID)
	}
	if r.PortIDSubtype != "mac-address" || r.PortID != "02:01:02:03:04:05" {
		t.Errorf("eth1 port = %s/%s", r.PortIDSubtype, r.PortID)
	}
	if r.RemoteIndex != 9 || r.TimeMark != 95415 {
		t.Errorf("eth1 rid/age = %d/%d", r.RemoteIndex, r.TimeMark)
	}
}

func TestTransformNeighborsKeyedJSON(t *testing.T) {
	out := decode(t, transformNeighbors([]byte(showNeighborsJSON)))

	if len(out.Port) != 1 {
		t.Fatalf("port count = %d, want 1", len(out.Port))
	}
	p := out.Port[0]
	if p.Name != "eth0" || len(p.RemoteSystems) != 1 {
		t.Fatalf("unexpected port: %#v", p)
	}
	r := p.RemoteSystems[0]
	if r.ChassisID != "aa:bb:cc:dd:ee:ff" || r.PortID != "swp1" || r.RemoteIndex != 7 {
		t.Fatalf("unexpected remote: %#v", r)
	}
}

func TestTransformNeighborsEmpty(t *testing.T) {
	for name, in := range map[string]string{
		"empty table json0": `{"lldp": [{}]}`,
		"empty object":      `{}`,
		"malformed":         `{not-json`,
	} {
		raw := transformNeighbors([]byte(in))
		if string(raw) != "{}" {
			t.Errorf("%s: got %s, want {}", name, raw)
		}
	}
}

// A neighbor that disappears between reads must vanish from the tree:
// every update is a full-table replace.
func TestUpdateTreeClearsRemovedNeighbors(t *testing.T) {
	tr := tree.New()
	m := New(tr, nil)

	m.query = func(context.Context) ([]byte, error) {
		return []byte(showNeighborsJSON0), nil
	}
	m.updateTree(context.Background())
	if out := decode(t, tr.Get(treeKey)); len(out.Port) != 2 {
		t.Fatalf("expected 2 ports after first read, got %d", len(out.Port))
	}

	m.query = func(context.Context) ([]byte, error) {
		return []byte(`{"lldp": [{}]}`), nil
	}
	m.updateTree(context.Background())
	if out := decode(t, tr.Get(treeKey)); len(out.Port) != 0 {
		t.Fatalf("stale neighbors not cleared: %d ports remain", len(out.Port))
	}
}

// A failing query must leave the previous data untouched.
func TestUpdateTreeQueryErrorKeepsData(t *testing.T) {
	tr := tree.New()
	m := New(tr, nil)

	m.query = func(context.Context) ([]byte, error) {
		return []byte(showNeighborsJSON0), nil
	}
	m.updateTree(context.Background())
	before := string(tr.Get(treeKey))

	m.query = func(context.Context) ([]byte, error) {
		return nil, errors.New("lldpcli gone")
	}
	m.updateTree(context.Background())

	if after := string(tr.Get(treeKey)); after != before {
		t.Fatal("query error overwrote existing lldp data")
	}
}

// Watch events are triggers only: added/updated/deleted all request a
// refresh, unknown events do not.
func TestProcessEventTriggers(t *testing.T) {
	m := New(tree.New(), nil)

	drain := func() {
		select {
		case <-m.refresh:
		default:
		}
	}

	for _, ev := range []string{"lldp-added", "lldp-updated", "lldp-deleted"} {
		drain()
		m.processEvent([]byte(`{"` + ev + `": {"lldp": {}}}`))
		select {
		case <-m.refresh:
		default:
			t.Errorf("%s did not trigger refresh", ev)
		}
	}

	drain()
	m.processEvent([]byte(`{"lldp-unknown": {}}`))
	select {
	case <-m.refresh:
		t.Error("unknown event triggered refresh")
	default:
	}
}

func TestParseAge(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{name: "zero day", in: "0 day, 00:05:30", want: 330},
		{name: "one day", in: "1 day, 02:30:15", want: 95415},
		{name: "ten days plural", in: "10 days, 00:00:00", want: 864000},
		{name: "empty", in: "", want: 0},
		{name: "invalid", in: "n/a", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseAge(tt.in); got != tt.want {
				t.Fatalf("parseAge(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestSubtypeMappings(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "ifalias", in: "ifalias", want: "interface-alias"},
		{name: "mac", in: "mac", want: "mac-address"},
		{name: "ip", in: "ip", want: "network-address"},
		{name: "ifname", in: "ifname", want: "interface-name"},
		{name: "local", in: "local", want: "local"},
		{name: "unknown", in: "foo", want: "unknown"},
	}

	for _, tt := range tests {
		t.Run("chassis_"+tt.name, func(t *testing.T) {
			if got := chassisIDSubtype(tt.in); got != tt.want {
				t.Fatalf("chassisIDSubtype(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
		t.Run("port_"+tt.name, func(t *testing.T) {
			if got := portIDSubtype(tt.in); got != tt.want {
				t.Fatalf("portIDSubtype(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
