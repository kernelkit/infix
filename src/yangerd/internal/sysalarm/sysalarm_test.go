package sysalarm

import (
	"log/slog"
	"testing"
)

const raucStatus = `{"compatible":"infix-x86_64","booted":"internal-primary","boot_primary":"rootfs.0","slots":[
 {"rootfs.0":{"class":"rootfs","bootname":"internal-primary","boot_status":"good","state":"booted","slot_status":{"bundle":{"version":"v26.10.0"}}}},
 {"rootfs.1":{"class":"rootfs","bootname":"internal-secondary","boot_status":"bad","state":"inactive","slot_status":{"bundle":{"version":"v26.9.1"}}}},
 {"net.0":{"class":"net","bootname":"net","boot_status":"bad","state":"inactive"}}]}`

// The netboot slot is always bad on a disk-booted box and must not
// alarm, the sibling rootfs slot must, by its bootname.
func TestSlotStatesIgnoresOtherClasses(t *testing.T) {
	states := slotStates([]byte(raucStatus), slog.Default())
	if len(states) != 1 || states[0].name != "secondary" || states[0].booted != "primary" || !states[0].bad || !states[0].outdated {
		t.Fatalf("slotStates = %+v", states)
	}
}

// A newer release staged in the other slot, waiting for a reboot, is
// not out of date; an empty slot is.
func TestSlotStatesVersions(t *testing.T) {
	for _, tc := range []struct {
		other    string
		outdated bool
	}{
		{`"slot_status":{"bundle":{"version":"v26.11.0"}}`, false},
		{`"slot_status":{"bundle":{"version":"v26.10.0"}}`, false},
		{`"slot_status":{"bundle":{"version":"v26.10.0-rc1"}}`, false},
		{`"slot_status":{"bundle":{"version":"v25.12.3"}}`, true},
		{`"slot_status":{}`, true},
	} {
		st := `{"booted":"primary","slots":[{"rootfs.0":{"class":"rootfs","bootname":"primary","boot_status":"good","slot_status":{"bundle":{"version":"v26.10.0"}}}},
			{"rootfs.1":{"class":"rootfs","bootname":"secondary","boot_status":"good",` + tc.other + `}}]}`
		states := slotStates([]byte(st), slog.Default())
		if len(states) != 1 || states[0].outdated != tc.outdated {
			t.Errorf("%s: states = %+v, want outdated %v", tc.other, states, tc.outdated)
		}
	}
}

func TestOlderVersion(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		older bool
	}{
		{"v26.9.1", "v26.10.0", true},
		{"v26.10.0", "v26.9.1", false},
		{"v26.10.0", "v26.10.0", false},
		{"v26.10", "v26.10.0", false},
		{"", "v26.10.0", true},
		{"unknown", "v26.10.0", true},
	} {
		if got := olderVersion(tc.a, tc.b); got != tc.older {
			t.Errorf("olderVersion(%q, %q) = %v", tc.a, tc.b, got)
		}
	}
}

func TestSlotStatesNoneBad(t *testing.T) {
	ok := `{"booted":"primary","slots":[{"rootfs.0":{"class":"rootfs","bootname":"primary","boot_status":"good"}},
		{"rootfs.1":{"class":"rootfs","bootname":"secondary","boot_status":"good"}},
		{"net.0":{"class":"net","bootname":"net","boot_status":"bad"}}]}`
	states := slotStates([]byte(ok), slog.Default())
	if len(states) != 1 || states[0].name != "secondary" || states[0].bad {
		t.Fatalf("slotStates = %+v", states)
	}
}

// Booted from the net slot: nothing on disk is a fallback of it.
func TestSlotStatesNetboot(t *testing.T) {
	net := `{"booted":"net","slots":[{"rootfs.0":{"class":"rootfs","bootname":"primary","boot_status":"bad"}},
		{"net.0":{"class":"net","bootname":"net","boot_status":"good"}}]}`
	if states := slotStates([]byte(net), slog.Default()); len(states) != 0 {
		t.Fatalf("slotStates = %+v", states)
	}
}
