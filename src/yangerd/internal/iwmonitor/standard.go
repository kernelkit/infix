package iwmonitor

import "strings"

// flagsStandard names the newest 802.11 amendment in the flags hostapd
// lists for a station, e.g. "[AUTH][ASSOC][AUTHORIZED][WMM][HT][VHT]".
func flagsStandard(flags string) string {
	switch {
	case strings.Contains(flags, "[EHT]"):
		return "11be"
	case strings.Contains(flags, "[HE]"):
		return "11ax"
	case strings.Contains(flags, "[VHT]"):
		return "11ac"
	case strings.Contains(flags, "[HT]"):
		return "11n"
	case flags != "":
		return "legacy"
	}
	return ""
}

// generationStandard maps the wifi_generation of a wpa_supplicant
// STATUS reply to the amendment behind it.
func generationStandard(generation string) string {
	switch generation {
	case "4":
		return "11n"
	case "5":
		return "11ac"
	case "6":
		return "11ax"
	case "7":
		return "11be"
	}
	return ""
}
