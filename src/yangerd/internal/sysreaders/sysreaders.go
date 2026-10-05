package sysreaders

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

const (
	resolvHead     = "/etc/resolv.conf.head"
	resolvIfaceDir = "/run/resolvconf/interfaces"
)

// ReadDNSResolver reports the configured resolvers: the static ones
// from resolv.conf.head, and the ones DHCP clients handed to resolvconf,
// one file per interface, with the interface each came from.
func ReadDNSResolver(_ string) (json.RawMessage, error) {
	return readDNSResolver(resolvHead, resolvIfaceDir)
}

func readDNSResolver(head, ifaceDir string) (json.RawMessage, error) {
	r := resolver{servers: []interface{}{}, options: map[string]interface{}{}, seen: map[string]bool{}}

	if data, err := os.ReadFile(head); err == nil {
		r.parse(string(data), "static", "")
	}

	files, _ := filepath.Glob(filepath.Join(ifaceDir, "*"))
	sort.Strings(files)
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		r.parse(string(data), "dhcp", resolvconfIface(file))
	}

	dns := map[string]interface{}{"server": r.servers}
	if len(r.search) > 0 {
		dns["search"] = r.search
	}
	if len(r.options) > 0 {
		dns["options"] = r.options
	}

	return json.Marshal(map[string]interface{}{"infix-system:dns-resolver": dns})
}

// resolvconfIface names the interface of a resolvconf file, written by
// the DHCP client scripts as <ifname>.conf or <ifname>-ipv6.conf.
func resolvconfIface(file string) string {
	name := strings.TrimSuffix(filepath.Base(file), ".conf")
	return strings.TrimSuffix(name, "-ipv6")
}

type resolver struct {
	servers []interface{}
	search  []string
	options map[string]interface{}
	seen    map[string]bool
}

// parse reads resolv.conf syntax.  The DHCP scripts tag each line with
// "# <ifname>", which names the interface when present.
func (r *resolver) parse(data, origin, iface string) {
	for _, line := range strings.Split(data, "\n") {
		line, comment, _ := strings.Cut(line, "#")
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		switch fields[0] {
		case "nameserver":
			addr, err := netip.ParseAddr(fields[1])
			if err != nil || r.seen[addr.String()] {
				continue
			}
			r.seen[addr.String()] = true
			server := map[string]interface{}{
				"address": addr.String(),
				"origin":  origin,
			}
			if name := strings.TrimSpace(comment); name != "" && origin == "dhcp" {
				server["interface"] = name
			} else if iface != "" {
				server["interface"] = iface
			}
			r.servers = append(r.servers, server)
		case "search":
			r.search = append(r.search, fields[1:]...)
		case "options":
			for _, opt := range fields[1:] {
				key, val, ok := strings.Cut(opt, ":")
				if !ok || (key != "timeout" && key != "attempts") {
					continue
				}
				if v, err := strconv.Atoi(val); err == nil {
					r.options[key] = v
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
