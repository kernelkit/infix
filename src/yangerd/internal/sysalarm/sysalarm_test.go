package sysalarm

import (
	"log/slog"
	"testing"
)

const raucStatus = `{"compatible":"infix-x86_64","booted":"internal-primary","boot_primary":"rootfs.0","slots":[
 {"rootfs.0":{"class":"rootfs","bootname":"internal-primary","boot_status":"good","state":"booted"}},
 {"rootfs.1":{"class":"rootfs","bootname":"internal-secondary","boot_status":"bad","state":"inactive"}},
 {"net.0":{"class":"net","bootname":"net","boot_status":"bad","state":"inactive"}}]}`

// The netboot slot is always bad on a disk-booted box and must not
// alarm, the sibling rootfs slot must, by its bootname.
func TestSlotStatesIgnoresOtherClasses(t *testing.T) {
	states := slotStates([]byte(raucStatus), slog.Default())
	if len(states) != 1 || states[0].name != "secondary" || states[0].booted != "primary" || !states[0].bad {
		t.Fatalf("slotStates = %+v", states)
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
