package nl80211

import (
	"fmt"
	"net"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

// Station is one entry of the kernel station table for an interface:
// an associated client in AP mode, a peer in mesh mode.
type Station struct {
	MAC           string
	Signal        int8
	HasSignal     bool
	ConnectedTime uint32
	RxBytes       uint64
	TxBytes       uint64
	RxPackets     uint64
	TxPackets     uint64
	RxBitrate     uint32 // 100 kbps
	TxBitrate     uint32 // 100 kbps
}

// Stations dumps the station table of ifindex.
func (c *Client) Stations(ifindex int) ([]Station, error) {
	ae := netlink.NewAttributeEncoder()
	ae.Uint32(unix.NL80211_ATTR_IFINDEX, uint32(ifindex))
	req, err := ae.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode get_station request: %w", err)
	}

	msgs, err := c.execute(unix.NL80211_CMD_GET_STATION, req, netlink.Request|netlink.Dump)
	if err != nil {
		return nil, err
	}

	out := make([]Station, 0, len(msgs))
	for _, msg := range msgs {
		if st, ok := parseStation(msg.Data); ok {
			out = append(out, st)
		}
	}
	return out, nil
}

// parseStation decodes one NL80211_CMD_NEW_STATION message.
func parseStation(data []byte) (Station, bool) {
	var st Station

	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return st, false
	}
	for ad.Next() {
		switch ad.Type() {
		case unix.NL80211_ATTR_MAC:
			st.MAC = net.HardwareAddr(ad.Bytes()).String()
		case unix.NL80211_ATTR_STA_INFO:
			parseStationInfo(&st, ad.Bytes())
		}
	}
	return st, st.MAC != ""
}

func parseStationInfo(st *Station, data []byte) {
	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return
	}
	for ad.Next() {
		switch ad.Type() {
		case unix.NL80211_STA_INFO_SIGNAL:
			st.Signal = ad.Int8()
			st.HasSignal = true
		case unix.NL80211_STA_INFO_CONNECTED_TIME:
			st.ConnectedTime = ad.Uint32()
		case unix.NL80211_STA_INFO_RX_BYTES:
			if st.RxBytes == 0 {
				st.RxBytes = uint64(ad.Uint32())
			}
		case unix.NL80211_STA_INFO_TX_BYTES:
			if st.TxBytes == 0 {
				st.TxBytes = uint64(ad.Uint32())
			}
		case unix.NL80211_STA_INFO_RX_BYTES64:
			st.RxBytes = ad.Uint64()
		case unix.NL80211_STA_INFO_TX_BYTES64:
			st.TxBytes = ad.Uint64()
		case unix.NL80211_STA_INFO_RX_PACKETS:
			st.RxPackets = uint64(ad.Uint32())
		case unix.NL80211_STA_INFO_TX_PACKETS:
			st.TxPackets = uint64(ad.Uint32())
		case unix.NL80211_STA_INFO_RX_BITRATE:
			st.RxBitrate = parseBitrate(ad.Bytes())
		case unix.NL80211_STA_INFO_TX_BITRATE:
			st.TxBitrate = parseBitrate(ad.Bytes())
		}
	}
}

// parseBitrate returns the rate in 100 kbps from a rate_info nest.  The
// 32-bit attribute is preferred, the 16-bit one saturates at 6.5 Gbps.
func parseBitrate(data []byte) uint32 {
	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return 0
	}
	var rate16 uint16
	for ad.Next() {
		switch ad.Type() {
		case unix.NL80211_RATE_INFO_BITRATE32:
			return ad.Uint32()
		case unix.NL80211_RATE_INFO_BITRATE:
			rate16 = ad.Uint16()
		}
	}
	return uint32(rate16)
}

// InterfaceType returns the iftype of ifindex as iw names it, e.g.
// "station", "AP" or "mesh_point".
func (c *Client) InterfaceType(ifindex int) (string, error) {
	ae := netlink.NewAttributeEncoder()
	ae.Uint32(unix.NL80211_ATTR_IFINDEX, uint32(ifindex))
	req, err := ae.Encode()
	if err != nil {
		return "", fmt.Errorf("encode get_interface request: %w", err)
	}

	msgs, err := c.execute(unix.NL80211_CMD_GET_INTERFACE, req, netlink.Request)
	if err != nil {
		return "", err
	}

	for _, msg := range msgs {
		if name, ok := parseInterfaceType(msg.Data); ok {
			return name, nil
		}
	}
	return "", fmt.Errorf("no iftype for ifindex %d", ifindex)
}

func parseInterfaceType(data []byte) (string, bool) {
	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return "", false
	}
	for ad.Next() {
		if ad.Type() == unix.NL80211_ATTR_IFTYPE {
			return iftypeName(int(ad.Uint32()))
		}
	}
	return "", false
}

// MeshForwarding returns the mesh_fwding parameter of a mesh interface.
func (c *Client) MeshForwarding(ifindex int) (bool, error) {
	ae := netlink.NewAttributeEncoder()
	ae.Uint32(unix.NL80211_ATTR_IFINDEX, uint32(ifindex))
	req, err := ae.Encode()
	if err != nil {
		return false, fmt.Errorf("encode get_mesh_config request: %w", err)
	}

	msgs, err := c.execute(unix.NL80211_CMD_GET_MESH_CONFIG, req, netlink.Request)
	if err != nil {
		return false, err
	}

	for _, msg := range msgs {
		if fwd, ok := parseMeshForwarding(msg.Data); ok {
			return fwd, nil
		}
	}
	return false, fmt.Errorf("no mesh config for ifindex %d", ifindex)
}

func parseMeshForwarding(data []byte) (bool, bool) {
	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return false, false
	}
	for ad.Next() {
		if ad.Type() != unix.NL80211_ATTR_MESH_CONFIG {
			continue
		}
		nested, err := netlink.NewAttributeDecoder(ad.Bytes())
		if err != nil {
			return false, false
		}
		for nested.Next() {
			if nested.Type() == unix.NL80211_MESHCONF_FORWARDING {
				return nested.Uint8() != 0, true
			}
		}
	}
	return false, false
}
