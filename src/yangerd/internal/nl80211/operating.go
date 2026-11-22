package nl80211

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

// Operating is the channel a radio is on right now.
type Operating struct {
	Frequency  int // MHz
	Width      int // MHz, 0 when the kernel names none we know
	TxPower    int // dBm
	HasTxPower bool
}

// Operating returns the operating channel of every radio that has
// one, by phy name.  The interfaces of a radio share its channel, so
// the first one that reports a frequency speaks for the radio.
func (c *Client) Operating() (map[string]Operating, error) {
	msgs, err := c.execute(unix.NL80211_CMD_GET_INTERFACE, nil, netlink.Request|netlink.Dump)
	if err != nil {
		return nil, err
	}

	out := make(map[string]Operating)
	for _, msg := range msgs {
		ifname, op, ok := parseOperating(msg.Data)
		if !ok {
			continue
		}
		phy := readPhyName(ifname)
		if phy == "" {
			continue
		}
		if _, seen := out[phy]; !seen {
			out[phy] = op
		}
	}
	return out, nil
}

// parseOperating decodes one NL80211_CMD_NEW_INTERFACE message.  An
// interface that is not on a channel carries no frequency.
func parseOperating(data []byte) (string, Operating, bool) {
	var op Operating
	var ifname string

	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return "", op, false
	}
	for ad.Next() {
		switch ad.Type() {
		case unix.NL80211_ATTR_IFNAME:
			ifname = ad.String()
		case unix.NL80211_ATTR_WIPHY_FREQ:
			op.Frequency = int(ad.Uint32())
		case unix.NL80211_ATTR_CHANNEL_WIDTH:
			op.Width = channelWidthMHz(ad.Uint32())
		case unix.NL80211_ATTR_WIPHY_TX_POWER_LEVEL:
			// mBm, and a driver that does not know says so with a
			// value far outside what a radio can do
			if mbm := ad.Int32(); mbm > -10000 && mbm < 10000 {
				op.TxPower = int(mbm) / 100
				op.HasTxPower = true
			}
		}
	}
	return ifname, op, ifname != "" && op.Frequency > 0
}

func channelWidthMHz(width uint32) int {
	switch width {
	case unix.NL80211_CHAN_WIDTH_20_NOHT, unix.NL80211_CHAN_WIDTH_20:
		return 20
	case unix.NL80211_CHAN_WIDTH_40:
		return 40
	case unix.NL80211_CHAN_WIDTH_80:
		return 80
	case unix.NL80211_CHAN_WIDTH_80P80, unix.NL80211_CHAN_WIDTH_160:
		return 160
	case unix.NL80211_CHAN_WIDTH_320:
		return 320
	}
	return 0
}

func readPhyName(ifname string) string {
	name, err := os.ReadFile(filepath.Join("/sys/class/net", ifname, "phy80211", "name"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(name))
}
