package collector

import (
	"encoding/json"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// LiveSystemState computes the on-demand portion of ietf-system:system-state.
// It reads uptime, current time, memory, load average from procfs and
// filesystem usage via statfs — all computed fresh on each call.
func LiveSystemState(fs FileReader) json.RawMessage {
	state := make(map[string]interface{})

	if clock := liveClock(fs); len(clock) > 0 {
		state["clock"] = clock
	}

	resource := make(map[string]interface{})
	if mem := liveMemory(fs); len(mem) > 0 {
		resource["memory"] = mem
	}
	if la := liveLoadAvg(fs); len(la) > 0 {
		resource["load-average"] = la
	}
	if filesys := liveFilesystems(); len(filesys) > 0 {
		resource["filesystem"] = filesys
	}
	if len(resource) > 0 {
		state["infix-system:resource-usage"] = resource
	}

	data, err := json.Marshal(state)
	if err != nil {
		return nil
	}
	return data
}

func liveClock(fs FileReader) map[string]interface{} {
	data, err := fs.ReadFile("/proc/uptime")
	if err != nil {
		return nil
	}
	parts := strings.Fields(string(data))
	if len(parts) < 1 {
		return nil
	}
	upSec, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return nil
	}

	now := time.Now()
	boot := now.Add(-time.Duration(upSec * float64(time.Second)))

	return map[string]interface{}{
		"current-datetime": yangDateTime(now),
		"boot-datetime":    yangDateTime(boot),
	}
}

func liveMemory(fs FileReader) map[string]interface{} {
	data, err := fs.ReadFile("/proc/meminfo")
	if err != nil {
		return nil
	}

	memFields := map[string]string{
		"MemTotal":     "total",
		"MemFree":      "free",
		"MemAvailable": "available",
	}

	memory := make(map[string]interface{})
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		jsonKey, ok := memFields[key]
		if !ok {
			continue
		}
		valStr := strings.TrimSpace(parts[1])
		fields := strings.Fields(valStr)
		if len(fields) < 1 {
			continue
		}
		memory[jsonKey] = fields[0]
	}
	return memory
}

func liveLoadAvg(fs FileReader) map[string]interface{} {
	data, err := fs.ReadFile("/proc/loadavg")
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return nil
	}
	return map[string]interface{}{
		"load-1min":  fields[0],
		"load-5min":  fields[1],
		"load-15min": fields[2],
	}
}

func liveFilesystems() []interface{} {
	mounts := []string{"/", "/var", "/cfg"}
	var filesystems []interface{}

	for _, mount := range mounts {
		var stat syscall.Statfs_t
		if err := syscall.Statfs(mount, &stat); err != nil {
			continue
		}
		bsize := uint64(stat.Bsize)
		sizeKB := (stat.Blocks * bsize) / 1024
		availKB := (stat.Bavail * bsize) / 1024
		usedKB := sizeKB - (stat.Bfree*bsize)/1024

		filesystems = append(filesystems, map[string]interface{}{
			"mount-point": mount,
			"size":        strconv.FormatUint(sizeKB, 10),
			"used":        strconv.FormatUint(usedKB, 10),
			"available":   strconv.FormatUint(availKB, 10),
		})
	}
	return filesystems
}
