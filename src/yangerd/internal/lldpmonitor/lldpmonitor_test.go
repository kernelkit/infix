package lldpmonitor

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTransformLLDPEvent(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantPort     int
		wantByIfName map[string]int
		wantFirst    map[string]string
	}{
		{
			name: "lldp-added single neighbor",
			input: `{
				"lldp-added": {
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
				}
			}`,
			wantPort:     1,
			wantByIfName: map[string]int{"eth0": 1},
			wantFirst: map[string]string{
				"chassis-id-subtype": "mac-address",
				"port-id-subtype":    "interface-name",
				"chassis-id":         "aa:bb:cc:dd:ee:ff",
				"port-id":            "swp1",
			},
		},
		{
			name: "lldp-updated multiple neighbors same port",
			input: `{
				"lldp-updated": {
					"lldp": {
						"interface": [
							{
								"eth1": {
									"rid": "1",
									"age": "1 day, 02:30:15",
									"chassis": {"id": {"type": "ifname", "value": "leaf1"}},
									"port": {"id": {"type": "local", "value": "portA"}}
								}
							},
							{
								"eth1": {
									"rid": 2,
									"age": "10 days, 00:00:00",
									"chassis": {"id": {"type": "ip", "value": "192.0.2.1"}},
									"port": {"id": {"type": "mac", "value": "00:11:22:33:44:55"}}
								}
							}
						]
					}
				}
			}`,
			wantPort:     1,
			wantByIfName: map[string]int{"eth1": 2},
		},
		{
			name: "lldp-deleted event",
			input: `{
				"lldp-deleted": {
					"lldp": {
						"interface": [
							{
								"eth2": {
									"rid": 4,
									"age": "0 day, 00:00:01",
									"chassis": {"id": {"type": "local", "value": "chassis-local"}},
									"port": {"id": {"type": "ip", "value": "198.51.100.3"}}
								}
							}
						]
					}
				}
			}`,
			wantPort:     1,
			wantByIfName: map[string]int{"eth2": 1},
		},
		{
			name:         "empty object",
			input:        `{}`,
			wantPort:     0,
			wantByIfName: map[string]int{},
		},
		{
			name:         "malformed input",
			input:        `{not-json`,
			wantPort:     0,
			wantByIfName: map[string]int{},
		},
	}

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
	type outShape struct {
		LLDP struct {
			Port []port `json:"port"`
		} `json:"ieee802-dot1ab-lldp:lldp"`
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := transformLLDPEvent([]byte(tt.input))

			var decoded outShape
			if err := json.Unmarshal(got, &decoded); err != nil {
				t.Fatalf("unmarshal output: %v", err)
			}

			if len(decoded.LLDP.Port) != tt.wantPort {
				t.Fatalf("port count = %d, want %d", len(decoded.LLDP.Port), tt.wantPort)
			}

			byIf := make(map[string]int)
			for _, p := range decoded.LLDP.Port {
				if p.DestMAC != lldpMulticastMAC {
					t.Fatalf("dest-mac-address = %q, want %q", p.DestMAC, lldpMulticastMAC)
				}
				byIf[p.Name] = len(p.RemoteSystems)
			}

			if !reflect.DeepEqual(byIf, tt.wantByIfName) {
				t.Fatalf("neighbors by if mismatch\n got: %#v\nwant: %#v", byIf, tt.wantByIfName)
			}

			if len(tt.wantFirst) > 0 {
				first := decoded.LLDP.Port[0].RemoteSystems[0]
				gotFirst := map[string]string{
					"chassis-id-subtype": first.ChassisIDSubtype,
					"port-id-subtype":    first.PortIDSubtype,
					"chassis-id":         first.ChassisID,
					"port-id":            first.PortID,
				}
				if !reflect.DeepEqual(gotFirst, tt.wantFirst) {
					t.Fatalf("first remote mismatch\n got: %#v\nwant: %#v", gotFirst, tt.wantFirst)
				}
			}
		})
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
