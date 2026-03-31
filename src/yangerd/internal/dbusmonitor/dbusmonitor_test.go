package dbusmonitor

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestParseDnsmasqLeases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []map[string]any
	}{
		{
			name:  "normal lease line",
			input: "1711900000 aa:bb:cc:dd:ee:ff 192.168.1.100 myhost 01:aa:bb:cc:dd:ee:ff",
			want: []map[string]any{{
				"expires":      time.Unix(1711900000, 0).UTC().Format(time.RFC3339),
				"address":      "192.168.1.100",
				"phys-address": "aa:bb:cc:dd:ee:ff",
				"hostname":     "myhost",
				"client-id":    "01:aa:bb:cc:dd:ee:ff",
			}},
		},
		{
			name:  "wildcard hostname and client id",
			input: "1711900000 aa:bb:cc:dd:ee:ff 192.168.1.100 * *",
			want: []map[string]any{{
				"expires":      time.Unix(1711900000, 0).UTC().Format(time.RFC3339),
				"address":      "192.168.1.100",
				"phys-address": "aa:bb:cc:dd:ee:ff",
				"hostname":     "",
				"client-id":    "",
			}},
		},
		{
			name:  "never expiring lease",
			input: "0 aa:bb:cc:dd:ee:ff 192.168.1.100 host *",
			want: []map[string]any{{
				"expires":      "never",
				"address":      "192.168.1.100",
				"phys-address": "aa:bb:cc:dd:ee:ff",
				"hostname":     "host",
				"client-id":    "",
			}},
		},
		{
			name: "multiple leases with malformed lines skipped",
			input: "1711900000 aa:bb:cc:dd:ee:ff 192.168.1.100 myhost 01:aa:bb:cc:dd:ee:ff\n" +
				"bad line with too few fields\n" +
				"1711900100 11:22:33:44:55:66 192.168.1.101 host2 *\n",
			want: []map[string]any{
				{
					"expires":      time.Unix(1711900000, 0).UTC().Format(time.RFC3339),
					"address":      "192.168.1.100",
					"phys-address": "aa:bb:cc:dd:ee:ff",
					"hostname":     "myhost",
					"client-id":    "01:aa:bb:cc:dd:ee:ff",
				},
				{
					"expires":      time.Unix(1711900100, 0).UTC().Format(time.RFC3339),
					"address":      "192.168.1.101",
					"phys-address": "11:22:33:44:55:66",
					"hostname":     "host2",
					"client-id":    "",
				},
			},
		},
		{
			name:  "empty input",
			input: "",
			want:  []map[string]any{},
		},
		{
			name:  "invalid timestamp skipped",
			input: "abc aa:bb:cc:dd:ee:ff 192.168.1.100 host *",
			want:  []map[string]any{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseDnsmasqLeases(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseDnsmasqLeases() mismatch\nwant: %#v\n got: %#v", tc.want, got)
			}
		})
	}
}

func TestBuildDHCPTree(t *testing.T) {
	tests := []struct {
		name   string
		leases []map[string]any
		stats  map[string]any
		check  func(t *testing.T, root map[string]any)
	}{
		{
			name: "with leases and stats",
			leases: []map[string]any{{
				"expires":      "never",
				"address":      "192.168.1.100",
				"phys-address": "aa:bb:cc:dd:ee:ff",
				"hostname":     "host",
				"client-id":    "",
			}},
			stats: map[string]any{"out-offers": 3, "in-requests": 4},
			check: func(t *testing.T, root map[string]any) {
				t.Helper()
				stats, ok := root["statistics"].(map[string]any)
				if !ok {
					t.Fatalf("missing statistics map")
				}
				if stats["out-offers"] != float64(3) || stats["in-requests"] != float64(4) {
					t.Fatalf("unexpected statistics: %#v", stats)
				}

				leasesNode, ok := root["leases"].(map[string]any)
				if !ok {
					t.Fatalf("missing leases map")
				}
				leaseList, ok := leasesNode["lease"].([]any)
				if !ok || len(leaseList) != 1 {
					t.Fatalf("unexpected lease list: %#v", leasesNode["lease"])
				}
				lease, ok := leaseList[0].(map[string]any)
				if !ok || lease["address"] != "192.168.1.100" {
					t.Fatalf("unexpected lease entry: %#v", leaseList[0])
				}
			},
		},
		{
			name:   "with empty leases",
			leases: []map[string]any{},
			stats:  map[string]any{"out-offers": 0},
			check: func(t *testing.T, root map[string]any) {
				t.Helper()
				leasesNode := root["leases"].(map[string]any)
				leaseList, ok := leasesNode["lease"].([]any)
				if !ok || len(leaseList) != 0 {
					t.Fatalf("expected empty lease list, got %#v", leasesNode["lease"])
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := buildDHCPTree(tc.leases, tc.stats)
			var root map[string]any
			if err := json.Unmarshal(raw, &root); err != nil {
				t.Fatalf("unmarshal buildDHCPTree output: %v", err)
			}
			tc.check(t, root)
		})
	}
}

func TestBuildFirewallTree(t *testing.T) {
	tests := []struct {
		name       string
		defaultZ   string
		logDenied  string
		lockdown   bool
		zones      []map[string]any
		policies   []map[string]any
		services   []map[string]any
		expectKeys map[string]bool
	}{
		{
			name:      "with zones policies and services",
			defaultZ:  "public",
			logDenied: "all",
			lockdown:  true,
			zones:     []map[string]any{{"name": "public"}},
			policies:  []map[string]any{{"name": "default-drop"}},
			services:  []map[string]any{{"name": "ssh"}},
			expectKeys: map[string]bool{
				"zone":    true,
				"policy":  true,
				"service": true,
			},
		},
		{
			name:      "omits empty zone policy service keys",
			defaultZ:  "trusted",
			logDenied: "off",
			lockdown:  false,
			expectKeys: map[string]bool{
				"zone":    false,
				"policy":  false,
				"service": false,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := buildFirewallTree(tc.defaultZ, tc.logDenied, tc.lockdown, tc.zones, tc.policies, tc.services)
			var root map[string]any
			if err := json.Unmarshal(raw, &root); err != nil {
				t.Fatalf("unmarshal buildFirewallTree output: %v", err)
			}
			if root["default"] != tc.defaultZ || root["logging"] != tc.logDenied || root["lockdown"] != tc.lockdown {
				t.Fatalf("default/logging/lockdown mismatch: %#v", root)
			}
			for k, shouldExist := range tc.expectKeys {
				_, exists := root[k]
				if exists != shouldExist {
					t.Fatalf("key %q exists=%v, want %v", k, exists, shouldExist)
				}
			}
		})
	}
}

func TestParseServicePorts(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]any
		want     []map[string]any
	}{
		{
			name:     "single port",
			settings: map[string]any{"ports": []any{[]any{"80", "tcp"}}},
			want:     []map[string]any{{"proto": "tcp", "lower": 80}},
		},
		{
			name:     "port range",
			settings: map[string]any{"ports": []any{[]any{"8080-8090", "tcp"}}},
			want:     []map[string]any{{"proto": "tcp", "lower": 8080, "upper": 8090}},
		},
		{
			name: "multiple ports",
			settings: map[string]any{"ports": []any{
				[]any{"80", "tcp"},
				[]any{"53", "udp"},
			}},
			want: []map[string]any{
				{"proto": "tcp", "lower": 80},
				{"proto": "udp", "lower": 53},
			},
		},
		{
			name:     "missing ports",
			settings: map[string]any{},
			want:     []map[string]any{},
		},
		{
			name:     "empty ports",
			settings: map[string]any{"ports": []any{}},
			want:     []map[string]any{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseServicePorts(tc.settings)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseServicePorts mismatch\nwant: %#v\n got: %#v", tc.want, got)
			}
		})
	}
}

func TestParsePolicyCustomFilters(t *testing.T) {
	tests := []struct {
		name  string
		rules []string
		want  []map[string]any
	}{
		{
			name:  "rich rule icmp type accept",
			rules: []string{`rule priority="0" family="ipv4" icmp-type name="echo-request" accept`},
			want: []map[string]any{{
				"name":     "icmp-echo-request",
				"priority": 0,
				"family":   "ipv4",
				"action":   "accept",
				"icmp":     map[string]any{"type": "echo-request"},
			}},
		},
		{
			name:  "rich rule icmp block reject",
			rules: []string{`rule family="ipv6" icmp-block name="router-advertisement" reject`},
			want: []map[string]any{{
				"name":     "icmp-router-advertisement",
				"priority": -1,
				"family":   "ipv6",
				"action":   "reject",
				"icmp":     map[string]any{"type": "router-advertisement"},
			}},
		},
		{
			name:  "rule without icmp skipped",
			rules: []string{`rule family="ipv4" service name="ssh" accept`},
			want:  []map[string]any{},
		},
		{
			name: "multiple rules include only icmp",
			rules: []string{
				`rule priority="10" family="ipv4" icmp-type name="echo-reply" drop`,
				`rule family="ipv4" service name="http" accept`,
				`rule family="ipv6" icmp-block name="router-advertisement" reject`,
			},
			want: []map[string]any{
				{
					"name":     "icmp-echo-reply",
					"priority": 10,
					"family":   "ipv4",
					"action":   "drop",
					"icmp":     map[string]any{"type": "echo-reply"},
				},
				{
					"name":     "icmp-router-advertisement",
					"priority": -1,
					"family":   "ipv6",
					"action":   "reject",
					"icmp":     map[string]any{"type": "router-advertisement"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parsePolicyCustomFilters(tc.rules)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parsePolicyCustomFilters mismatch\nwant: %#v\n got: %#v", tc.want, got)
			}
		})
	}
}

func TestGetForwardPorts(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]any
		want     []map[string]any
		wantNil  bool
	}{
		{
			name:     "single port forward",
			settings: map[string]any{"forward_ports": []any{[]any{"80", "tcp", "8080", "192.168.1.1"}}},
			want: []map[string]any{{
				"proto": "tcp",
				"lower": 80,
				"to":    map[string]any{"addr": "192.168.1.1", "port": 8080},
			}},
		},
		{
			name:     "port range forward",
			settings: map[string]any{"forward_ports": []any{[]any{"1000-1005", "udp", "2000", "10.0.0.2"}}},
			want: []map[string]any{{
				"proto": "udp",
				"lower": 1000,
				"upper": 1005,
				"to":    map[string]any{"addr": "10.0.0.2", "port": 2000},
			}},
		},
		{
			name:     "missing to port defaults to lower",
			settings: map[string]any{"forward_ports": []any{[]any{"8081", "tcp", "", "192.168.1.1"}}},
			want: []map[string]any{{
				"proto": "tcp",
				"lower": 8081,
				"to":    map[string]any{"addr": "192.168.1.1", "port": 8081},
			}},
		},
		{
			name:     "missing forward ports",
			settings: map[string]any{},
			wantNil:  true,
		},
		{
			name:     "empty forward ports",
			settings: map[string]any{"forward_ports": []any{}},
			want:     []map[string]any{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := getForwardPorts(tc.settings)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %#v", got)
				}
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("getForwardPorts mismatch\nwant: %#v\n got: %#v", tc.want, got)
			}
		})
	}
}

func TestMapZoneTarget(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "percent reject", in: "%%REJECT%%", want: "reject"},
		{name: "reject", in: "REJECT", want: "reject"},
		{name: "drop", in: "DROP", want: "drop"},
		{name: "accept", in: "ACCEPT", want: "accept"},
		{name: "default", in: "DEFAULT", want: "accept"},
		{name: "empty", in: "", want: "accept"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := mapZoneTarget(tc.in); got != tc.want {
				t.Fatalf("mapZoneTarget(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMapPolicyTarget(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "continue", in: "CONTINUE", want: "continue"},
		{name: "accept", in: "ACCEPT", want: "accept"},
		{name: "drop", in: "DROP", want: "drop"},
		{name: "reject", in: "REJECT", want: "reject"},
		{name: "empty", in: "", want: "reject"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := mapPolicyTarget(tc.in); got != tc.want {
				t.Fatalf("mapPolicyTarget(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestAsBool(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want bool
	}{
		{name: "bool true", in: true, want: true},
		{name: "bool false", in: false, want: false},
		{name: "int one", in: 1, want: true},
		{name: "int zero", in: 0, want: false},
		{name: "int8 one", in: int8(1), want: true},
		{name: "int16 zero", in: int16(0), want: false},
		{name: "int64 one", in: int64(1), want: true},
		{name: "uint32 zero", in: uint32(0), want: false},
		{name: "uint64 one", in: uint64(1), want: true},
		{name: "string true", in: "true", want: true},
		{name: "string false", in: "false", want: false},
		{name: "string one", in: "1", want: true},
		{name: "string zero", in: "0", want: false},
		{name: "string yes", in: "yes", want: true},
		{name: "string no", in: "no", want: false},
		{name: "string on", in: "on", want: true},
		{name: "trim and case", in: "  TRUE  ", want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := asBool(tc.in); got != tc.want {
				t.Fatalf("asBool(%#v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestToUint64(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want uint64
	}{
		{name: "uint8", in: uint8(8), want: 8},
		{name: "uint16", in: uint16(16), want: 16},
		{name: "uint32", in: uint32(32), want: 32},
		{name: "uint64", in: uint64(64), want: 64},
		{name: "uint", in: uint(7), want: 7},
		{name: "int positive", in: 42, want: 42},
		{name: "int negative", in: -1, want: 0},
		{name: "int64 negative", in: int64(-9), want: 0},
		{name: "float64", in: float64(99.9), want: 99},
		{name: "float64 negative", in: float64(-0.1), want: 0},
		{name: "string number", in: "42", want: 42},
		{name: "string invalid", in: "nope", want: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := toUint64(tc.in); got != tc.want {
				t.Fatalf("toUint64(%#v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseHelpers(t *testing.T) {
	t.Run("parseQuotedName", func(t *testing.T) {
		tests := []struct {
			name string
			rule string
			want string
		}{
			{name: "extract name", rule: `rule icmp-type name="echo-request" accept`, want: "echo-request"},
			{name: "missing name", rule: `rule icmp-type accept`, want: ""},
			{name: "unterminated quote", rule: `rule icmp-type name="echo-request accept`, want: ""},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if got := parseQuotedName(tc.rule); got != tc.want {
					t.Fatalf("parseQuotedName(%q) = %q, want %q", tc.rule, got, tc.want)
				}
			})
		}
	})

	t.Run("parsePriority", func(t *testing.T) {
		tests := []struct {
			name string
			in   string
			want int
		}{
			{name: "quoted value", in: `"0" family="ipv4"`, want: 0},
			{name: "plain value", in: `10 family="ipv6"`, want: 10},
			{name: "empty", in: ``, want: -1},
			{name: "invalid", in: `abc family="ipv4"`, want: -1},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if got := parsePriority(tc.in); got != tc.want {
					t.Fatalf("parsePriority(%q) = %d, want %d", tc.in, got, tc.want)
				}
			})
		}
	})

	t.Run("hasImmutableTag", func(t *testing.T) {
		tests := []struct {
			name string
			in   string
			want bool
		}{
			{name: "has tag", in: "Public (immutable)", want: true},
			{name: "no tag", in: "Public", want: false},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if got := hasImmutableTag(tc.in); got != tc.want {
					t.Fatalf("hasImmutableTag(%q) = %v, want %v", tc.in, got, tc.want)
				}
			})
		}
	})
}

func TestSleepOrDone(t *testing.T) {
	tests := []struct {
		name      string
		cancelNow bool
		delay     time.Duration
		wantErr   bool
	}{
		{name: "done context returns error", cancelNow: true, delay: time.Millisecond, wantErr: true},
		{name: "sleep completes when context active", cancelNow: false, delay: time.Millisecond, wantErr: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			if tc.cancelNow {
				cancel()
			} else {
				defer cancel()
			}

			err := sleepOrDone(ctx, tc.delay)
			if tc.wantErr {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("expected context.Canceled, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestDecodeActiveZones(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want map[string]map[string]any
	}{
		{
			name: "godbus concrete type (a{sa{sas}})",
			in: map[string]map[string][]string{
				"public": {
					"interfaces": {"eth0", "eth1"},
					"sources":    {"10.0.0.0/8"},
				},
				"mgmt": {
					"interfaces": {"eth2"},
				},
			},
			want: map[string]map[string]any{
				"public": {
					"interfaces": []string{"eth0", "eth1"},
					"sources":    []string{"10.0.0.0/8"},
				},
				"mgmt": {
					"interfaces": []string{"eth2"},
				},
			},
		},
		{
			name: "pre-decoded map[string]map[string]any",
			in: map[string]map[string]any{
				"home": {
					"interfaces": []string{"wlan0"},
				},
			},
			want: map[string]map[string]any{
				"home": {
					"interfaces": []string{"wlan0"},
				},
			},
		},
		{
			name: "nil input",
			in:   nil,
			want: map[string]map[string]any{},
		},
		{
			name: "unsupported type",
			in:   "garbage",
			want: map[string]map[string]any{},
		},
		{
			name: "empty map",
			in:   map[string]map[string][]string{},
			want: map[string]map[string]any{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeActiveZones(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("decodeActiveZones() =\n  %v\nwant:\n  %v", got, tc.want)
			}
		})
	}
}

func TestNextDelay(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want time.Duration
	}{
		{name: "doubles normal delay", in: reconnectInitial, want: reconnectInitial * 2},
		{name: "caps at reconnectMax", in: reconnectMax, want: reconnectMax},
		{name: "near max also caps", in: reconnectMax - time.Second, want: reconnectMax},
		{name: "zero becomes reconnectInitial", in: 0, want: reconnectInitial},
		{name: "negative becomes reconnectInitial", in: -time.Second, want: reconnectInitial},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextDelay(tc.in); got != tc.want {
				t.Fatalf("nextDelay(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
