// Package wgquery reads WireGuard peer status over the wireguard
// generic netlink family, the same channel the wg tool uses.
package wgquery

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
	"github.com/mdlayher/netlink/nlenc"
	"golang.org/x/sys/unix"
)

// Peer is the subset of WireGuard peer state reported in operational data.
type Peer struct {
	PublicKey     string
	Endpoint      *net.UDPAddr
	LastHandshake time.Time
	RxBytes       uint64
	TxBytes       uint64
}

// peersFunc returns the peers of one WireGuard interface.
type peersFunc func(ifname string) ([]Peer, error)

// Query returns peer-status JSON per WireGuard interface found in the
// ip -j link listing, or nil when there is nothing to report.
func Query(links json.RawMessage) map[string]json.RawMessage {
	wgIfaces := findWireguardIfaces(links)
	if len(wgIfaces) == 0 {
		return nil
	}

	client, err := dial()
	if err != nil {
		return nil
	}
	defer client.Close()

	return query(wgIfaces, client.peers, time.Now().UTC())
}

func query(ifaces []string, peersOf peersFunc, now time.Time) map[string]json.RawMessage {
	result := make(map[string]json.RawMessage)

	for _, ifname := range ifaces {
		peers, err := peersOf(ifname)
		if err != nil || len(peers) == 0 {
			continue
		}

		var out []map[string]any
		for _, p := range peers {
			peer := map[string]any{
				"public-key":        p.PublicKey,
				"connection-status": connectionStatus(p.LastHandshake, now),
			}

			if !p.LastHandshake.IsZero() {
				peer["latest-handshake"] = p.LastHandshake.UTC().Format("2006-01-02T15:04:05+00:00")
			}

			if p.Endpoint != nil {
				peer["endpoint-address"] = p.Endpoint.IP.String()
				peer["endpoint-port"] = p.Endpoint.Port
			}

			if p.TxBytes > 0 || p.RxBytes > 0 {
				peer["transfer"] = map[string]any{
					"tx-bytes": strconv.FormatUint(p.TxBytes, 10),
					"rx-bytes": strconv.FormatUint(p.RxBytes, 10),
				}
			}

			out = append(out, peer)
		}

		data, err := json.Marshal(map[string]any{"peer-status": map[string]any{"peer": out}})
		if err != nil {
			continue
		}
		result[ifname] = data
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func findWireguardIfaces(links json.RawMessage) []string {
	var ifaces []map[string]any
	if json.Unmarshal(links, &ifaces) != nil {
		return nil
	}

	var result []string
	for _, iface := range ifaces {
		linkinfo, _ := iface["linkinfo"].(map[string]any)
		if linkinfo == nil {
			continue
		}
		if kind, _ := linkinfo["info_kind"].(string); kind == "wireguard" {
			if name, _ := iface["ifname"].(string); name != "" {
				result = append(result, name)
			}
		}
	}
	return result
}

func connectionStatus(handshake time.Time, now time.Time) string {
	if handshake.IsZero() {
		return "down"
	}
	if now.Sub(handshake) < 180*time.Second {
		return "up"
	}
	return "down"
}

// --- generic netlink ---

type client struct {
	conn   *genetlink.Conn
	family genetlink.Family
}

func dial() (*client, error) {
	conn, err := genetlink.Dial(nil)
	if err != nil {
		return nil, fmt.Errorf("dial genetlink: %w", err)
	}

	family, err := conn.GetFamily(unix.WG_GENL_NAME)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("resolve wireguard family: %w", err)
	}

	return &client{conn: conn, family: family}, nil
}

func (c *client) Close() error {
	return c.conn.Close()
}

// peers dumps one device.  The kernel splits a device with many peers
// over several messages, and a peer with many allowed IPs can repeat
// across them, so peers are merged by public key.
func (c *client) peers(ifname string) ([]Peer, error) {
	req, err := netlink.MarshalAttributes([]netlink.Attribute{{
		Type: unix.WGDEVICE_A_IFNAME,
		Data: nlenc.Bytes(ifname),
	}})
	if err != nil {
		return nil, err
	}

	msgs, err := c.conn.Execute(genetlink.Message{
		Header: genetlink.Header{Command: unix.WG_CMD_GET_DEVICE, Version: unix.WG_GENL_VERSION},
		Data:   req,
	}, c.family.ID, netlink.Request|netlink.Dump)
	if err != nil {
		return nil, fmt.Errorf("wireguard get device %s: %w", ifname, err)
	}

	return parsePeers(msgs)
}

func parsePeers(msgs []genetlink.Message) ([]Peer, error) {
	var peers []Peer
	seen := map[string]bool{}

	for _, m := range msgs {
		ad, err := netlink.NewAttributeDecoder(m.Data)
		if err != nil {
			return nil, err
		}

		for ad.Next() {
			if ad.Type() != unix.WGDEVICE_A_PEERS {
				continue
			}
			ad.Nested(func(nad *netlink.AttributeDecoder) error {
				for nad.Next() {
					nad.Nested(func(pad *netlink.AttributeDecoder) error {
						p := parsePeer(pad)
						if p.PublicKey == "" || seen[p.PublicKey] {
							return nil
						}
						seen[p.PublicKey] = true
						peers = append(peers, p)
						return nil
					})
				}
				return nil
			})
		}

		if err := ad.Err(); err != nil {
			return nil, err
		}
	}

	return peers, nil
}

func parsePeer(ad *netlink.AttributeDecoder) Peer {
	var p Peer
	for ad.Next() {
		switch ad.Type() {
		case unix.WGPEER_A_PUBLIC_KEY:
			p.PublicKey = base64.StdEncoding.EncodeToString(ad.Bytes())
		case unix.WGPEER_A_ENDPOINT:
			p.Endpoint = parseSockaddr(ad.Bytes())
		case unix.WGPEER_A_LAST_HANDSHAKE_TIME:
			p.LastHandshake = parseTimespec(ad.Bytes())
		case unix.WGPEER_A_RX_BYTES:
			p.RxBytes = ad.Uint64()
		case unix.WGPEER_A_TX_BYTES:
			p.TxBytes = ad.Uint64()
		}
	}
	return p
}

// parseSockaddr decodes a raw sockaddr_in or sockaddr_in6: family,
// port in network byte order, then the address (after flowinfo for v6).
func parseSockaddr(b []byte) *net.UDPAddr {
	switch len(b) {
	case unix.SizeofSockaddrInet4:
		return &net.UDPAddr{
			IP:   net.IP(b[4:8]).To4(),
			Port: int(binary.BigEndian.Uint16(b[2:4])),
		}
	case unix.SizeofSockaddrInet6:
		ip := make(net.IP, net.IPv6len)
		copy(ip, b[8:24])
		return &net.UDPAddr{
			IP:   ip,
			Port: int(binary.BigEndian.Uint16(b[2:4])),
		}
	}
	return nil
}

// parseTimespec decodes a __kernel_timespec, 32 or 64 bit fields in
// host byte order.  A zero value means no handshake yet.
func parseTimespec(b []byte) time.Time {
	var sec, nsec int64
	switch len(b) {
	case 8:
		sec = int64(int32(nlenc.Uint32(b[0:4])))
		nsec = int64(int32(nlenc.Uint32(b[4:8])))
	case 16:
		sec = int64(nlenc.Uint64(b[0:8]))
		nsec = int64(nlenc.Uint64(b[8:16]))
	default:
		return time.Time{}
	}
	if sec <= 0 && nsec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, nsec)
}
