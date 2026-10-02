package nl80211

import (
	"testing"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

func encode(t *testing.T, fn func(ae *netlink.AttributeEncoder)) []byte {
	t.Helper()
	ae := netlink.NewAttributeEncoder()
	fn(ae)
	b, err := ae.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseStation(t *testing.T) {
	msg := encode(t, func(ae *netlink.AttributeEncoder) {
		ae.Bytes(unix.NL80211_ATTR_MAC, []byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x02})
		ae.Nested(unix.NL80211_ATTR_STA_INFO, func(sta *netlink.AttributeEncoder) error {
			sta.Int8(unix.NL80211_STA_INFO_SIGNAL, -47)
			sta.Uint32(unix.NL80211_STA_INFO_CONNECTED_TIME, 321)
			sta.Uint32(unix.NL80211_STA_INFO_RX_BYTES, 1)
			sta.Uint64(unix.NL80211_STA_INFO_RX_BYTES64, 5000000000)
			sta.Uint32(unix.NL80211_STA_INFO_TX_BYTES, 2)
			sta.Uint64(unix.NL80211_STA_INFO_TX_BYTES64, 6000000000)
			sta.Uint32(unix.NL80211_STA_INFO_RX_PACKETS, 10)
			sta.Uint32(unix.NL80211_STA_INFO_TX_PACKETS, 20)
			sta.Nested(unix.NL80211_STA_INFO_TX_BITRATE, func(r *netlink.AttributeEncoder) error {
				r.Uint16(unix.NL80211_RATE_INFO_BITRATE, 8667)
				r.Uint32(unix.NL80211_RATE_INFO_BITRATE32, 8667)
				return nil
			})
			sta.Nested(unix.NL80211_STA_INFO_RX_BITRATE, func(r *netlink.AttributeEncoder) error {
				r.Uint16(unix.NL80211_RATE_INFO_BITRATE, 650)
				return nil
			})
			return nil
		})
	})

	st, ok := parseStation(msg)
	if !ok {
		t.Fatal("station not parsed")
	}
	if st.MAC != "02:00:00:00:00:02" {
		t.Errorf("MAC = %q", st.MAC)
	}
	if !st.HasSignal || st.Signal != -47 {
		t.Errorf("Signal = %d (has %v)", st.Signal, st.HasSignal)
	}
	if st.ConnectedTime != 321 {
		t.Errorf("ConnectedTime = %d", st.ConnectedTime)
	}
	if st.RxBytes != 5000000000 || st.TxBytes != 6000000000 {
		t.Errorf("bytes = %d/%d, 64-bit counters must win", st.RxBytes, st.TxBytes)
	}
	if st.RxPackets != 10 || st.TxPackets != 20 {
		t.Errorf("packets = %d/%d", st.RxPackets, st.TxPackets)
	}
	if st.TxBitrate != 8667 || st.RxBitrate != 650 {
		t.Errorf("bitrate = %d/%d", st.TxBitrate, st.RxBitrate)
	}
}

func TestParseStationWithoutMAC(t *testing.T) {
	msg := encode(t, func(ae *netlink.AttributeEncoder) {
		ae.Uint32(unix.NL80211_ATTR_IFINDEX, 3)
	})
	if _, ok := parseStation(msg); ok {
		t.Error("a message without a MAC is not a station")
	}
}

func TestParseMeshForwarding(t *testing.T) {
	for _, want := range []bool{true, false} {
		msg := encode(t, func(ae *netlink.AttributeEncoder) {
			ae.Uint32(unix.NL80211_ATTR_IFINDEX, 3)
			ae.Nested(unix.NL80211_ATTR_MESH_CONFIG, func(mc *netlink.AttributeEncoder) error {
				mc.Uint16(unix.NL80211_MESHCONF_RETRY_TIMEOUT, 100)
				if want {
					mc.Uint8(unix.NL80211_MESHCONF_FORWARDING, 1)
				} else {
					mc.Uint8(unix.NL80211_MESHCONF_FORWARDING, 0)
				}
				return nil
			})
		})
		got, ok := parseMeshForwarding(msg)
		if !ok || got != want {
			t.Errorf("forwarding = %v (ok %v), want %v", got, ok, want)
		}
	}

	msg := encode(t, func(ae *netlink.AttributeEncoder) {
		ae.Uint32(unix.NL80211_ATTR_IFINDEX, 3)
	})
	if _, ok := parseMeshForwarding(msg); ok {
		t.Error("no mesh config must not parse")
	}
}
