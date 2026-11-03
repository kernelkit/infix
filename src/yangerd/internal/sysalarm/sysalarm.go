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
	startupType = "startup-config-failed"
	bootType    = "boot-fallback"
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
// the fallback, and clears it for the others.  Fed by the D-Bus
// monitor's RAUC status read, at start and after each install, so the
// status is read once.  Only slots of the booted slot's class count:
// the netboot slot is never booted from disk and is always bad.
func BootFallback(raucData map[string]interface{}, log *slog.Logger) {
	out, err := json.Marshal(raucData)
	if err != nil {
		return
	}
	for _, slot := range slotStates(out, log) {
		var err error
		if slot.bad {
			err = alarm.Raise(bootType, slot.name, "", alarm.Major,
				fmt.Sprintf("Software slot %s failed to boot and is marked bad, the device runs %s.", slot.name, slot.booted))
		} else {
			err = alarm.Clear(bootType, slot.name, "")
		}
		if err != nil {
			log.Warn("sysalarm: boot-fallback alarm", "err", err)
		}
	}
}

type slotState struct {
	name, booted string
	bad          bool
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
		} `json:"slots"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		log.Warn("sysalarm: rauc status", "err", err)
		return nil
	}

	booted := strings.TrimPrefix(status.Booted, "internal-")
	class := ""
	for _, slots := range status.Slots {
		for _, slot := range slots {
			if strings.TrimPrefix(slot.Bootname, "internal-") == booted {
				class = slot.Class
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
			states = append(states, slotState{name: name, booted: booted, bad: slot.BootStatus == "bad"})
		}
	}
	return states
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
