// Package zapi implements a minimal ZAPI v6 client for FRR 10.5.
//
// It speaks only the subset of the Zebra wire protocol needed by
// yangerd: Hello, RouterIDAdd, RedistributeAdd, and reading message
// headers, which is all a change trigger needs.
package zapi

import (
	"encoding/binary"
	"fmt"
	"io"
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
	RouteLocal   RouteType = 3
	RouteStatic  RouteType = 4
	RouteRIP     RouteType = 5
	RouteRIPNG   RouteType = 6
	RouteOSPF    RouteType = 7
	RouteOSPF6   RouteType = 8
	RouteISIS    RouteType = 9
	RouteBGP     RouteType = 10
)

// AFI values.
const (
	AFIIPv4 uint8 = 1
	AFIIPv6 uint8 = 2
)

// Header is a ZAPI v6 message header.
type Header struct {
	Length  uint16
	Marker  uint8
	Version uint8
	VrfID   uint32
	Command Command
}

// EncodeHeader serializes a ZAPI v6 header.
func EncodeHeader(length uint16, vrfID uint32, cmd Command) []byte {
	buf := make([]byte, HeaderSize)
	binary.BigEndian.PutUint16(buf[0:2], length)
	buf[2] = HeaderMarker
	buf[3] = HeaderVersion
	binary.BigEndian.PutUint32(buf[4:8], vrfID)
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
		VrfID:   binary.BigEndian.Uint32(data[4:8]),
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
// Fields: redistDefault(1), instance(2), sessionID(4), synchronous(1) = 8 bytes.
// We send zeros for everything (redistDefault=0 means ZEBRA_ROUTE_SYSTEM).
func EncodeHello() []byte {
	return make([]byte, 8)
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
