package iwmonitor

import "testing"

func TestStandard(t *testing.T) {
	for flags, want := range map[string]string{
		"[AUTH][ASSOC][AUTHORIZED][WMM][HT][VHT][HE]": "11ax",
		"[AUTH][ASSOC][AUTHORIZED][WMM][HT][VHT]":     "11ac",
		"[AUTH][ASSOC][AUTHORIZED][WMM][HT]":          "11n",
		"[AUTH][ASSOC][AUTHORIZED]":                   "legacy",
		"":                                            "",
	} {
		if got := flagsStandard(flags); got != want {
			t.Errorf("flagsStandard(%q) = %q, want %q", flags, got, want)
		}
	}
	if got := generationStandard("6"); got != "11ax" {
		t.Errorf("generationStandard(6) = %q", got)
	}
	if got := generationStandard(""); got != "" {
		t.Errorf("generationStandard() = %q", got)
	}
}
