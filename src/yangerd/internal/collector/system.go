package collector

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

var platformKeyMap = map[string]string{
	"NAME":         "os-name",
	"VERSION_ID":   "os-version",
	"BUILD_ID":     "os-release",
	"ARCHITECTURE": "machine",
}

// SystemCollector gathers ietf-system operational data.
type SystemCollector struct {
	cmd      CommandRunner
	fs       FileReader
	interval time.Duration
}

// NewSystemCollector creates a SystemCollector with the given dependencies.
func NewSystemCollector(cmd CommandRunner, fs FileReader, interval time.Duration) *SystemCollector {
	return &SystemCollector{cmd: cmd, fs: fs, interval: interval}
}

// Name implements Collector.
func (c *SystemCollector) Name() string { return "system" }

// Interval implements Collector.
func (c *SystemCollector) Interval() time.Duration { return c.interval }

// Collect implements Collector.  It merges service data into
// "ietf-system:system-state".  DNS is handled reactively by
// fswatcher on /var/lib/misc/resolv.conf.  Other system-state
// subtrees (platform, software, users, hostname, timezone, clock,
// memory, load, filesystems) are populated by boot-once, reactive,
// or on-demand providers.
func (c *SystemCollector) Collect(ctx context.Context, t *tree.Tree) error {
	state := make(map[string]interface{})

	c.addServices(ctx, state)

	if data, err := json.Marshal(state); err == nil {
		t.Merge("ietf-system:system-state", data)
	}
	return nil
}

func (c *SystemCollector) addServices(ctx context.Context, state map[string]interface{}) {
	out, err := c.cmd.Run(ctx, "initctl", "-j")
	if err != nil {
		return
	}

	var initData []map[string]interface{}
	if json.Unmarshal(out, &initData) != nil {
		return
	}

	var services []interface{}
	for _, d := range initData {
		pid, ok := d["pid"]
		if !ok {
			continue
		}
		identity, ok := d["identity"]
		if !ok {
			continue
		}
		svc := map[string]interface{}{
			"pid":         toInt(pid),
			"name":        identity,
			"status":      d["status"],
			"description": d["description"],
			"statistics": map[string]interface{}{
				"memory-usage":  strconv.Itoa(toInt(zeroIfNil(d["memory"]))),
				"uptime":        strconv.Itoa(toInt(zeroIfNil(d["uptime"]))),
				"restart-count": toInt(zeroIfNil(d["restarts"])),
			},
		}
		services = append(services, svc)
	}

	state["infix-system:services"] = map[string]interface{}{
		"service": services,
	}
}

func yangDateTime(t time.Time) string {
	return t.Format("2006-01-02T15:04:05-07:00")
}

func toInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	default:
		return 0
	}
}

func zeroIfNil(v interface{}) interface{} {
	if v == nil {
		return 0
	}
	return v
}
