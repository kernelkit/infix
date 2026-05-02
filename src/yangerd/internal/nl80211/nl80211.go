package nl80211

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

type Client struct {
	conn   *genetlink.Conn
	family genetlink.Family
}

func Dial() (*Client, error) {
	conn, err := genetlink.Dial(nil)
	if err != nil {
		return nil, fmt.Errorf("dial genetlink: %w", err)
	}

	family, err := conn.GetFamily("nl80211")
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("resolve nl80211 family: %w", err)
	}

	return &Client{conn: conn, family: family}, nil
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) ListPhys() ([]string, error) {
	msgs, err := c.execute(unix.NL80211_CMD_GET_WIPHY, nil, netlink.Request|netlink.Dump)
	if err != nil {
		return nil, err
	}

	set := make(map[string]bool)
	for _, msg := range msgs {
		attrs, err := netlink.NewAttributeDecoder(msg.Data)
		if err != nil {
			continue
		}
		for attrs.Next() {
			if attrs.Type() != unix.NL80211_ATTR_WIPHY_NAME {
				continue
			}
			name := attrs.String()
			if name != "" {
				set[name] = true
			}
		}
		if err := attrs.Err(); err != nil {
			continue
		}
	}

	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)

	return out, nil
}

func (c *Client) PhyInterfaces() (map[string][]string, error) {
	msgs, err := c.execute(unix.NL80211_CMD_GET_INTERFACE, nil, netlink.Request|netlink.Dump)
	if err != nil {
		return nil, err
	}

	out := make(map[string][]string)
	for _, msg := range msgs {
		ad, err := netlink.NewAttributeDecoder(msg.Data)
		if err != nil {
			continue
		}

		phyIdx := -1
		ifname := ""

		for ad.Next() {
			switch ad.Type() {
			case unix.NL80211_ATTR_WIPHY:
				phyIdx = int(ad.Uint32())
			case unix.NL80211_ATTR_IFNAME:
				ifname = ad.String()
			}
		}
		if err := ad.Err(); err != nil {
			continue
		}
		if phyIdx < 0 || ifname == "" {
			continue
		}

		k := strconv.Itoa(phyIdx)
		out[k] = append(out[k], ifname)
	}

	for k := range out {
		sort.Strings(out[k])
	}

	return out, nil
}

func (c *Client) PhyInfo(phyName string) (map[string]interface{}, error) {
	ae := netlink.NewAttributeEncoder()
	ae.Flag(unix.NL80211_ATTR_SPLIT_WIPHY_DUMP, true)
	req, _ := ae.Encode()
	msgs, err := c.execute(unix.NL80211_CMD_GET_WIPHY, req, netlink.Request|netlink.Dump)
	if err != nil {
		return nil, err
	}

	// Collect all messages belonging to the target PHY.  The kernel
	// identifies fragments by repeating NL80211_ATTR_WIPHY (index)
	// or NL80211_ATTR_WIPHY_NAME in each fragment.
	targetIdx := -1
	var phyMsgs [][]byte
	for _, msg := range msgs {
		idx, name := parseWiphyIdent(msg.Data)
		if name == phyName {
			targetIdx = idx
			phyMsgs = append(phyMsgs, msg.Data)
		} else if targetIdx >= 0 && idx == targetIdx {
			phyMsgs = append(phyMsgs, msg.Data)
		}
	}
	if len(phyMsgs) == 0 {
		return nil, fmt.Errorf("phy %q not found", phyName)
	}

	info := map[string]interface{}{
		"bands":                  []interface{}{},
		"driver":                 readDriver(phyName),
		"manufacturer":           readManufacturer(phyName),
		"interface_combinations": []interface{}{},
		"max_txpower":            0,
		"num_virtual_interfaces": 0,
	}

	phyIdx := -1
	bandMap := make(map[uint16]*bandInfo)

	for _, data := range phyMsgs {
		ad, err := netlink.NewAttributeDecoder(data)
		if err != nil {
			continue
		}
		for ad.Next() {
			switch ad.Type() {
			case unix.NL80211_ATTR_WIPHY:
				phyIdx = int(ad.Uint32())
			case unix.NL80211_ATTR_WIPHY_BANDS:
				mergeBands(bandMap, ad.Bytes())
			case unix.NL80211_ATTR_INTERFACE_COMBINATIONS:
				if combs := parseInterfaceCombinations(ad.Bytes()); len(combs) > 0 {
					info["interface_combinations"] = combs
				}
			case unix.NL80211_ATTR_WIPHY_TX_POWER_LEVEL:
				info["max_txpower"] = int(ad.Uint32() / 100)
			}
		}
	}
	if bands := finalizeBands(bandMap); len(bands) > 0 {
		info["bands"] = bands
	}

	if phyIdx >= 0 {
		ifs, err := c.PhyInterfaces()
		if err == nil {
			info["num_virtual_interfaces"] = len(ifs[strconv.Itoa(phyIdx)])
		}
	}

	return info, nil
}

func (c *Client) Survey(ifindex int) ([]map[string]interface{}, error) {
	ae := netlink.NewAttributeEncoder()
	ae.Uint32(unix.NL80211_ATTR_IFINDEX, uint32(ifindex))
	req, err := ae.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode get_survey request: %w", err)
	}

	msgs, err := c.execute(unix.NL80211_CMD_GET_SURVEY, req, netlink.Request|netlink.Dump)
	if err != nil {
		return nil, err
	}

	out := make([]map[string]interface{}, 0)
	for _, msg := range msgs {
		ad, err := netlink.NewAttributeDecoder(msg.Data)
		if err != nil {
			continue
		}
		for ad.Next() {
			if ad.Type() != unix.NL80211_ATTR_SURVEY_INFO {
				continue
			}
			entry := parseSurveyEntry(ad.Bytes())
			if entry != nil {
				out = append(out, entry)
			}
		}
		if err := ad.Err(); err != nil {
			continue
		}
	}

	return out, nil
}

func (c *Client) execute(cmd uint8, data []byte, flags netlink.HeaderFlags) ([]genetlink.Message, error) {
	msgs, err := c.conn.Execute(
		genetlink.Message{Header: genetlink.Header{Command: cmd, Version: c.family.Version}, Data: data},
		c.family.ID,
		flags,
	)
	if err != nil {
		return nil, fmt.Errorf("nl80211 command %d: %w", cmd, err)
	}

	return msgs, nil
}

func parseWiphyIdent(data []byte) (int, string) {
	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return -1, ""
	}
	idx := -1
	name := ""
	for ad.Next() {
		switch ad.Type() {
		case unix.NL80211_ATTR_WIPHY:
			idx = int(ad.Uint32())
		case unix.NL80211_ATTR_WIPHY_NAME:
			name = ad.String()
		}
	}
	return idx, name
}

type bandInfo struct {
	frequencies []interface{}
	htCapable   bool
	vhtCapable  bool
	heCapable   bool
}

func mergeBands(m map[uint16]*bandInfo, data []byte) {
	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return
	}
	for ad.Next() {
		bandType := ad.Type()
		bi, ok := m[bandType]
		if !ok {
			bi = &bandInfo{}
			m[bandType] = bi
		}
		mergeBandAttrs(bi, ad.Bytes())
	}
}

func mergeBandAttrs(bi *bandInfo, data []byte) {
	nad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return
	}
	for nad.Next() {
		switch nad.Type() {
		case unix.NL80211_BAND_ATTR_FREQS:
			if freqs := parseBandFrequencies(nad.Bytes()); len(freqs) > 0 {
				bi.frequencies = freqs
			}
		case unix.NL80211_BAND_ATTR_HT_CAPA:
			if nad.Uint16() != 0 {
				bi.htCapable = true
			}
		case unix.NL80211_BAND_ATTR_VHT_CAPA:
			if nad.Uint32() != 0 {
				bi.vhtCapable = true
			}
		case unix.NL80211_BAND_ATTR_IFTYPE_DATA:
			if len(nad.Bytes()) > 0 {
				bi.heCapable = true
			}
		}
	}
}

func finalizeBands(m map[uint16]*bandInfo) []interface{} {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)

	out := make([]interface{}, 0, len(keys))
	for _, k := range keys {
		bi := m[uint16(k)]
		if len(bi.frequencies) == 0 && !bi.htCapable && !bi.vhtCapable && !bi.heCapable {
			continue
		}
		out = append(out, map[string]interface{}{
			"band":        k,
			"name":        detectBandName(bi.frequencies),
			"ht_capable":  bi.htCapable,
			"vht_capable": bi.vhtCapable,
			"he_capable":  bi.heCapable,
			"frequencies": bi.frequencies,
		})
	}
	return out
}

func parseBandFrequencies(data []byte) []interface{} {
	out := make([]interface{}, 0)

	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return out
	}

	for ad.Next() {
		freq, ok := parseFrequencyEntry(ad.Bytes())
		if ok {
			out = append(out, freq)
		}
	}

	return out
}

func parseFrequencyEntry(data []byte) (int, bool) {
	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return 0, false
	}

	freq := 0
	disabled := false

	for ad.Next() {
		switch ad.Type() {
		case unix.NL80211_FREQUENCY_ATTR_FREQ:
			freq = int(ad.Uint32())
		case unix.NL80211_FREQUENCY_ATTR_DISABLED:
			disabled = true
		}
	}

	if disabled || freq == 0 {
		return 0, false
	}

	return freq, true
}

func detectBandName(freqs []interface{}) string {
	has24 := false
	has5 := false
	has6 := false

	for _, f := range freqs {
		freq, ok := f.(int)
		if !ok {
			continue
		}
		switch {
		case freq >= 2400 && freq <= 2500:
			has24 = true
		case freq >= 5000 && freq <= 5900:
			has5 = true
		case freq >= 5925 && freq <= 7125:
			has6 = true
		}
	}

	switch {
	case has24:
		return "2.4 GHz"
	case has5:
		return "5 GHz"
	case has6:
		return "6 GHz"
	default:
		return "Unknown"
	}
}

func parseInterfaceCombinations(data []byte) []interface{} {
	out := make([]interface{}, 0)

	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return out
	}

	for ad.Next() {
		comb := parseInterfaceCombination(ad.Bytes())
		if comb != nil {
			out = append(out, comb)
		}
	}

	return out
}

func parseInterfaceCombination(data []byte) map[string]interface{} {
	limits := make([]interface{}, 0)

	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return nil
	}

	for ad.Next() {
		if ad.Type() != unix.NL80211_IFACE_COMB_LIMITS {
			continue
		}
		limits = parseInterfaceLimits(ad.Bytes())
	}

	if len(limits) == 0 {
		return nil
	}

	return map[string]interface{}{"limits": limits}
}

func parseInterfaceLimits(data []byte) []interface{} {
	out := make([]interface{}, 0)

	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return out
	}

	for ad.Next() {
		entry := parseInterfaceLimitEntry(ad.Bytes())
		if entry != nil {
			out = append(out, entry)
		}
	}

	return out
}

func parseInterfaceLimitEntry(data []byte) map[string]interface{} {
	max := 0
	types := make([]interface{}, 0)

	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return nil
	}

	for ad.Next() {
		switch ad.Type() {
		case unix.NL80211_IFACE_LIMIT_MAX:
			max = int(ad.Uint32())
		case unix.NL80211_IFACE_LIMIT_TYPES:
			types = parseIfaceLimitTypes(ad.Bytes())
		}
	}

	if max == 0 || len(types) == 0 {
		return nil
	}

	return map[string]interface{}{
		"max":   max,
		"types": types,
	}
}

func parseIfaceLimitTypes(data []byte) []interface{} {
	out := make([]interface{}, 0)
	seen := make(map[string]bool)

	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return out
	}

	for ad.Next() {
		if len(ad.Bytes()) == 4 {
			iftype := int(ad.Uint32())
			if s, ok := iftypeName(iftype); ok {
				if !seen[s] {
					seen[s] = true
					out = append(out, s)
				}
			}
			continue
		}

		iftype := int(ad.Type())
		if s, ok := iftypeName(iftype); ok {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].(string) < out[j].(string)
	})

	return out
}

func iftypeName(v int) (string, bool) {
	switch v {
	case 0:
		return "unspecified", true
	case 1:
		return "adhoc", true
	case 2:
		return "station", true
	case 3:
		return "AP", true
	case 4:
		return "AP_VLAN", true
	case 5:
		return "WDS", true
	case 6:
		return "monitor", true
	case 7:
		return "mesh_point", true
	case 8:
		return "P2P_client", true
	case 9:
		return "P2P_GO", true
	case 10:
		return "P2P_device", true
	default:
		return "", false
	}
}

func parseSurveyEntry(data []byte) map[string]interface{} {
	entry := map[string]interface{}{
		"frequency":     0,
		"noise":         0,
		"in_use":        false,
		"active_time":   0,
		"busy_time":     0,
		"receive_time":  0,
		"transmit_time": 0,
	}

	ad, err := netlink.NewAttributeDecoder(data)
	if err != nil {
		return nil
	}

	hasFrequency := false

	for ad.Next() {
		switch ad.Type() {
		case unix.NL80211_SURVEY_INFO_FREQUENCY:
			entry["frequency"] = int(readUint(ad.Bytes()))
			hasFrequency = true
		case unix.NL80211_SURVEY_INFO_NOISE:
			entry["noise"] = int(ad.Int8())
		case unix.NL80211_SURVEY_INFO_IN_USE:
			entry["in_use"] = true
		case unix.NL80211_SURVEY_INFO_TIME:
			entry["active_time"] = int(readUint(ad.Bytes()))
		case unix.NL80211_SURVEY_INFO_TIME_BUSY:
			entry["busy_time"] = int(readUint(ad.Bytes()))
		case unix.NL80211_SURVEY_INFO_TIME_RX:
			entry["receive_time"] = int(readUint(ad.Bytes()))
		case unix.NL80211_SURVEY_INFO_TIME_TX:
			entry["transmit_time"] = int(readUint(ad.Bytes()))
		}
	}

	if !hasFrequency {
		return nil
	}

	return entry
}

func readUint(b []byte) uint64 {
	switch len(b) {
	case 1:
		return uint64(b[0])
	case 2:
		return uint64(binary.NativeEndian.Uint16(b))
	case 4:
		return uint64(binary.NativeEndian.Uint32(b))
	case 8:
		return binary.NativeEndian.Uint64(b)
	default:
		return 0
	}
}

func readDriver(phyName string) string {
	path := filepath.Join("/sys/class/ieee80211", phyName, "device", "driver")
	target, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	if base := filepath.Base(target); base != "." && base != "/" {
		return base
	}
	return ""
}

func readManufacturer(phyName string) string {
	driver := readDriver(phyName)
	if driver == "" {
		return "Unknown"
	}
	d := strings.ToLower(driver)
	switch {
	case strings.Contains(d, "mt") || strings.Contains(d, "mediatek"):
		return "MediaTek Inc."
	case strings.Contains(d, "rtw") || strings.Contains(d, "realtek"):
		return "Realtek Semiconductor Corp."
	case strings.Contains(d, "ath") || strings.Contains(d, "qca"):
		return "Qualcomm Atheros"
	case strings.Contains(d, "iwl") || strings.Contains(d, "intel"):
		return "Intel Corporation"
	case strings.Contains(d, "brcm") || strings.Contains(d, "broadcom"):
		return "Broadcom Inc."
	default:
		return "Unknown"
	}
}
