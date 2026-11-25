package nl80211

import (
	"testing"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

func TestParseOperating(t *testing.T) {
	msg := encode(t, func(ae *netlink.AttributeEncoder) {
		ae.String(unix.NL80211_ATTR_IFNAME, "wifi0")
		ae.Uint32(unix.NL80211_ATTR_WIPHY_FREQ, 5180)
		ae.Uint32(unix.NL80211_ATTR_CHANNEL_WIDTH, unix.NL80211_CHAN_WIDTH_80)
		ae.Int32(unix.NL80211_ATTR_WIPHY_TX_POWER_LEVEL, 2300)
	})

	ifname, op, ok := parseOperating(msg)
	if !ok || ifname != "wifi0" {
		t.Fatalf("parseOperating = %q, %v", ifname, ok)
	}
	if op.Frequency != 5180 || op.Width != 80 {
		t.Errorf("channel = %d MHz, %d MHz wide", op.Frequency, op.Width)
	}
	if !op.HasTxPower || op.TxPower != 23 {
		t.Errorf("TxPower = %d (has %v)", op.TxPower, op.HasTxPower)
	}
}

func TestParseOperatingOffChannel(t *testing.T) {
	msg := encode(t, func(ae *netlink.AttributeEncoder) {
		ae.String(unix.NL80211_ATTR_IFNAME, "wifi0")
	})

	if _, _, ok := parseOperating(msg); ok {
		t.Error("an interface without a frequency is not on a channel")
	}
}

func TestParseOperatingUnknownTxPower(t *testing.T) {
	msg := encode(t, func(ae *netlink.AttributeEncoder) {
		ae.String(unix.NL80211_ATTR_IFNAME, "wifi0")
		ae.Uint32(unix.NL80211_ATTR_WIPHY_FREQ, 2412)
		ae.Int32(unix.NL80211_ATTR_WIPHY_TX_POWER_LEVEL, -2147483648)
	})

	_, op, ok := parseOperating(msg)
	if !ok || op.HasTxPower {
		t.Errorf("TxPower = %d (has %v), want none", op.TxPower, op.HasTxPower)
	}
}

func TestParseCountry(t *testing.T) {
	msg := encode(t, func(ae *netlink.AttributeEncoder) {
		ae.String(unix.NL80211_ATTR_REG_ALPHA2, "SE")
	})

	if got := parseCountry(msg); got != "SE" {
		t.Errorf("parseCountry = %q, want SE", got)
	}
}
