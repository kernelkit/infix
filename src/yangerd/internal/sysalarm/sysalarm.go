// Package sysalarm raises the alarms that come from the boot itself: a
// startup configuration that failed to apply, and a software slot the
// bootloader gave up on.
package sysalarm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kernelkit/infix/src/yangerd/internal/alarm"
	"github.com/kernelkit/infix/src/yangerd/internal/inotify"
)

// CondDir is where confd sets its finit conditions, overridable in tests.
var CondDir = "/run/finit/cond/usr"

const (
	startupType   = "startup-config-failed"
	bootType      = "boot-fallback"
	partitionType = "partition-out-of-date"
)

// RunStartupConfig watches the finit conditions confd sets after the
// bootstrap and raises startup-config-failed while one says the
// startup, or the failure, configuration could not be applied.  The
// condition stays until reboot, and so does the alarm.
func RunStartupConfig(ctx context.Context, log *slog.Logger) error {
	w, err := inotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("inotify: %w", err)
	}
	defer w.Close()
	if err := w.Add(CondDir); err != nil {
		return fmt.Errorf("watch %s: %w", CondDir, err)
	}

	checkStartupConfig(log)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-w.Events:
			if !ok {
				return fmt.Errorf("watcher closed")
			}
			checkStartupConfig(log)
		case err, ok := <-w.Errors:
			if !ok {
				return fmt.Errorf("watcher error channel closed")
			}
			log.Warn("sysalarm: inotify error", "err", err)
		}
	}
}

func checkStartupConfig(log *slog.Logger) {
	var err error
	switch {
	case exists(filepath.Join(CondDir, "failure-config-error")):
		err = alarm.Raise(startupType, "startup-config", "", alarm.Critical,
			"Neither the startup nor the failure configuration could be applied, the device is unconfigured.")
	case exists(filepath.Join(CondDir, "startup-config-error")):
		err = alarm.Raise(startupType, "startup-config", "", alarm.Critical,
			"The startup configuration could not be applied, the device runs its failure configuration.")
	default:
		err = alarm.Clear(startupType, "startup-config", "")
	}
	if err != nil {
		log.Warn("sysalarm: startup-config alarm", "err", err)
	}
}

// BootFallback raises boot-fallback for a software slot the bootloader
// has marked bad, which is how a slot that failed to boot looks after
// the fallback, and partition-out-of-date for one holding an older
// release than the booted one, or none, since it is the fallback the
// next upgrade should refresh.  Fed by the D-Bus monitor's RAUC status
// read, at start and after each install, so the status is read once.
// Only slots of the booted slot's class count: the netboot slot is
// never booted from disk and is always bad.
func BootFallback(raucData map[string]interface{}, log *slog.Logger) {
	out, err := json.Marshal(raucData)
	if err != nil {
		return
	}
	for _, slot := range slotStates(out, log) {
		set := func(typ string, raise bool, sev alarm.Severity, text string) {
			var err error
			if raise {
				err = alarm.Raise(typ, slot.name, "", sev, text)
			} else {
				err = alarm.Clear(typ, slot.name, "")
			}
			if err != nil {
				log.Warn("sysalarm: "+typ+" alarm", "err", err)
			}
		}
		set(bootType, slot.bad, alarm.Major,
			fmt.Sprintf("Software slot %s failed to boot and is marked bad, the device runs %s.", slot.name, slot.booted))
		set(partitionType, slot.outdated, alarm.Warning,
			fmt.Sprintf("The %s partition has %s, this is %s, use 'upgrade' to update it.", slot.name, slot.version, slot.bootedVersion))
	}
}

type slotState struct {
	name, booted           string
	version, bootedVersion string
	bad, outdated          bool
}

// slotStates parses a RAUC status and returns the sibling slots of the
// booted one, named by their bootname as the boot order lists them,
// with whether the bootloader has marked each bad.
func slotStates(out []byte, log *slog.Logger) []slotState {
	var status struct {
		Booted string `json:"booted"`
		Slots  []map[string]struct {
			Class      string `json:"class"`
			Bootname   string `json:"bootname"`
			BootStatus string `json:"boot_status"`
			SlotStatus struct {
				Bundle struct {
					Version string `json:"version"`
				} `json:"bundle"`
			} `json:"slot_status"`
		} `json:"slots"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		log.Warn("sysalarm: rauc status", "err", err)
		return nil
	}

	booted := strings.TrimPrefix(status.Booted, "internal-")
	class, bootedVersion := "", ""
	for _, slots := range status.Slots {
		for _, slot := range slots {
			if strings.TrimPrefix(slot.Bootname, "internal-") == booted {
				class = slot.Class
				bootedVersion = slot.SlotStatus.Bundle.Version
			}
		}
	}
	if class == "" {
		return nil
	}

	var states []slotState
	for _, slots := range status.Slots {
		for _, slot := range slots {
			name := strings.TrimPrefix(slot.Bootname, "internal-")
			if slot.Class != class || name == booted {
				continue
			}
			version := slot.SlotStatus.Bundle.Version
			st := slotState{name: name, booted: booted, version: version, bootedVersion: bootedVersion,
				bad: slot.BootStatus == "bad", outdated: bootedVersion != "" && olderVersion(version, bootedVersion)}
			if version == "" {
				st.version = "no release"
			}
			states = append(states, st)
		}
	}
	return states
}

// olderVersion reports whether release a is older than b, comparing
// the numbers in them in order, so v26.10.0 is newer than v26.9.1 and
// an empty or unreadable version is older than anything.
func olderVersion(a, b string) bool {
	na, nb := versionNumbers(a), versionNumbers(b)
	if len(na) == 0 {
		return len(nb) > 0
	}
	for i := 0; i < len(na) || i < len(nb); i++ {
		var x, y int
		if i < len(na) {
			x = na[i]
		}
		if i < len(nb) {
			y = nb[i]
		}
		if x != y {
			return x < y
		}
	}
	return false
}

func versionNumbers(v string) []int {
	var nums []int
	n, in := 0, false
	for _, c := range v {
		if c >= '0' && c <= '9' {
			n, in = n*10+int(c-'0'), true
			continue
		}
		if in {
			nums = append(nums, n)
			n, in = 0, false
		}
	}
	if in {
		nums = append(nums, n)
	}
	return nums
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
