package wgquery

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
	"github.com/mdlayher/netlink/nlenc"
	"golang.org/x/sys/unix"
)

var (
	keyA = make([]byte, 32)
	keyB = append([]byte{1}, make([]byte, 31)...)
)

func sockaddr4(t *testing.T, ip string, port int) []byte {
	t.Helper()
	b := make([]byte, unix.SizeofSockaddrInet4)
	nlenc.PutUint16(b[0:2], unix.AF_INET)
	binary.BigEndian.PutUint16(b[2:4], uint16(port))
	copy(b[4:8], net.ParseIP(ip).To4())
	return b
}

func sockaddr6(t *testing.T, ip string, port int) []byte {
	t.Helper()
	b := make([]byte, unix.SizeofSockaddrInet6)
	nlenc.PutUint16(b[0:2], unix.AF_INET6)
	binary.BigEndian.PutUint16(b[2:4], uint16(port))
	copy(b[8:24], net.ParseIP(ip).To16())
	return b
}

func timespec64(sec int64) []byte {
	b := make([]byte, 16)
	nlenc.PutUint64(b[0:8], uint64(sec))
	return b
}

type peerAttrs struct {
	key       []byte
	endpoint  []byte
	handshake []byte
	rx, tx    uint64
}

func deviceMessage(t *testing.T, peers ...peerAttrs) genetlink.Message {
	t.Helper()
	ae := netlink.NewAttributeEncoder()
	ae.String(unix.WGDEVICE_A_IFNAME, "wg0")
	ae.Nested(unix.WGDEVICE_A_PEERS, func(nae *netlink.AttributeEncoder) error {
		for i, p := range peers {
			nae.Nested(uint16(i), func(pae *netlink.AttributeEncoder) error {
				pae.Bytes(unix.WGPEER_A_PUBLIC_KEY, p.key)
				if p.endpoint != nil {
					pae.Bytes(unix.WGPEER_A_ENDPOINT, p.endpoint)
				}
				if p.handshake != nil {
					pae.Bytes(unix.WGPEER_A_LAST_HANDSHAKE_TIME, p.handshake)
				}
				pae.Uint64(unix.WGPEER_A_RX_BYTES, p.rx)
				pae.Uint64(unix.WGPEER_A_TX_BYTES, p.tx)
				return nil
			})
		}
		return nil
	})
	data, err := ae.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return genetlink.Message{Data: data}
}

func TestParsePeers(t *testing.T) {
	msg := deviceMessage(t,
		peerAttrs{key: keyA, endpoint: sockaddr4(t, "192.0.2.1", 51820), handshake: timespec64(1700000000), rx: 10, tx: 20},
		peerAttrs{key: keyB, endpoint: sockaddr6(t, "2001:db8::1", 4242)},
	)

	peers, err := parsePeers([]genetlink.Message{msg})
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 2 {
		t.Fatalf("want 2 peers, got %d", len(peers))
	}

	a := peers[0]
	if a.PublicKey != base64.StdEncoding.EncodeToString(keyA) {
		t.Errorf("public key = %q", a.PublicKey)
	}
	if a.Endpoint == nil || a.Endpoint.IP.String() != "192.0.2.1" || a.Endpoint.Port != 51820 {
		t.Errorf("endpoint = %v", a.Endpoint)
	}
	if !a.LastHandshake.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("handshake = %v", a.LastHandshake)
	}
	if a.RxBytes != 10 || a.TxBytes != 20 {
		t.Errorf("transfer = rx %d tx %d", a.RxBytes, a.TxBytes)
	}

	b := peers[1]
	if b.Endpoint == nil || b.Endpoint.IP.String() != "2001:db8::1" || b.Endpoint.Port != 4242 {
		t.Errorf("v6 endpoint = %v", b.Endpoint)
	}
	if !b.LastHandshake.IsZero() {
		t.Errorf("want no handshake, got %v", b.LastHandshake)
	}
}

func TestPeersSplitAcrossMessagesAreMerged(t *testing.T) {
	msgs := []genetlink.Message{
		deviceMessage(t, peerAttrs{key: keyA, rx: 1}),
		deviceMessage(t, peerAttrs{key: keyA, rx: 1}, peerAttrs{key: keyB}),
	}
	peers, err := parsePeers(msgs)
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 2 {
		t.Fatalf("want 2 distinct peers, got %d", len(peers))
	}
}

func TestQueryJSON(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	peersOf := func(ifname string) ([]Peer, error) {
		switch ifname {
		case "wg0":
			return []Peer{
				{PublicKey: "AAAA", Endpoint: &net.UDPAddr{IP: net.ParseIP("192.0.2.1"), Port: 51820},
					LastHandshake: now.Add(-time.Minute), RxBytes: 10, TxBytes: 20},
				{PublicKey: "BBBB", LastHandshake: now.Add(-time.Hour)},
				{PublicKey: "CCCC"},
			}, nil
		case "wg1":
			return nil, errors.New("no such device")
		}
		return nil, nil
	}

	out := query([]string{"wg0", "wg1", "wg2"}, peersOf, now)
	if len(out) != 1 {
		t.Fatalf("want wg0 only, got %v", out)
	}

	var doc map[string]map[string][]map[string]any
	if err := json.Unmarshal(out["wg0"], &doc); err != nil {
		t.Fatal(err)
	}
	peers := doc["peer-status"]["peer"]
	if len(peers) != 3 {
		t.Fatalf("want 3 peers, got %d", len(peers))
	}
	if peers[0]["connection-status"] != "up" || peers[0]["endpoint-port"] != float64(51820) ||
		peers[0]["latest-handshake"] != "2026-10-05T11:59:00+00:00" {
		t.Errorf("peer 0 = %v", peers[0])
	}
	if tr := peers[0]["transfer"].(map[string]any); tr["tx-bytes"] != "20" || tr["rx-bytes"] != "10" {
		t.Errorf("transfer = %v", tr)
	}
	if peers[1]["connection-status"] != "down" {
		t.Errorf("stale handshake should be down: %v", peers[1])
	}
	if _, ok := peers[2]["latest-handshake"]; ok || peers[2]["connection-status"] != "down" {
		t.Errorf("never-connected peer = %v", peers[2])
	}
}

func TestQueryNothingToReport(t *testing.T) {
	if out := query([]string{"wg0"}, func(string) ([]Peer, error) { return nil, nil }, time.Now()); out != nil {
		t.Errorf("want nil, got %v", out)
	}
	if ifaces := findWireguardIfaces(json.RawMessage(`[{"ifname":"e0","linkinfo":{"info_kind":"bridge"}},{"ifname":"wg0","linkinfo":{"info_kind":"wireguard"}}]`)); len(ifaces) != 1 || ifaces[0] != "wg0" {
		t.Errorf("ifaces = %v", ifaces)
	}
}
