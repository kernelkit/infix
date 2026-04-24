package zapi

import (
	"bytes"
	"encoding/binary"
	"net"
	"syscall"
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

func TestDecodeRouteIPv4(t *testing.T) {
	body := buildRouteBody(t, syscall.AF_INET,
		net.ParseIP("10.0.0.0").To4(), 24,
		uint32(MsgNexthop|MsgDistance|MsgMetric),
		RouteConnect, 0,
		[]testNexthop{
			{nhType: NHIPv4IFIndex, gate: net.ParseIP("10.0.0.1").To4(), ifindex: 2},
		},
		10, 100,
	)

	route, err := DecodeRoute(body)
	if err != nil {
		t.Fatal(err)
	}

	if route.Type != RouteConnect {
		t.Errorf("Type = %d, want %d", route.Type, RouteConnect)
	}
	want := "10.0.0.0/24"
	if route.Prefix.String() != want {
		t.Errorf("Prefix = %s, want %s", route.Prefix.String(), want)
	}
	if len(route.Nexthops) != 1 {
		t.Fatalf("Nexthops = %d, want 1", len(route.Nexthops))
	}
	if route.Nexthops[0].Gate.String() != "10.0.0.1" {
		t.Errorf("Gate = %s, want 10.0.0.1", route.Nexthops[0].Gate)
	}
	if route.Nexthops[0].Ifindex != 2 {
		t.Errorf("Ifindex = %d, want 2", route.Nexthops[0].Ifindex)
	}
	if route.Distance != 10 {
		t.Errorf("Distance = %d, want 10", route.Distance)
	}
	if route.Metric != 100 {
		t.Errorf("Metric = %d, want 100", route.Metric)
	}
}

func TestDecodeRouteIPv6(t *testing.T) {
	body := buildRouteBody(t, syscall.AF_INET6,
		net.ParseIP("2001:db8::").To16(), 48,
		uint32(MsgNexthop|MsgMetric),
		RouteOSPF, 0,
		[]testNexthop{
			{nhType: NHIPv6IFIndex, gate: net.ParseIP("2001:db8::1").To16(), ifindex: 3},
		},
		0, 200,
	)

	route, err := DecodeRoute(body)
	if err != nil {
		t.Fatal(err)
	}

	want := "2001:db8::/48"
	if route.Prefix.String() != want {
		t.Errorf("Prefix = %s, want %s", route.Prefix.String(), want)
	}
	if route.Metric != 200 {
		t.Errorf("Metric = %d, want 200", route.Metric)
	}
	if len(route.Nexthops) != 1 {
		t.Fatalf("Nexthops = %d, want 1", len(route.Nexthops))
	}
	if route.Nexthops[0].Gate.String() != "2001:db8::1" {
		t.Errorf("Gate = %s, want 2001:db8::1", route.Nexthops[0].Gate)
	}
}

func TestDecodeRouteNoNexthops(t *testing.T) {
	body := buildRouteBody(t, syscall.AF_INET,
		net.ParseIP("192.168.1.0").To4(), 24,
		uint32(MsgMetric),
		RouteKernel, 0,
		nil,
		0, 50,
	)

	route, err := DecodeRoute(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(route.Nexthops) != 0 {
		t.Errorf("Nexthops = %d, want 0", len(route.Nexthops))
	}
	if route.Metric != 50 {
		t.Errorf("Metric = %d, want 50", route.Metric)
	}
}

func TestDecodeRouteMultipleNexthops(t *testing.T) {
	body := buildRouteBody(t, syscall.AF_INET,
		net.ParseIP("10.0.0.0").To4(), 8,
		uint32(MsgNexthop),
		RouteStatic, 0,
		[]testNexthop{
			{nhType: NHIPv4IFIndex, gate: net.ParseIP("10.0.0.1").To4(), ifindex: 1},
			{nhType: NHIPv4IFIndex, gate: net.ParseIP("10.0.0.2").To4(), ifindex: 2},
		},
		0, 0,
	)

	route, err := DecodeRoute(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(route.Nexthops) != 2 {
		t.Fatalf("Nexthops = %d, want 2", len(route.Nexthops))
	}
	if route.Nexthops[1].Gate.String() != "10.0.0.2" {
		t.Errorf("Gate[1] = %s, want 10.0.0.2", route.Nexthops[1].Gate)
	}
}

func TestDecodeRouteDefaultRoute(t *testing.T) {
	body := buildRouteBody(t, syscall.AF_INET,
		net.ParseIP("0.0.0.0").To4(), 0,
		uint32(MsgNexthop|MsgMetric),
		RouteStatic, 0,
		[]testNexthop{
			{nhType: NHIPv4IFIndex, gate: net.ParseIP("192.168.1.1").To4(), ifindex: 5},
		},
		0, 0,
	)

	route, err := DecodeRoute(body)
	if err != nil {
		t.Fatal(err)
	}
	if route.Prefix.String() != "0.0.0.0/0" {
		t.Errorf("Prefix = %s, want 0.0.0.0/0", route.Prefix.String())
	}
}

func TestDecodeRouteIFIndexOnly(t *testing.T) {
	body := buildRouteBody(t, syscall.AF_INET,
		net.ParseIP("10.0.0.0").To4(), 24,
		uint32(MsgNexthop),
		RouteConnect, 0,
		[]testNexthop{
			{nhType: NHIFIndex, ifindex: 7},
		},
		0, 0,
	)

	route, err := DecodeRoute(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(route.Nexthops) != 1 {
		t.Fatalf("Nexthops = %d, want 1", len(route.Nexthops))
	}
	if route.Nexthops[0].Ifindex != 7 {
		t.Errorf("Ifindex = %d, want 7", route.Nexthops[0].Ifindex)
	}
}

type testNexthop struct {
	nhType  NHType
	gate    net.IP
	ifindex uint32
}

// TestDecodeRouteFRRRedistribute tests the exact wire format that FRR's
// zsend_redistribute_route produces: message flags 0x17 = nexthop|distance|metric|mtu.
func TestDecodeRouteFRRRedistribute(t *testing.T) {
	msgFlags := uint32(MsgNexthop | MsgDistance | MsgMetric | MsgMTU)

	var body []byte
	body = append(body, uint8(RouteRIP)) // type
	body = append(body, 0, 0)            // instance
	tmp := make([]byte, 4)
	binary.BigEndian.PutUint32(tmp, 0) // flags
	body = append(body, tmp...)
	binary.BigEndian.PutUint32(tmp, msgFlags)
	body = append(body, tmp...)
	body = append(body, 1)               // safi=unicast
	body = append(body, syscall.AF_INET) // family
	body = append(body, 24)              // prefixlen
	body = append(body, 192, 168, 10)    // prefix (3 bytes for /24)

	// 1 nexthop: type=IFINDEX, flags=0, ifindex=5
	binary.BigEndian.PutUint16(tmp[:2], 1) // nexthop count
	body = append(body, tmp[:2]...)
	body = append(body, 0, 0, 0, 0)       // vrfID
	body = append(body, uint8(NHIFIndex)) // nh type
	body = append(body, 0)                // nh flags
	binary.BigEndian.PutUint32(tmp, 5)    // ifindex
	body = append(body, tmp...)

	body = append(body, 120)           // distance (RIP=120)
	binary.BigEndian.PutUint32(tmp, 3) // metric
	body = append(body, tmp...)
	binary.BigEndian.PutUint32(tmp, 1500) // mtu
	body = append(body, tmp...)

	route, err := DecodeRoute(body)
	if err != nil {
		t.Fatal(err)
	}
	if route.Type != RouteRIP {
		t.Errorf("Type = %d, want %d", route.Type, RouteRIP)
	}
	if route.Prefix.String() != "192.168.10.0/24" {
		t.Errorf("Prefix = %s, want 192.168.10.0/24", route.Prefix.String())
	}
	if route.Distance != 120 {
		t.Errorf("Distance = %d, want 120", route.Distance)
	}
	if route.Metric != 3 {
		t.Errorf("Metric = %d, want 3", route.Metric)
	}
	if route.MTU != 1500 {
		t.Errorf("MTU = %d, want 1500", route.MTU)
	}
	if len(route.Nexthops) != 1 {
		t.Fatalf("Nexthops = %d, want 1", len(route.Nexthops))
	}
	if route.Nexthops[0].Ifindex != 5 {
		t.Errorf("Ifindex = %d, want 5", route.Nexthops[0].Ifindex)
	}
}

// TestDecodeRouteWithTag tests message flags 0x1f = nexthop|distance|metric|tag|mtu.
func TestDecodeRouteWithTag(t *testing.T) {
	msgFlags := uint32(MsgNexthop | MsgDistance | MsgMetric | MsgTag | MsgMTU)

	var body []byte
	body = append(body, uint8(RouteOSPF)) // type
	body = append(body, 0, 0)             // instance
	tmp := make([]byte, 4)
	binary.BigEndian.PutUint32(tmp, 0)
	body = append(body, tmp...) // flags
	binary.BigEndian.PutUint32(tmp, msgFlags)
	body = append(body, tmp...)          // message
	body = append(body, 1)               // safi
	body = append(body, syscall.AF_INET) // family
	body = append(body, 32)              // prefixlen
	body = append(body, 10, 1, 1, 1)     // prefix

	binary.BigEndian.PutUint16(tmp[:2], 1)
	body = append(body, tmp[:2]...) // nexthop count
	body = append(body, 0, 0, 0, 0) // vrfID
	body = append(body, uint8(NHIPv4IFIndex))
	body = append(body, 0)           // nh flags
	body = append(body, 10, 0, 0, 1) // gate
	binary.BigEndian.PutUint32(tmp, 3)
	body = append(body, tmp...) // ifindex

	body = append(body, 110) // distance (OSPF=110)
	binary.BigEndian.PutUint32(tmp, 20)
	body = append(body, tmp...) // metric
	binary.BigEndian.PutUint32(tmp, 42)
	body = append(body, tmp...) // tag
	binary.BigEndian.PutUint32(tmp, 9000)
	body = append(body, tmp...) // mtu

	route, err := DecodeRoute(body)
	if err != nil {
		t.Fatal(err)
	}
	if route.Distance != 110 {
		t.Errorf("Distance = %d, want 110", route.Distance)
	}
	if route.Metric != 20 {
		t.Errorf("Metric = %d, want 20", route.Metric)
	}
	if route.Tag != 42 {
		t.Errorf("Tag = %d, want 42", route.Tag)
	}
	if route.MTU != 9000 {
		t.Errorf("MTU = %d, want 9000", route.MTU)
	}
}

func buildRouteBody(t *testing.T, family uint8, prefix net.IP, prefixLen uint8, msgFlags uint32, routeType RouteType, flags uint32, nexthops []testNexthop, distance uint8, metric uint32) []byte {
	t.Helper()
	var buf []byte

	buf = append(buf, uint8(routeType))

	// instance(2)
	buf = append(buf, 0, 0)

	// flags(4)
	tmp := make([]byte, 4)
	binary.BigEndian.PutUint32(tmp, flags)
	buf = append(buf, tmp...)

	// message(4)
	binary.BigEndian.PutUint32(tmp, msgFlags)
	buf = append(buf, tmp...)

	// safi(1) = unicast
	buf = append(buf, 1)

	// family(1)
	buf = append(buf, family)

	// prefixlen(1)
	buf = append(buf, prefixLen)

	// prefix bytes
	byteLen := int((prefixLen + 7) / 8)
	buf = append(buf, prefix[:byteLen]...)

	// nexthops
	if MsgFlag(msgFlags)&MsgNexthop != 0 {
		nhCount := make([]byte, 2)
		binary.BigEndian.PutUint16(nhCount, uint16(len(nexthops)))
		buf = append(buf, nhCount...)

		for _, nh := range nexthops {
			buf = append(buf, encodeTestNexthop(nh)...)
		}
	}

	// distance
	if MsgFlag(msgFlags)&MsgDistance != 0 {
		buf = append(buf, distance)
	}

	// metric
	if MsgFlag(msgFlags)&MsgMetric != 0 {
		binary.BigEndian.PutUint32(tmp, metric)
		buf = append(buf, tmp...)
	}

	return buf
}

func encodeTestNexthop(nh testNexthop) []byte {
	var buf []byte

	// vrfID(4) = 0
	buf = append(buf, 0, 0, 0, 0)

	// type(1)
	buf = append(buf, uint8(nh.nhType))

	// flags(1) = 0
	buf = append(buf, 0)

	nhType := nh.nhType
	switch nhType {
	case NHIPv4:
		nhType = NHIPv4IFIndex
	case NHIPv6:
		nhType = NHIPv6IFIndex
	}

	switch nhType {
	case NHIPv4IFIndex:
		buf = append(buf, nh.gate.To4()...)
	case NHIPv6IFIndex:
		buf = append(buf, nh.gate.To16()...)
	}

	switch nhType {
	case NHIFIndex, NHIPv4IFIndex, NHIPv6IFIndex:
		tmp := make([]byte, 4)
		binary.BigEndian.PutUint32(tmp, nh.ifindex)
		buf = append(buf, tmp...)
	}

	return buf
}
