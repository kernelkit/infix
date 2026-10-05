package iwmonitor

import (
	"net"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kernelkit/infix/src/yangerd/internal/nl80211"
	"github.com/kernelkit/infix/src/yangerd/internal/wpactrl"
)

// meshState is what the kernel knows about a mesh interface, read on
// every refresh.  The default implementation asks nl80211, tests
// substitute their own.
type meshState struct {
	iftype     string
	forwarding *bool
	peers      []nl80211.Station
}

func kernelMeshState(ifname string) meshState {
	var ms meshState

	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		return ms
	}
	client, err := nl80211.Dial()
	if err != nil {
		return ms
	}
	defer client.Close()

	if iftype, err := client.InterfaceType(iface.Index); err == nil {
		ms.iftype = iftype
	}
	if ms.iftype != "mesh_point" {
		return ms
	}
	if fwd, err := client.MeshForwarding(iface.Index); err == nil {
		ms.forwarding = &fwd
	}
	if peers, err := client.Stations(iface.Index); err == nil {
		ms.peers = peers
	}
	return ms
}

// meshEvent tells whether a wpa_supplicant event changes mesh state:
// a peer link came or went, or the mesh itself was joined or left.
func meshEvent(ev wpactrl.Event) bool {
	return strings.HasPrefix(ev.Name, "MESH-PEER-") || strings.HasPrefix(ev.Name, "MESH-GROUP-")
}

// buildMeshData assembles the mesh-point container.  The mesh id comes
// from wpa_supplicant, which is what joined the mesh, and is absent
// until it has.  Forwarding and peers come from the kernel.
func (m *IWMonitor) buildMeshData(iface string, si wpactrl.SocketInfo, status map[string]string, ms meshState) map[string]any {
	mesh := make(map[string]any)

	if id := decodeWPASSID(resolveSSID(iface, si, status)); id != "" {
		mesh["mesh-id"] = id
	}
	if ms.forwarding != nil {
		mesh["forwarding"] = *ms.forwarding
	}
	if len(ms.peers) > 0 {
		mesh["peers"] = map[string]any{"peer": formatPeers(ms.peers)}
	}

	return mesh
}

type peerEntry struct {
	MAC           string `json:"mac-address"`
	Signal        *int16 `json:"signal-strength,omitempty"`
	ConnectedTime uint32 `json:"connected-time"`
	RxPackets     string `json:"rx-packets"`
	TxPackets     string `json:"tx-packets"`
	RxBytes       string `json:"rx-bytes"`
	TxBytes       string `json:"tx-bytes"`
	RxSpeed       uint32 `json:"rx-speed,omitempty"`
	TxSpeed       uint32 `json:"tx-speed,omitempty"`
}

func formatPeers(stas []nl80211.Station) []peerEntry {
	out := make([]peerEntry, 0, len(stas))
	for _, st := range stas {
		p := peerEntry{
			MAC:           st.MAC,
			ConnectedTime: st.ConnectedTime,
			RxPackets:     strconv.FormatUint(st.RxPackets, 10),
			TxPackets:     strconv.FormatUint(st.TxPackets, 10),
			RxBytes:       strconv.FormatUint(st.RxBytes, 10),
			TxBytes:       strconv.FormatUint(st.TxBytes, 10),
			RxSpeed:       st.RxBitrate,
			TxSpeed:       st.TxBitrate,
		}
		if st.HasSignal {
			sig := int16(st.Signal)
			p.Signal = &sig
		}
		out = append(out, p)
	}
	return out
}

// decodeWPASSID undoes wpa_supplicant's printf_encode(): bytes outside
// printable ASCII arrive as \xHH, plus \\ \" \e \n \r \t.  The result is
// taken as UTF-8 when it is valid, otherwise the escaped form is kept.
// Control characters are dropped either way, a rogue peer must not get
// to write escape sequences to a terminal.
func decodeWPASSID(s string) string {
	out := s
	if raw := unescapePrintf(s); utf8.Valid(raw) {
		out = string(raw)
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, out)
}

func unescapePrintf(s string) []byte {
	raw := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			raw = append(raw, c)
			continue
		}
		i++
		switch s[i] {
		case 'x':
			if i+2 < len(s) {
				if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
					raw = append(raw, byte(v))
					i += 2
					continue
				}
			}
			raw = append(raw, '\\', 'x')
		case '\\':
			raw = append(raw, '\\')
		case '"':
			raw = append(raw, '"')
		case 'e':
			raw = append(raw, 0x1b)
		case 'n':
			raw = append(raw, '\n')
		case 'r':
			raw = append(raw, '\r')
		case 't':
			raw = append(raw, '\t')
		default:
			raw = append(raw, '\\', s[i])
		}
	}
	return raw
}
