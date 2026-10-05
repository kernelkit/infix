package collector

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
)

func BootPlatform(fs FileReader) json.RawMessage {
	data, err := fs.ReadFile("/etc/os-release")
	if err != nil {
		log.Printf("boot: os-release: %v", err)
		return nil
	}
	platform := make(map[string]interface{})
	for _, line := range strings.Split(string(data), "\n") {
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := line[:idx]
		val := strings.Trim(line[idx+1:], "\"")
		if mapped, ok := platformKeyMap[key]; ok {
			platform[mapped] = val
		}
	}
	result, _ := json.Marshal(map[string]interface{}{"platform": platform})
	return result
}

func BootSoftware(ctx context.Context, cmd CommandRunner) json.RawMessage {
	software := make(map[string]interface{})

	var raucData map[string]interface{}
	if err := runJSON(ctx, cmd, &raucData, "rauc", "status", "--detailed", "--output-format=json"); err != nil {
		log.Printf("boot: %v", err)
	} else {
		for _, key := range []string{"compatible", "variant", "booted"} {
			if v, ok := raucData[key]; ok {
				software[key] = v
			}
		}
		if slots := softwareSlots(raucData); slots != nil {
			software["slot"] = slots
		}
	}

	bootOrder := ReadBootOrder(ctx, cmd)
	if bootOrder != nil {
		software["boot-order"] = bootOrder
	}

	result, _ := json.Marshal(map[string]interface{}{"infix-system:software": software})
	return result
}

func ReadBootOrder(ctx context.Context, cmd CommandRunner) []string {
	out, err := cmd.Run(ctx, "fw_printenv", "BOOT_ORDER")
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "BOOT_ORDER") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					return strings.Fields(parts[1])
				}
			}
		}
	}

	out, err = cmd.Run(ctx, "grub-editenv", "/mnt/aux/grub/grubenv", "list")
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "ORDER") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					return strings.Fields(strings.TrimSpace(parts[1]))
				}
			}
		}
	}

	return nil
}

// softwareSlots lists the RAUC slots, nil when rauc reported none.
func softwareSlots(raucData map[string]interface{}) []interface{} {
	slotsArr, ok := raucData["slots"].([]interface{})
	if !ok {
		return nil
	}

	slots := []interface{}{}
	for _, slotItem := range slotsArr {
		slotMap, ok := slotItem.(map[string]interface{})
		if !ok {
			continue
		}
		for name, valRaw := range slotMap {
			if val, ok := valRaw.(map[string]interface{}); ok {
				slots = append(slots, softwareSlot(name, val))
			}
		}
	}
	return slots
}

func softwareSlot(name string, val map[string]interface{}) map[string]interface{} {
	s := map[string]interface{}{
		"name":     name,
		"bootname": val["bootname"],
		"class":    val["class"],
		"state":    val["state"],
	}

	slotStatus, _ := val["slot_status"].(map[string]interface{})
	if slotStatus == nil {
		return s
	}

	bundle := make(map[string]interface{})
	if b, ok := slotStatus["bundle"].(map[string]interface{}); ok {
		setIfPresent(bundle, "compatible", b, "compatible")
		setIfPresent(bundle, "version", b, "version")
	}
	s["bundle"] = bundle

	if ck, ok := slotStatus["checksum"].(map[string]interface{}); ok {
		if v := ck["size"]; v != nil {
			s["size"] = strconv.FormatInt(int64(toInt(v)), 10)
		}
		setIfPresent(s, "sha256", ck, "sha256")
	}

	s["installed"] = slotEvent(slotStatus["installed"])
	s["activated"] = slotEvent(slotStatus["activated"])

	return s
}

// slotEvent maps a RAUC {timestamp, count} record, as for the last
// install or activation of a slot.
func slotEvent(raw interface{}) map[string]interface{} {
	event := make(map[string]interface{})
	if rec, ok := raw.(map[string]interface{}); ok {
		setIfPresent(event, "datetime", rec, "timestamp")
		setIfPresentInt(event, "count", rec, "count")
	}
	return event
}
