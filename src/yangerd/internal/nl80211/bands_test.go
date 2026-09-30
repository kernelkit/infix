package nl80211

import "testing"

// Only 2.4, 5 and 6 GHz are reported; hwsim's S1G and 60 GHz bands are
// dropped rather than listed as "Unknown".
func TestFinalizeBandsDropsUnsupported(t *testing.T) {
	bands := map[uint16]*bandInfo{
		0: {frequencies: []interface{}{2412, 2437}, htCapable: true},
		1: {frequencies: []interface{}{5180, 5200}, vhtCapable: true},
		2: {frequencies: []interface{}{58320, 60480}},
		4: {frequencies: []interface{}{902, 904}},
	}

	var names []string
	for _, raw := range finalizeBands(bands) {
		names = append(names, raw.(map[string]interface{})["name"].(string))
	}
	if len(names) != 2 || names[0] != "2.4 GHz" || names[1] != "5 GHz" {
		t.Fatalf("bands = %v, want [2.4 GHz 5 GHz]", names)
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
