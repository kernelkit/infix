package nl80211

import (
	"strings"
	"testing"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

// Only 2.4, 5 and 6 GHz are reported; hwsim's S1G and 60 GHz bands are
// dropped rather than listed as "Unknown".
func TestFinalizeBandsDropsUnsupported(t *testing.T) {
	bands := map[uint16]*bandInfo{
		0: {frequencies: []interface{}{2412, 2437}, htCapable: true},
		1: {frequencies: []interface{}{5180, 5200}, vhtCapable: true},
		2: {frequencies: []interface{}{58320, 60480}},
		4: {frequencies: []interface{}{902, 904}},
		3: {disabled: []interface{}{5955, 5975}, heCapable: true},
	}

	var names []string
	for _, raw := range finalizeBands(bands) {
		names = append(names, raw.(map[string]interface{})["name"].(string))
	}
	if got := strings.Join(names, ","); got != "2.4 GHz,5 GHz,6 GHz" {
		t.Fatalf("bands = %v, want 2.4, 5 and the disabled 6 GHz", names)
	}
}

func TestManufacturerFor(t *testing.T) {
	for driver, want := range map[string]string{
		"mac80211_hwsim": "Virtual (hwsim)",
		"mt7915e":        "MediaTek Inc.",
		"ath11k_pci":     "Qualcomm Atheros",
		"":               "Unknown",
	} {
		if got := manufacturerFor(driver); got != want {
			t.Errorf("manufacturerFor(%q) = %q, want %q", driver, got, want)
		}
	}
}

// A split wiphy dump carries one frequency per message.
func TestMergeBandsAcrossMessages(t *testing.T) {
	bands := map[uint16]*bandInfo{}

	for _, freq := range []uint32{2412, 2417, 2422} {
		ae := netlink.NewAttributeEncoder()
		ae.Nested(0, func(band *netlink.AttributeEncoder) error {
			band.Nested(unix.NL80211_BAND_ATTR_FREQS, func(freqs *netlink.AttributeEncoder) error {
				freqs.Nested(0, func(entry *netlink.AttributeEncoder) error {
					entry.Uint32(unix.NL80211_FREQUENCY_ATTR_FREQ, freq)
					return nil
				})
				return nil
			})
			return nil
		})
		data, err := ae.Encode()
		if err != nil {
			t.Fatal(err)
		}
		mergeBands(bands, data)
	}

	if got := len(bands[0].frequencies); got != 3 {
		t.Fatalf("frequencies = %v, want 3 entries", bands[0].frequencies)
	}
}
