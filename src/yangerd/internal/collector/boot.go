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

	raucOut, err := cmd.Run(ctx, "rauc", "status", "--detailed", "--output-format=json")
	if err == nil {
		var raucData map[string]interface{}
		if json.Unmarshal(raucOut, &raucData) == nil {
			if v, ok := raucData["compatible"]; ok {
				software["compatible"] = v
			}
			if v, ok := raucData["variant"]; ok {
				software["variant"] = v
			}
			if v, ok := raucData["booted"]; ok {
				software["booted"] = v
			}
			bootSoftwareSlots(software, raucData)
		}
	}

	bootOrder := ReadBootOrder(ctx, cmd)
	if bootOrder != nil {
		software["boot-order"] = bootOrder
	}

	installer := make(map[string]interface{})
	instOut, err := cmd.Run(ctx, "rauc-installation-status")
	if err == nil {
		var instData map[string]interface{}
		if json.Unmarshal(instOut, &instData) == nil {
			if op, ok := instData["operation"]; ok && op != "" {
				installer["operation"] = op
			}
			if prog, ok := instData["progress"].(map[string]interface{}); ok {
				progress := make(map[string]interface{})
				if pct, ok := prog["percentage"]; ok {
					progress["percentage"] = toInt(pct)
				}
				if msg, ok := prog["message"]; ok {
					progress["message"] = msg
				}
				installer["progress"] = progress
			}
		}
	}
	software["installer"] = installer

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

func bootSoftwareSlots(software map[string]interface{}, raucData map[string]interface{}) {
	slotsRaw, ok := raucData["slots"]
	if !ok {
		return
	}
	slotsArr, ok := slotsRaw.([]interface{})
	if !ok {
		return
	}

	var slots []interface{}
	for _, slotItem := range slotsArr {
		slotMap, ok := slotItem.(map[string]interface{})
		if !ok {
			continue
		}
		for name, valRaw := range slotMap {
			val, ok := valRaw.(map[string]interface{})
			if !ok {
				continue
			}
			s := map[string]interface{}{
				"name":     name,
				"bootname": val["bootname"],
				"class":    val["class"],
				"state":    val["state"],
			}

			slotStatus, _ := val["slot_status"].(map[string]interface{})
			if slotStatus == nil {
				slots = append(slots, s)
				continue
			}

			bundle := make(map[string]interface{})
			if b, ok := slotStatus["bundle"].(map[string]interface{}); ok {
				if v := b["compatible"]; v != nil {
					bundle["compatible"] = v
				}
				if v := b["version"]; v != nil {
					bundle["version"] = v
				}
			}
			s["bundle"] = bundle

			if ck, ok := slotStatus["checksum"].(map[string]interface{}); ok {
				if v := ck["size"]; v != nil {
					s["size"] = strconv.FormatInt(int64(toInt(v)), 10)
				}
				if v := ck["sha256"]; v != nil {
					s["sha256"] = v
				}
			}

			installed := make(map[string]interface{})
			if inst, ok := slotStatus["installed"].(map[string]interface{}); ok {
				if v := inst["timestamp"]; v != nil {
					installed["datetime"] = v
				}
				if v := inst["count"]; v != nil {
					installed["count"] = toInt(v)
				}
			}
			s["installed"] = installed

			activated := make(map[string]interface{})
			if act, ok := slotStatus["activated"].(map[string]interface{}); ok {
				if v := act["timestamp"]; v != nil {
					activated["datetime"] = v
				}
				if v := act["count"]; v != nil {
					activated["count"] = toInt(v)
				}
			}
			s["activated"] = activated

			slots = append(slots, s)
		}
	}
	software["slot"] = slots
}
