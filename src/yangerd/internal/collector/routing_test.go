package collector

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/testutil"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

// Canned FRR JSON matching real /usr/libexec/statd/ospf-status output.
const testOSPFStatus = `{
  "routerId": "10.0.0.1",
  "areas": {
    "0.0.0.0": {
      "area-type": "ietf-ospf:normal-area",
      "interfaces": [
        {
          "name": "e0",
          "state": "DR",
          "ospfEnabled": true,
          "networkType": "BROADCAST",
          "cost": 10,
          "priority": 1,
          "timerDeadSecs": 40,
          "timerRetransmitSecs": 5,
          "transmitDelaySecs": 1,
          "timerMsecs": 10000,
          "timerHelloInMsecs": 7000,
          "timerWaitSecs": 40,
          "drId": "10.0.0.1",
          "drAddress": "192.168.1.1",
          "bdrId": "10.0.0.2",
          "bdrAddress": "192.168.1.2",
          "neighbors": [
            {
              "neighborIp": "10.0.0.2",
              "ifaceAddress": "192.168.1.2",
              "nbrPriority": 1,
              "nbrState": "Full/DR",
              "role": "Backup",
              "lastPrgrsvChangeMsec": 120000,
              "routerDeadIntervalTimerDueMsec": 35000,
              "routerDesignatedId": "10.0.0.1",
              "routerDesignatedBackupId": "10.0.0.2",
              "ifaceName": "e0",
              "localIfaceAddress": "192.168.1.1"
            }
          ]
        },
        {
          "name": "lo",
          "state": "Loopback",
          "ospfEnabled": true,
          "networkType": "POINTOPOINT",
          "cost": 0,
          "priority": 0,
          "timerPassiveIface": true,
          "timerDeadSecs": 0,
          "timerRetransmitSecs": 0,
          "transmitDelaySecs": 0,
          "timerMsecs": 10000,
          "neighbors": []
        }
      ]
    }
  }
}`

const testOSPFRoutes = `{
  "10.0.0.0/24": {
    "routeType": "N IA",
    "area": "0.0.0.0",
    "cost": 20,
    "nexthops": [
      {"ip": "192.168.1.2", "via": "e0"}
    ]
  },
  "10.0.1.0/24": {
    "routeType": "N E2",
    "area": "0.0.0.0",
    "cost": 100,
    "tag": 42,
    "nexthops": [
      {"ip": " ", "directlyAttachedTo": "e0"}
    ]
  }
}`

const testRIPStatus = `Routing Protocol is "rip"
  Sending updates every 30 seconds with +/-50%, next due in 12 seconds
  Timeout after 180 seconds, garbage collect after 120 seconds
  Outgoing update filter list for all interface is not set
  Incoming update filter list for all interface is not set
  Default redistribution metric is 1
  Redistributing:
  Default version control: send version 2, receive version 2
    Interface        Send  Recv   Key-chain
    e0               2     2
    e1               2     2
  Routing for Networks:
    10.0.0.0/24
    10.0.1.0/24
  Routing Information Sources:
    Gateway          BadPackets BadRoutes  Distance Last Update
    10.0.0.2                 0         0       120   00:00:12
    10.0.0.3                 1         2       120   00:00:25
  Distance: (default is 120)
`

const testRIPRoutes = `{
  "10.0.0.0/24": [
    {
      "prefix": "10.0.0.0/24",
      "protocol": "rip",
      "metric": 1,
      "nexthops": [
        {"ip": "10.0.0.2", "interfaceName": "e0"}
      ]
    }
  ],
  "10.0.1.0/24": [
    {
      "prefix": "10.0.1.0/24",
      "protocol": "rip",
      "metric": 2,
      "nexthops": [
        {"ip": "10.0.0.3", "interfaceName": "e1"}
      ]
    }
  ]
}`

const testBFDPeers = `[
  {
    "multihop": false,
    "peer": "10.0.0.2",
    "interface": "e0",
    "id": 1,
    "remote-id": 2,
    "status": "up",
    "receive-interval": 300,
    "transmit-interval": 300,
    "detect-multiplier": 3
  },
  {
    "multihop": true,
    "peer": "10.0.0.99",
    "interface": "e1",
    "id": 5,
    "remote-id": 6,
    "status": "down"
  }
]`

func newRoutingCollector(runner *testutil.MockRunner) *RoutingCollector {
	return NewRoutingCollector(runner, 10*time.Second)
}

func routingCollect(t *testing.T, runner *testutil.MockRunner) map[string]interface{} {
	t.Helper()
	c := newRoutingCollector(runner)
	tr := tree.New()
	if err := c.Collect(context.Background(), tr); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	raw := tr.Get("ietf-routing:routing")
	if raw == nil {
		t.Fatal("missing ietf-routing:routing in tree")
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal routing: %v", err)
	}
	return out
}

func fullRunner() *testutil.MockRunner {
	return &testutil.MockRunner{
		Results: map[string][]byte{
			"/usr/libexec/statd/ospf-status":   []byte(testOSPFStatus),
			"vtysh -c show ip ospf route json": []byte(testOSPFRoutes),
			"vtysh -c show ip rip status":      []byte(testRIPStatus),
			"vtysh -c show ip route rip json":  []byte(testRIPRoutes),
			"vtysh -c show bfd peers json":     []byte(testBFDPeers),
		},
		Errors: map[string]error{},
	}
}

func TestRoutingCollectorNameAndInterval(t *testing.T) {
	c := newRoutingCollector(fullRunner())
	if c.Name() != "routing" {
		t.Fatalf("expected name 'routing', got %q", c.Name())
	}
	if c.Interval() != 10*time.Second {
		t.Fatalf("expected interval 10s, got %v", c.Interval())
	}
}

func TestRoutingCollectorMergesThreeProtocols(t *testing.T) {
	out := routingCollect(t, fullRunner())
	cpp := out["control-plane-protocols"].(map[string]interface{})
	protocols := cpp["control-plane-protocol"].([]interface{})
	if len(protocols) != 3 {
		t.Fatalf("expected 3 protocols (OSPF+RIP+BFD), got %d", len(protocols))
	}

	types := make(map[string]bool)
	for _, p := range protocols {
		pm := p.(map[string]interface{})
		types[pm["type"].(string)] = true
	}
	for _, expected := range []string{"infix-routing:ospfv2", "infix-routing:ripv2", "infix-routing:bfdv1"} {
		if !types[expected] {
			t.Fatalf("missing protocol type %q; got %v", expected, types)
		}
	}
}

// --- OSPF tests ---

func getOSPFProtocol(t *testing.T, out map[string]interface{}) map[string]interface{} {
	t.Helper()
	cpp := out["control-plane-protocols"].(map[string]interface{})
	for _, p := range cpp["control-plane-protocol"].([]interface{}) {
		pm := p.(map[string]interface{})
		if pm["type"] == "infix-routing:ospfv2" {
			return pm
		}
	}
	t.Fatal("OSPF protocol not found")
	return nil
}

func TestOSPFRouterID(t *testing.T) {
	out := routingCollect(t, fullRunner())
	ospfProto := getOSPFProtocol(t, out)
	ospf := ospfProto["ietf-ospf:ospf"].(map[string]interface{})
	if ospf["ietf-ospf:router-id"] != "10.0.0.1" {
		t.Fatalf("router-id: expected 10.0.0.1, got %v", ospf["ietf-ospf:router-id"])
	}
	if ospf["ietf-ospf:address-family"] != "ipv4" {
		t.Fatalf("address-family: expected ipv4, got %v", ospf["ietf-ospf:address-family"])
	}
}

func TestOSPFAreaAndInterfaces(t *testing.T) {
	out := routingCollect(t, fullRunner())
	ospfProto := getOSPFProtocol(t, out)
	ospf := ospfProto["ietf-ospf:ospf"].(map[string]interface{})
	areasContainer := ospf["ietf-ospf:areas"].(map[string]interface{})
	areas := areasContainer["ietf-ospf:area"].([]interface{})
	if len(areas) != 1 {
		t.Fatalf("expected 1 area, got %d", len(areas))
	}

	area := areas[0].(map[string]interface{})
	if area["ietf-ospf:area-id"] != "0.0.0.0" {
		t.Fatalf("area-id: expected 0.0.0.0, got %v", area["ietf-ospf:area-id"])
	}

	ifacesContainer := area["ietf-ospf:interfaces"].(map[string]interface{})
	ifaces := ifacesContainer["ietf-ospf:interface"].([]interface{})
	if len(ifaces) != 2 {
		t.Fatalf("expected 2 interfaces, got %d", len(ifaces))
	}

	// First interface: e0 (DR)
	e0 := ifaces[0].(map[string]interface{})
	if e0["name"] != "e0" {
		t.Fatalf("interface[0] name: expected e0, got %v", e0["name"])
	}
	if e0["state"] != "dr" {
		t.Fatalf("interface[0] state: expected dr, got %v", e0["state"])
	}
	if e0["interface-type"] != "broadcast" {
		t.Fatalf("interface[0] type: expected broadcast, got %v", e0["interface-type"])
	}
	if e0["passive"] != false {
		t.Fatalf("e0 passive: expected false, got %v", e0["passive"])
	}
	if e0["enabled"] != true {
		t.Fatalf("e0 enabled: expected true, got %v", e0["enabled"])
	}
	if e0["dr-router-id"] != "10.0.0.1" {
		t.Fatalf("e0 dr-router-id: expected 10.0.0.1, got %v", e0["dr-router-id"])
	}

	// Check timer conversions (ms → seconds)
	if toInt(e0["hello-interval"]) != 10 {
		t.Fatalf("e0 hello-interval: expected 10, got %v", e0["hello-interval"])
	}
	if toInt(e0["hello-timer"]) != 7 {
		t.Fatalf("e0 hello-timer: expected 7, got %v", e0["hello-timer"])
	}
	if toInt(e0["cost"]) != 10 {
		t.Fatalf("e0 cost: expected 10, got %v", e0["cost"])
	}

	// Second interface: lo (passive loopback)
	lo := ifaces[1].(map[string]interface{})
	if lo["name"] != "lo" {
		t.Fatalf("interface[1] name: expected lo, got %v", lo["name"])
	}
	if lo["state"] != "loopback" {
		t.Fatalf("lo state: expected loopback, got %v", lo["state"])
	}
	if lo["passive"] != true {
		t.Fatalf("lo passive: expected true, got %v", lo["passive"])
	}
	if lo["interface-type"] != "point-to-point" {
		t.Fatalf("lo type: expected point-to-point, got %v", lo["interface-type"])
	}
}

func TestOSPFNeighbors(t *testing.T) {
	out := routingCollect(t, fullRunner())
	ospfProto := getOSPFProtocol(t, out)
	ospf := ospfProto["ietf-ospf:ospf"].(map[string]interface{})
	areasContainer := ospf["ietf-ospf:areas"].(map[string]interface{})
	area := areasContainer["ietf-ospf:area"].([]interface{})[0].(map[string]interface{})
	ifacesContainer := area["ietf-ospf:interfaces"].(map[string]interface{})
	e0 := ifacesContainer["ietf-ospf:interface"].([]interface{})[0].(map[string]interface{})

	neighborsContainer := e0["ietf-ospf:neighbors"].(map[string]interface{})
	neighbors := neighborsContainer["ietf-ospf:neighbor"].([]interface{})
	if len(neighbors) != 1 {
		t.Fatalf("expected 1 neighbor, got %d", len(neighbors))
	}

	n := neighbors[0].(map[string]interface{})
	if n["neighbor-router-id"] != "10.0.0.2" {
		t.Fatalf("neighbor router-id: expected 10.0.0.2, got %v", n["neighbor-router-id"])
	}
	if n["address"] != "192.168.1.2" {
		t.Fatalf("neighbor address: expected 192.168.1.2, got %v", n["address"])
	}
	if n["state"] != "full" {
		t.Fatalf("neighbor state: expected full, got %v", n["state"])
	}
	if n["infix-routing:role"] != "BDR" {
		t.Fatalf("neighbor role: expected BDR, got %v", n["infix-routing:role"])
	}
	// Uptime: 120000ms → 120s
	if toInt(n["infix-routing:uptime"]) != 120 {
		t.Fatalf("neighbor uptime: expected 120, got %v", n["infix-routing:uptime"])
	}
	// Dead timer: 35000ms → 35s
	if toInt(n["dead-timer"]) != 35 {
		t.Fatalf("neighbor dead-timer: expected 35, got %v", n["dead-timer"])
	}
	// Interface name augmentation
	if n["infix-routing:interface-name"] != "e0:192.168.1.1" {
		t.Fatalf("neighbor interface-name: expected e0:192.168.1.1, got %v", n["infix-routing:interface-name"])
	}
}

func TestOSPFRoutes(t *testing.T) {
	out := routingCollect(t, fullRunner())
	ospfProto := getOSPFProtocol(t, out)
	ospf := ospfProto["ietf-ospf:ospf"].(map[string]interface{})
	rib := ospf["ietf-ospf:local-rib"].(map[string]interface{})
	routes := rib["ietf-ospf:route"].([]interface{})
	if len(routes) != 2 {
		t.Fatalf("expected 2 OSPF routes, got %d", len(routes))
	}

	routeByPrefix := make(map[string]map[string]interface{})
	for _, r := range routes {
		rm := r.(map[string]interface{})
		routeByPrefix[rm["prefix"].(string)] = rm
	}

	// Inter-area route
	r1 := routeByPrefix["10.0.0.0/24"]
	if r1 == nil {
		t.Fatal("missing route 10.0.0.0/24")
	}
	if r1["route-type"] != "inter-area" {
		t.Fatalf("route 10.0.0.0/24 type: expected inter-area, got %v", r1["route-type"])
	}

	// External-2 route with tag
	r2 := routeByPrefix["10.0.1.0/24"]
	if r2 == nil {
		t.Fatal("missing route 10.0.1.0/24")
	}
	if r2["route-type"] != "external-2" {
		t.Fatalf("route 10.0.1.0/24 type: expected external-2, got %v", r2["route-type"])
	}
	// tag should be present
	if r2["route-tag"] == nil {
		t.Fatal("route 10.0.1.0/24 should have route-tag")
	}
	// Directly attached nexthop
	nhs := r2["next-hops"].(map[string]interface{})
	nhList := nhs["next-hop"].([]interface{})
	if len(nhList) != 1 {
		t.Fatalf("expected 1 nexthop, got %d", len(nhList))
	}
	nh := nhList[0].(map[string]interface{})
	if nh["outgoing-interface"] != "e0" {
		t.Fatalf("nexthop outgoing-interface: expected e0, got %v", nh["outgoing-interface"])
	}
}

// --- RIP tests ---

func getRIPProtocol(t *testing.T, out map[string]interface{}) map[string]interface{} {
	t.Helper()
	cpp := out["control-plane-protocols"].(map[string]interface{})
	for _, p := range cpp["control-plane-protocol"].([]interface{}) {
		pm := p.(map[string]interface{})
		if pm["type"] == "infix-routing:ripv2" {
			return pm
		}
	}
	t.Fatal("RIP protocol not found")
	return nil
}

func TestRIPTimers(t *testing.T) {
	out := routingCollect(t, fullRunner())
	ripProto := getRIPProtocol(t, out)
	rip := ripProto["ietf-rip:rip"].(map[string]interface{})

	timers := rip["timers"].(map[string]interface{})
	if toInt(timers["update-interval"]) != 30 {
		t.Fatalf("RIP update-interval: expected 30, got %v", timers["update-interval"])
	}
	if toInt(timers["invalid-interval"]) != 180 {
		t.Fatalf("RIP invalid-interval: expected 180, got %v", timers["invalid-interval"])
	}
	if toInt(timers["flush-interval"]) != 120 {
		t.Fatalf("RIP flush-interval: expected 120, got %v", timers["flush-interval"])
	}

	if toInt(rip["default-metric"]) != 1 {
		t.Fatalf("RIP default-metric: expected 1, got %v", rip["default-metric"])
	}
	if toInt(rip["distance"]) != 120 {
		t.Fatalf("RIP distance: expected 120, got %v", rip["distance"])
	}
}

func TestRIPInterfaces(t *testing.T) {
	out := routingCollect(t, fullRunner())
	ripProto := getRIPProtocol(t, out)
	rip := ripProto["ietf-rip:rip"].(map[string]interface{})

	ifContainer := rip["interfaces"].(map[string]interface{})
	ifaces := ifContainer["interface"].([]interface{})
	if len(ifaces) != 2 {
		t.Fatalf("expected 2 RIP interfaces, got %d", len(ifaces))
	}

	iface0 := ifaces[0].(map[string]interface{})
	if iface0["interface"] != "e0" {
		t.Fatalf("RIP iface[0]: expected e0, got %v", iface0["interface"])
	}
	if iface0["oper-status"] != "up" {
		t.Fatalf("RIP iface[0] status: expected up, got %v", iface0["oper-status"])
	}
	if iface0["send-version"] != "2" {
		t.Fatalf("RIP iface[0] send-version: expected '2', got %v", iface0["send-version"])
	}
}

func TestRIPRoutes(t *testing.T) {
	out := routingCollect(t, fullRunner())
	ripProto := getRIPProtocol(t, out)
	rip := ripProto["ietf-rip:rip"].(map[string]interface{})

	ipv4 := rip["ipv4"].(map[string]interface{})
	routesContainer := ipv4["routes"].(map[string]interface{})
	routes := routesContainer["route"].([]interface{})
	if len(routes) != 2 {
		t.Fatalf("expected 2 RIP routes, got %d", len(routes))
	}

	if toInt(rip["num-of-routes"]) != 2 {
		t.Fatalf("RIP num-of-routes: expected 2, got %v", rip["num-of-routes"])
	}
}

func TestRIPNeighbors(t *testing.T) {
	out := routingCollect(t, fullRunner())
	ripProto := getRIPProtocol(t, out)
	rip := ripProto["ietf-rip:rip"].(map[string]interface{})

	ipv4 := rip["ipv4"].(map[string]interface{})
	neighContainer := ipv4["neighbors"].(map[string]interface{})
	neighs := neighContainer["neighbor"].([]interface{})
	if len(neighs) != 2 {
		t.Fatalf("expected 2 RIP neighbors, got %d", len(neighs))
	}

	n0 := neighs[0].(map[string]interface{})
	if n0["ipv4-address"] != "10.0.0.2" {
		t.Fatalf("RIP neighbor[0] address: expected 10.0.0.2, got %v", n0["ipv4-address"])
	}
	// Bad packets/routes should be int (from text parse)
	if toInt(n0["bad-packets-rcvd"]) != 0 {
		t.Fatalf("RIP neighbor[0] bad-packets: expected 0, got %v", n0["bad-packets-rcvd"])
	}

	n1 := neighs[1].(map[string]interface{})
	if toInt(n1["bad-packets-rcvd"]) != 1 {
		t.Fatalf("RIP neighbor[1] bad-packets: expected 1, got %v", n1["bad-packets-rcvd"])
	}
	if toInt(n1["bad-routes-rcvd"]) != 2 {
		t.Fatalf("RIP neighbor[1] bad-routes: expected 2, got %v", n1["bad-routes-rcvd"])
	}
}

// --- BFD tests ---

func getBFDProtocol(t *testing.T, out map[string]interface{}) map[string]interface{} {
	t.Helper()
	cpp := out["control-plane-protocols"].(map[string]interface{})
	for _, p := range cpp["control-plane-protocol"].([]interface{}) {
		pm := p.(map[string]interface{})
		if pm["type"] == "infix-routing:bfdv1" {
			return pm
		}
	}
	t.Fatal("BFD protocol not found")
	return nil
}

func TestBFDSessions(t *testing.T) {
	out := routingCollect(t, fullRunner())
	bfdProto := getBFDProtocol(t, out)
	bfd := bfdProto["ietf-bfd:bfd"].(map[string]interface{})
	ipsh := bfd["ietf-bfd-ip-sh:ip-sh"].(map[string]interface{})
	sessionsContainer := ipsh["sessions"].(map[string]interface{})
	sessions := sessionsContainer["session"].([]interface{})

	// Only single-hop sessions included (multihop=true is filtered)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 BFD session (multihop filtered), got %d", len(sessions))
	}

	s := sessions[0].(map[string]interface{})
	if s["interface"] != "e0" {
		t.Fatalf("BFD session interface: expected e0, got %v", s["interface"])
	}
	if s["dest-addr"] != "10.0.0.2" {
		t.Fatalf("BFD session dest-addr: expected 10.0.0.2, got %v", s["dest-addr"])
	}
	if s["path-type"] != "ietf-bfd-types:path-ip-sh" {
		t.Fatalf("BFD path-type: expected ietf-bfd-types:path-ip-sh, got %v", s["path-type"])
	}

	running := s["session-running"].(map[string]interface{})
	if running["local-state"] != "up" {
		t.Fatalf("BFD local-state: expected up, got %v", running["local-state"])
	}
	if running["detection-mode"] != "async-without-echo" {
		t.Fatalf("BFD detection-mode: expected async-without-echo, got %v", running["detection-mode"])
	}

	// Intervals: ms → µs (×1000)
	// receive-interval=300ms → 300000µs
	if toInt(running["negotiated-rx-interval"]) != 300000 {
		t.Fatalf("BFD rx-interval: expected 300000, got %v", running["negotiated-rx-interval"])
	}
	if toInt(running["negotiated-tx-interval"]) != 300000 {
		t.Fatalf("BFD tx-interval: expected 300000, got %v", running["negotiated-tx-interval"])
	}
	// detection-time = detect-multiplier * receive-interval * 1000 = 3 * 300 * 1000 = 900000
	if toInt(running["detection-time"]) != 900000 {
		t.Fatalf("BFD detection-time: expected 900000, got %v", running["detection-time"])
	}
}

// --- Graceful degradation tests ---

func TestRoutingCollectorOSPFOnly(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"/usr/libexec/statd/ospf-status":   []byte(testOSPFStatus),
			"vtysh -c show ip ospf route json": []byte(testOSPFRoutes),
		},
		Errors: map[string]error{},
	}

	out := routingCollect(t, runner)
	cpp := out["control-plane-protocols"].(map[string]interface{})
	protocols := cpp["control-plane-protocol"].([]interface{})
	if len(protocols) != 1 {
		t.Fatalf("expected 1 protocol when only OSPF available, got %d", len(protocols))
	}
	pm := protocols[0].(map[string]interface{})
	if pm["type"] != "infix-routing:ospfv2" {
		t.Fatalf("expected OSPF protocol, got %v", pm["type"])
	}
}

func TestRoutingCollectorAllFail(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{},
		Errors:  map[string]error{},
	}

	c := newRoutingCollector(runner)
	tr := tree.New()
	err := c.Collect(context.Background(), tr)
	if err != nil {
		t.Fatalf("Collect should not error when all protocols fail: %v", err)
	}
	// No tree key should be set when there's nothing to report
	if tr.Get("ietf-routing:routing") != nil {
		t.Fatal("expected no ietf-routing:routing key when all protocols fail")
	}
}

func TestRIPStatusParsing(t *testing.T) {
	status := parseRIPStatus(testRIPStatus)

	if status["update-interval"] != 30 {
		t.Fatalf("update-interval: expected 30, got %v", status["update-interval"])
	}
	if status["invalid-interval"] != 180 {
		t.Fatalf("invalid-interval: expected 180, got %v", status["invalid-interval"])
	}
	if status["flush-interval"] != 120 {
		t.Fatalf("flush-interval: expected 120, got %v", status["flush-interval"])
	}
	if status["default-metric"] != 1 {
		t.Fatalf("default-metric: expected 1, got %v", status["default-metric"])
	}
	if status["distance"] != 120 {
		t.Fatalf("distance: expected 120, got %v", status["distance"])
	}

	ifaces := status["interfaces"].([]interface{})
	if len(ifaces) != 2 {
		t.Fatalf("expected 2 parsed interfaces, got %d", len(ifaces))
	}

	neighs := status["neighbors"].([]interface{})
	if len(neighs) != 2 {
		t.Fatalf("expected 2 parsed neighbors, got %d", len(neighs))
	}
}

func TestFrrToIETFNeighborState(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Full/DR", "full"},
		{"TwoWay/DROther", "2-way"},
		{"Init/DROther", "init"},
		{"Down/DROther", "down"},
		{"ExStart", "exstart"},
	}
	for _, tt := range tests {
		got := frrToIETFNeighborState(tt.input)
		if got != tt.expected {
			t.Fatalf("frrToIETFNeighborState(%q): expected %q, got %q", tt.input, tt.expected, got)
		}
	}
}

func TestOSPFNetworkType(t *testing.T) {
	tests := []struct {
		nt       string
		p2mpNB   bool
		expected string
	}{
		{"POINTOPOINT", false, "point-to-point"},
		{"BROADCAST", false, "broadcast"},
		{"POINTOMULTIPOINT", false, "hybrid"},
		{"POINTOMULTIPOINT", true, "point-to-multipoint"},
		{"NBMA", false, "non-broadcast"},
		{"UNKNOWN", false, ""},
	}
	for _, tt := range tests {
		got := ospfNetworkType(tt.nt, tt.p2mpNB)
		if got != tt.expected {
			t.Fatalf("ospfNetworkType(%q, %v): expected %q, got %q", tt.nt, tt.p2mpNB, tt.expected, got)
		}
	}
}

func TestBFDMultihopFiltered(t *testing.T) {
	// Ensure multihop peers don't appear in output
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"vtysh -c show bfd peers json": []byte(`[
				{"multihop": true, "peer": "10.0.0.99", "interface": "e1", "id": 5, "status": "up"}
			]`),
		},
		Errors: map[string]error{},
	}

	c := newRoutingCollector(runner)
	tr := tree.New()
	c.Collect(context.Background(), tr)

	// BFD should not set anything when all peers are multihop
	raw := tr.Get("ietf-routing:routing")
	if raw != nil {
		// If routing is set, BFD should not be present
		var out map[string]interface{}
		json.Unmarshal(raw, &out)
		cpp := out["control-plane-protocols"].(map[string]interface{})
		protocols := cpp["control-plane-protocol"].([]interface{})
		for _, p := range protocols {
			pm := p.(map[string]interface{})
			if pm["type"] == "infix-routing:bfdv1" {
				t.Fatal("multihop-only BFD should not produce a protocol entry")
			}
		}
	}
}
