package sysreaders

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var gmtOffsetRe = regexp.MustCompile(`Etc/GMT([+-]\d{1,2})$`)

var zonePrefixes = []string{
	"/usr/share/zoneinfo/posix/",
	"/usr/share/zoneinfo/right/",
	"/usr/share/zoneinfo/",
}

var userShellMap = map[string]string{
	"/bin/bash":         "infix-system:bash",
	"/bin/sh":           "infix-system:sh",
	"/usr/bin/clish":    "infix-system:clish",
	"/bin/false":        "infix-system:false",
	"/sbin/nologin":     "infix-system:false",
	"/usr/sbin/nologin": "infix-system:false",
}

const SSHDKeysDir = "/var/run/sshd"

func ReadHostname(path string) (json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(string(data))
	return json.Marshal(map[string]string{"hostname": name})
}

func ReadTimezone(path string) (json.RawMessage, error) {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}

	var tz string
	for _, p := range zonePrefixes {
		if strings.HasPrefix(target, p) {
			tz = target[len(p):]
			break
		}
	}
	if tz == "" {
		return nil, fmt.Errorf("unrecognized zoneinfo path: %s", target)
	}

	clock := make(map[string]interface{})
	if m := gmtOffsetRe.FindStringSubmatch(tz); m != nil {
		offset, _ := strconv.Atoi(m[1])
		clock["timezone-utc-offset"] = -offset
	} else if tz == "Etc/UTC" {
		clock["timezone-utc-offset"] = 0
	} else {
		clock["timezone-name"] = tz
	}

	return json.Marshal(map[string]interface{}{"clock": clock})
}

func ReadUsers(_ string) (json.RawMessage, error) {
	passwdData, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return nil, err
	}

	passwdUsers := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(passwdData))
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ":")
		if len(parts) < 7 {
			continue
		}
		uid, err := strconv.Atoi(parts[2])
		if err != nil || uid < 1000 || uid >= 10000 {
			continue
		}
		shell := strings.TrimSpace(parts[6])
		mapped, ok := userShellMap[shell]
		if !ok {
			mapped = "infix-system:false"
		}
		passwdUsers[parts[0]] = mapped
	}

	shadowHashes := make(map[string]string)
	shadowData, err := os.ReadFile("/etc/shadow")
	if err == nil {
		scanner = bufio.NewScanner(bytes.NewReader(shadowData))
		for scanner.Scan() {
			parts := strings.SplitN(scanner.Text(), ":", 3)
			if len(parts) < 2 {
				continue
			}
			hash := parts[1]
			if hash == "" || strings.HasPrefix(hash, "*") || strings.HasPrefix(hash, "!") {
				continue
			}
			shadowHashes[parts[0]] = hash
		}
	}

	users := make([]interface{}, 0)
	for username, shell := range passwdUsers {
		user := map[string]interface{}{
			"name":               username,
			"infix-system:shell": shell,
		}
		if hash, ok := shadowHashes[username]; ok {
			user["password"] = hash
		}

		keysData, err := os.ReadFile(filepath.Join(SSHDKeysDir, username+".keys"))
		if err == nil {
			var authKeys []interface{}
			for _, line := range strings.Split(string(keysData), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, " ", 3)
				if len(parts) < 2 {
					continue
				}
				keyName := fmt.Sprintf("%s-key-%d", username, len(authKeys))
				if len(parts) > 2 {
					keyName = parts[2]
				}
				authKeys = append(authKeys, map[string]interface{}{
					"name":      keyName,
					"algorithm": parts[0],
					"key-data":  parts[1],
				})
			}
			if len(authKeys) > 0 {
				user["authorized-key"] = authKeys
			}
		}
		users = append(users, user)
	}

	return json.Marshal(map[string]interface{}{
		"authentication": map[string]interface{}{
			"user": users,
		},
	})
}

func ReadDNSResolver(_ string) (json.RawMessage, error) {
	servers := make([]interface{}, 0)
	var search []string
	options := make(map[string]interface{})
	seen := make(map[string]bool)

	for _, path := range []string{"/etc/resolv.conf.head", "/var/lib/misc/resolv.conf"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		ParseResolvConf(string(data), &servers, &search, options, seen)
	}

	dns := make(map[string]interface{})
	dns["server"] = servers
	if len(search) > 0 {
		dns["search"] = search
	}
	if len(options) > 0 {
		dns["options"] = options
	}

	return json.Marshal(map[string]interface{}{"infix-system:dns-resolver": dns})
}

func ParseResolvConf(data string, servers *[]interface{}, search *[]string, options map[string]interface{}, seen map[string]bool) {
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "nameserver"):
			ip := strings.TrimSpace(strings.TrimPrefix(line, "nameserver"))
			if ip != "" && ip != "127.0.0.1" && ip != "::1" && !seen[ip] {
				seen[ip] = true
				*servers = append(*servers, map[string]interface{}{
					"address": ip,
				})
			}
		case strings.HasPrefix(line, "search"):
			*search = append(*search, strings.Fields(line)[1:]...)
		case strings.HasPrefix(line, "options"):
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

// ForwardingAggregator tracks all /proc/sys/net/ipv{4,6}/conf/*/forwarding
// files and rebuilds the complete interfaces list on every change.
type ForwardingAggregator struct {
	mu sync.Mutex
}

func NewForwardingAggregator() *ForwardingAggregator {
	return &ForwardingAggregator{}
}

func (fa *ForwardingAggregator) HandleForwardingChange(_ string) (json.RawMessage, error) {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	enabled := make(map[string]bool)

	for _, family := range []string{"ipv4", "ipv6"} {
		sysctl := "forwarding"
		if family == "ipv6" {
			sysctl = "force_forwarding"
		}
		pattern := fmt.Sprintf("/proc/sys/net/%s/conf/*/%s", family, sysctl)
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, path := range matches {
			b, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if strings.TrimSpace(string(b)) != "1" {
				continue
			}
			parts := strings.Split(filepath.Clean(path), string(os.PathSeparator))
			if len(parts) >= 7 {
				ifname := parts[len(parts)-2]
				if ifname != "all" && ifname != "default" && ifname != "lo" {
					enabled[ifname] = true
				}
			}
		}
	}

	ifnames := make([]string, 0)
	for name := range enabled {
		ifnames = append(ifnames, name)
	}

	data, err := json.Marshal(map[string]interface{}{
		"interfaces": map[string]interface{}{
			"interface": ifnames,
		},
	})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}
