// Package zapi implements a minimal ZAPI v6 client for FRR 10.5.
//
// It speaks only the subset of the Zebra wire protocol needed by
// yangerd: Hello, RouterIDAdd, RedistributeAdd, and decoding of
// RedistributeRouteAdd/Del messages.
package zapi

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"syscall"
)

// Wire constants for ZAPI v6.
const (
	HeaderSize    = 10
	HeaderMarker  = 0xFE
	HeaderVersion = 6

	DefaultVrf uint32 = 0
)

// Command IDs for FRR 10.5 ZAPI v6 (from lib/zclient.h).
type Command uint16

const (
	CmdInterfaceAdd          Command = 0
	CmdInterfaceDelete       Command = 1
	CmdInterfaceAddrAdd      Command = 2
	CmdInterfaceAddrDelete   Command = 3
	CmdInterfaceUp           Command = 4
	CmdInterfaceDown         Command = 5
	CmdInterfaceSetMaster    Command = 6
	CmdInterfaceSetARP       Command = 7 // new in FRR 10.x
	CmdInterfaceSetProtodown Command = 8
	CmdRouteAdd              Command = 9
	CmdRouteDelete           Command = 10
	CmdRouteNotifyOwner      Command = 11
	CmdRedistributeAdd       Command = 12
	CmdRedistributeDelete    Command = 13
	CmdRedistDefaultAdd      Command = 14
	CmdRedistDefaultDelete   Command = 15
	CmdRouterIDAdd           Command = 16
	CmdRouterIDDelete        Command = 17
	CmdRouterIDUpdate        Command = 18
	CmdHello                 Command = 19
	CmdCapabilities          Command = 20
	CmdNexthopRegister       Command = 21
	CmdNexthopUnregister     Command = 22
	CmdNexthopUpdate         Command = 23

	CmdRedistRouteAdd Command = 31
	CmdRedistRouteDel Command = 32
)

// RouteType identifies the source protocol of a route.
type RouteType uint8

const (
	RouteSystem  RouteType = 0
	RouteKernel  RouteType = 1
	RouteConnect RouteType = 2
	RouteStatic  RouteType = 3
	RouteRIP     RouteType = 4
	RouteRIPNG   RouteType = 5
	RouteOSPF    RouteType = 6
	RouteOSPF6   RouteType = 7
	RouteISIS    RouteType = 8
	RouteBGP     RouteType = 9
)

// AFI values.
const (
	AFIIPv4 uint8 = 1
	AFIIPv6 uint8 = 2
)

// Message flags (from struct zapi_route.message).
type MsgFlag uint32

const (
	MsgNexthop  MsgFlag = 0x01
	MsgDistance MsgFlag = 0x02
	MsgMetric   MsgFlag = 0x04
	MsgTag      MsgFlag = 0x08
	MsgMTU      MsgFlag = 0x10
	MsgSrcPfx   MsgFlag = 0x20
	MsgBackupNH MsgFlag = 0x40
	MsgNHG      MsgFlag = 0x80
	MsgTableID  MsgFlag = 0x100
	MsgSRTE     MsgFlag = 0x200
	MsgOpaque   MsgFlag = 0x400
)

// Nexthop type values (from lib/nexthop.h).
type NHType uint8

const (
	NHIFIndex     NHType = 1
	NHIPv4        NHType = 2
	NHIPv4IFIndex NHType = 3
	NHIPv6        NHType = 4
	NHIPv6IFIndex NHType = 5
	NHBlackhole   NHType = 6
)

// Nexthop flags (from lib/zclient.h: ZAPI_NEXTHOP_FLAG_*).
const (
	nhFlagLabel     uint8 = 0x02
	nhFlagWeight    uint8 = 0x04
	nhFlagHasBackup uint8 = 0x08
	nhFlagSeg6      uint8 = 0x10
	nhFlagSeg6Local uint8 = 0x20
)

// Header is a ZAPI v6 message header.
type Header struct {
	Length  uint16
	Marker  uint8
	Version uint8
	VrfID   uint32
	Command Command
}

// Nexthop holds one entry from the nexthop list in a route message.
type Nexthop struct {
	Type    NHType
	Gate    net.IP
	Ifindex uint32
}

// Route is the decoded content of a RedistributeRouteAdd/Del message.
type Route struct {
	Type     RouteType
	Flags    uint32
	Message  MsgFlag
	Prefix   net.IPNet
	Nexthops []Nexthop
	Distance uint8
	Metric   uint32
	Tag      uint32
	MTU      uint32
}

// Message is a decoded ZAPI message received from zebra.
type Message struct {
	Header Header
	Route  *Route // non-nil only for route messages
}

// EncodeHeader serializes a ZAPI v6 header.
func EncodeHeader(length uint16, vrfID uint32, cmd Command) []byte {
	buf := make([]byte, HeaderSize)
	binary.BigEndian.PutUint16(buf[0:2], length)
	buf[2] = HeaderMarker
	buf[3] = HeaderVersion
	binary.LittleEndian.PutUint32(buf[4:8], vrfID)
	binary.BigEndian.PutUint16(buf[8:10], uint16(cmd))
	return buf
}

// DecodeHeader parses a ZAPI v6 header from exactly HeaderSize bytes.
func DecodeHeader(data []byte) (Header, error) {
	if len(data) < HeaderSize {
		return Header{}, fmt.Errorf("header too short: %d bytes", len(data))
	}
	h := Header{
		Length:  binary.BigEndian.Uint16(data[0:2]),
		Marker:  data[2],
		Version: data[3],
		VrfID:   binary.LittleEndian.Uint32(data[4:8]),
		Command: Command(binary.BigEndian.Uint16(data[8:10])),
	}
	if h.Marker != HeaderMarker {
		return Header{}, fmt.Errorf("bad marker: 0x%02x", h.Marker)
	}
	if h.Version != HeaderVersion {
		return Header{}, fmt.Errorf("unsupported version: %d", h.Version)
	}
	return h, nil
}

// EncodeHello builds a Hello message body.
// Fields: redistDefault(1), instance(2), sessionID(4), receiveNotify(1), synchronous(1), flags(4), ...
// We send zeros for everything (redistDefault=0 means "no default route type").
func EncodeHello() []byte {
	body := make([]byte, 12) // redistDefault + instance + sessionID + receiveNotify + synchronous + flags
	return body
}

// EncodeRouterIDAdd builds a RouterIDAdd message body.
// Body is just the AFI value (1 byte).
func EncodeRouterIDAdd(afi uint8) []byte {
	return []byte{afi}
}

// EncodeRedistributeAdd builds a RedistributeAdd body.
// Body: afi(1), routeType(1), instance(2).
func EncodeRedistributeAdd(afi uint8, rt RouteType) []byte {
	buf := make([]byte, 4)
	buf[0] = afi
	buf[1] = uint8(rt)
	// instance = 0 (already zeroed)
	return buf
}

// BuildMessage constructs a complete wire message from command and body.
func BuildMessage(cmd Command, vrfID uint32, body []byte) []byte {
	length := uint16(HeaderSize + len(body))
	hdr := EncodeHeader(length, vrfID, cmd)
	return append(hdr, body...)
}

// ReadMessage reads one complete ZAPI message from the connection.
// It returns the header and the raw body bytes.
func ReadMessage(r io.Reader) (Header, []byte, error) {
	hdrBuf := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, hdrBuf); err != nil {
		return Header{}, nil, fmt.Errorf("read header: %w", err)
	}

	hdr, err := DecodeHeader(hdrBuf)
	if err != nil {
		return Header{}, nil, err
	}

	bodyLen := int(hdr.Length) - HeaderSize
	if bodyLen < 0 {
		return Header{}, nil, fmt.Errorf("invalid message length: %d", hdr.Length)
	}
	if bodyLen == 0 {
		return hdr, nil, nil
	}

	body := make([]byte, bodyLen)
	if _, err := io.ReadFull(r, body); err != nil {
		return Header{}, nil, fmt.Errorf("read body: %w", err)
	}

	return hdr, body, nil
}

// DecodeRoute parses an IPRouteBody from the given body bytes.
// This handles the FRR 10.5 / ZAPI v6 (frr >= 7.5) format:
//
//	type(1) instance(2) flags(4) message(4) safi(1) family(1) prefixlen(1) prefix(var) ...
func DecodeRoute(body []byte) (*Route, error) {
	if len(body) < 10 {
		return nil, fmt.Errorf("route body too short: %d bytes", len(body))
	}

	r := &Route{}
	pos := 0

	// type(1)
	r.Type = RouteType(body[pos])
	pos++

	// instance(2)
	pos += 2

	// flags(4)
	r.Flags = binary.BigEndian.Uint32(body[pos : pos+4])
	pos += 4

	// message(4) — FRR >= 7.5 uses 4-byte message field
	r.Message = MsgFlag(binary.BigEndian.Uint32(body[pos : pos+4]))
	pos += 4

	// safi(1)
	pos++

	// family(1)
	if pos >= len(body) {
		return nil, fmt.Errorf("truncated at family")
	}
	family := body[pos]
	pos++

	addrLen, err := addressByteLen(family)
	if err != nil {
		return nil, err
	}

	// prefixlen(1)
	if pos >= len(body) {
		return nil, fmt.Errorf("truncated at prefixlen")
	}
	prefixLen := body[pos]
	pos++

	// prefix (ceil(prefixLen/8) bytes)
	byteLen := int((prefixLen + 7) / 8)
	if pos+byteLen > len(body) {
		return nil, fmt.Errorf("truncated at prefix data")
	}
	ipBuf := make([]byte, addrLen)
	copy(ipBuf, body[pos:pos+byteLen])
	pos += byteLen

	var ip net.IP
	if family == syscall.AF_INET {
		ip = net.IP(ipBuf).To4()
	} else {
		ip = net.IP(ipBuf).To16()
	}
	mask := net.CIDRMask(int(prefixLen), addrLen*8)
	r.Prefix = net.IPNet{IP: ip, Mask: mask}

	// source prefix (if MsgSrcPfx set)
	if r.Message&MsgSrcPfx != 0 {
		if pos >= len(body) {
			return nil, fmt.Errorf("truncated at src prefix")
		}
		srcPfxLen := body[pos]
		pos++
		srcByteLen := int((srcPfxLen + 7) / 8)
		if pos+srcByteLen > len(body) {
			return nil, fmt.Errorf("truncated at src prefix data")
		}
		pos += srcByteLen
	}

	// NHG (if MsgNHG set, frr >= 8)
	if r.Message&MsgNHG != 0 {
		if pos+4 > len(body) {
			return nil, fmt.Errorf("truncated at nhg")
		}
		pos += 4 // skip nhgid
	}

	// Nexthops
	if r.Message&MsgNexthop != 0 {
		if pos+2 > len(body) {
			return nil, fmt.Errorf("truncated at nexthop count")
		}
		numNH := binary.BigEndian.Uint16(body[pos : pos+2])
		pos += 2
		r.Nexthops = make([]Nexthop, 0, numNH)

		for i := uint16(0); i < numNH; i++ {
			nh, n, err := decodeNexthop(body[pos:], family)
			if err != nil {
				return nil, fmt.Errorf("nexthop %d: %w", i, err)
			}
			r.Nexthops = append(r.Nexthops, nh)
			pos += n
		}
	}

	// Backup nexthops (skip)
	if r.Message&MsgBackupNH != 0 {
		if pos+2 > len(body) {
			return nil, fmt.Errorf("truncated at backup nexthop count")
		}
		numBackup := binary.BigEndian.Uint16(body[pos : pos+2])
		pos += 2
		for i := uint16(0); i < numBackup; i++ {
			_, n, err := decodeNexthop(body[pos:], family)
			if err != nil {
				return nil, fmt.Errorf("backup nexthop %d: %w", i, err)
			}
			pos += n
		}
	}

	// Distance
	if r.Message&MsgDistance != 0 {
		if pos >= len(body) {
			return nil, fmt.Errorf("truncated at distance")
		}
		r.Distance = body[pos]
		pos++
	}

	// Metric
	if r.Message&MsgMetric != 0 {
		if pos+4 > len(body) {
			return nil, fmt.Errorf("truncated at metric")
		}
		r.Metric = binary.BigEndian.Uint32(body[pos : pos+4])
		pos += 4
	}

	// Tag
	if r.Message&MsgTag != 0 {
		if pos+4 > len(body) {
			return nil, fmt.Errorf("truncated at tag")
		}
		r.Tag = binary.BigEndian.Uint32(body[pos : pos+4])
		pos += 4
	}

	// MTU
	if r.Message&MsgMTU != 0 {
		if pos+4 > len(body) {
			return nil, fmt.Errorf("truncated at mtu")
		}
		r.MTU = binary.BigEndian.Uint32(body[pos : pos+4])
		pos += 4
	}

	// Remaining fields (tableID, SRTE, opaque) are not needed.
	return r, nil
}

// decodeNexthop decodes one nexthop from the body.
// FRR >= 7.3 / ZAPI v6 format: vrfID(4) type(1) flags(1) [gate] [ifindex] ...
func decodeNexthop(data []byte, family uint8) (Nexthop, int, error) {
	nh := Nexthop{}
	pos := 0

	// vrfID(4)
	if pos+4 > len(data) {
		return nh, 0, fmt.Errorf("truncated at vrf_id")
	}
	pos += 4 // skip vrfID

	// type(1)
	if pos >= len(data) {
		return nh, 0, fmt.Errorf("truncated at nh type")
	}
	nh.Type = NHType(data[pos])
	pos++

	// flags(1) — FRR >= 7.3
	if pos >= len(data) {
		return nh, 0, fmt.Errorf("truncated at nh flags")
	}
	flags := data[pos]
	pos++

	// For FRR >= 7.3, IPv4 and IPv6 are treated as IPv4IFIndex and IPv6IFIndex
	// respectively (nexthopProcessIPToIPIFindex). We do the same mapping.
	nhType := nh.Type
	switch nhType {
	case NHIPv4:
		nhType = NHIPv4IFIndex
	case NHIPv6:
		nhType = NHIPv6IFIndex
	}

	// Decode gate address
	switch nhType {
	case NHIPv4IFIndex:
		if pos+4 > len(data) {
			return nh, 0, fmt.Errorf("truncated at ipv4 gate")
		}
		nh.Gate = net.IP(data[pos : pos+4]).To4()
		pos += 4
	case NHIPv6IFIndex:
		if pos+16 > len(data) {
			return nh, 0, fmt.Errorf("truncated at ipv6 gate")
		}
		nh.Gate = net.IP(data[pos : pos+16]).To16()
		pos += 16
	}

	// Decode ifindex
	switch nhType {
	case NHIFIndex, NHIPv4IFIndex, NHIPv6IFIndex:
		if pos+4 > len(data) {
			return nh, 0, fmt.Errorf("truncated at ifindex")
		}
		nh.Ifindex = binary.BigEndian.Uint32(data[pos : pos+4])
		pos += 4
	}

	// Blackhole type
	if nhType == NHBlackhole {
		if pos >= len(data) {
			return nh, 0, fmt.Errorf("truncated at blackhole type")
		}
		pos++ // skip blackhole type byte
	}

	// Labels (if flag set)
	if flags&nhFlagLabel != 0 {
		if pos >= len(data) {
			return nh, 0, fmt.Errorf("truncated at label count")
		}
		labelNum := int(data[pos])
		pos++
		if labelNum > 16 {
			labelNum = 16
		}
		pos += labelNum * 4 // skip label data
	}

	// Weight (if flag set)
	if flags&nhFlagWeight != 0 {
		if pos+4 > len(data) {
			return nh, 0, fmt.Errorf("truncated at weight")
		}
		pos += 4
	}

	// SRTE color — present when message has MsgSRTE flag.
	// We can't check message flags here; instead rely on the frr >=7.5
	// behavior: SRTE color is only present if the SRTE message flag was
	// set on the route. The caller handles this by not passing SRTE routes
	// to us (we only subscribe to simple route types). For safety, we skip
	// nothing here — SRTE is handled at the route level if needed.

	// Backup nexthop indices (if flag set)
	if flags&nhFlagHasBackup != 0 {
		if pos >= len(data) {
			return nh, 0, fmt.Errorf("truncated at backup count")
		}
		backupNum := int(data[pos])
		pos++
		pos += backupNum // skip backup indices
	}

	// SEG6 (if flag set)
	if flags&nhFlagSeg6 != 0 {
		// seg6local_action(4) + seg6local_context(4+16+4 = 24)
		pos += 28
	}

	// SEG6 local (if flag set)
	if flags&nhFlagSeg6Local != 0 {
		pos += 16 // struct in6_addr
	}

	return nh, pos, nil
}

func addressByteLen(family uint8) (int, error) {
	switch family {
	case syscall.AF_INET:
		return 4, nil
	case syscall.AF_INET6:
		return 16, nil
	default:
		return 0, fmt.Errorf("unsupported address family: %d", family)
	}
}
