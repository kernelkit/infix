package collector

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"strings"
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

// Collect implements Collector.  It merges DNS and service data into
// "ietf-system:system-state".  Other system-state subtrees (platform,
// software, users, hostname, timezone, clock, memory, load, filesystems)
// are populated by boot-once, reactive, or on-demand providers.
func (c *SystemCollector) Collect(ctx context.Context, t *tree.Tree) error {
	state := make(map[string]interface{})

	c.addDNS(ctx, state)
	c.addServices(ctx, state)

	if data, err := json.Marshal(state); err == nil {
		t.Merge("ietf-system:system-state", data)
	}
	return nil
}

func (c *SystemCollector) addDNS(ctx context.Context, state map[string]interface{}) {
	dns := make(map[string]interface{})
	servers := make([]interface{}, 0)
	var search []string
	options := make(map[string]interface{})

	headData, err := c.fs.ReadFile("/etc/resolv.conf.head")
	if err == nil {
		for _, line := range strings.Split(string(headData), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "nameserver") {
				ip := strings.TrimSpace(strings.TrimPrefix(line, "nameserver"))
				if net.ParseIP(ip) != nil {
					servers = append(servers, map[string]interface{}{
						"address": ip,
						"origin":  "static",
					})
				}
			} else if strings.HasPrefix(line, "search") {
				search = append(search, strings.Fields(line)[1:]...)
			} else if strings.HasPrefix(line, "options") {
				for _, opt := range strings.Fields(line)[1:] {
					if strings.HasPrefix(opt, "timeout:") {
						if v, err := strconv.Atoi(strings.TrimPrefix(opt, "timeout:")); err == nil {
							options["timeout"] = v
						}
					} else if strings.HasPrefix(opt, "attempts:") {
						if v, err := strconv.Atoi(strings.TrimPrefix(opt, "attempts:")); err == nil {
							options["attempts"] = v
						}
					}
				}
			}
		}
	}

	resolvOut, err := c.cmd.Run(ctx, "/sbin/resolvconf", "-l")
	if err == nil {
		for _, line := range strings.Split(string(resolvOut), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "nameserver") {
				hashParts := strings.SplitN(line, "#", 2)
				ip := strings.TrimSpace(strings.TrimPrefix(hashParts[0], "nameserver"))
				if net.ParseIP(ip) == nil {
					continue
				}
				entry := map[string]interface{}{
					"address": ip,
					"origin":  "dhcp",
				}
				if len(hashParts) > 1 {
					entry["interface"] = strings.TrimSpace(hashParts[1])
				}
				servers = append(servers, entry)
			} else if strings.HasPrefix(line, "search") {
				hashParts := strings.SplitN(line, "#", 2)
				search = append(search, strings.Fields(hashParts[0])[1:]...)
			}
		}
	}

	if len(options) > 0 {
		dns["options"] = options
	}
	dns["server"] = servers
	if len(search) > 0 {
		dns["search"] = search
	}

	state["infix-system:dns-resolver"] = dns
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
