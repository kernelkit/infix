// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strings"
)

// Service represents a single mDNS service advertised by a host.
type Service struct {
	Type string `json:"type"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Host groups all services for one mDNS host with its metadata.
type Host struct {
	Addr    string    `json:"addr"`
	Product string    `json:"product,omitempty"`
	Version string    `json:"version,omitempty"`
	Other   bool      `json:"other"`
	Svcs    []Service `json:"svcs"`
}

type serviceInfo struct {
	displayName string
	urlTemplate string // empty means no URL
}

var knownServices = map[string]serviceInfo{
	"_http._tcp":         {"HTTP", "http://{address}:{port}{path}"},
	"_https._tcp":        {"HTTPS", "https://{address}:{port}{path}"},
	"_netconf-ssh._tcp":  {"NETCONF", ""},
	"_restconf-tls._tcp": {"RESTCONF", ""},
	"_ssh._tcp":          {"SSH", ""},
	"_sftp-ssh._tcp":     {"SFTP", ""},
}

var typeOrder = map[string]int{
	"HTTPS": 1, "HTTP": 2, "SSH": 3, "SFTP": 4,
}

// txtMeta holds fields extracted from a set of DNS-SD TXT records.
type txtMeta struct {
	vv1      bool
	legacy   bool
	path     string
	adminurl string
	product  string
	version  string
}

// parseTxt extracts well-known fields from a slice of individual TXT record
// strings. Each element must already be a single record without quoting.
// avahi-browse \DDD escape sequences in values are resolved via decode().
func parseTxt(records []string) txtMeta {
	var m txtMeta
	for _, r := range records {
		switch {
		case r == "vv=1":
			m.vv1 = true
		case r == "on=Infix":
			m.legacy = true
		case m.path == "" && strings.HasPrefix(r, "path="):
			m.path = decode(r[5:])
		case m.adminurl == "" && strings.HasPrefix(r, "adminurl="):
			m.adminurl = decode(r[9:])
		case m.product == "" && strings.HasPrefix(r, "product="):
			m.product = decode(r[8:])
		case m.product == "" && strings.HasPrefix(r, "am="):
			// RAOP/AirPlay 1 Apple model key
			m.product = decode(r[3:])
		case m.product == "" && strings.HasPrefix(r, "model="):
			// AirPlay 2 Apple model key
			m.product = decode(r[6:])
		case m.version == "" && strings.HasPrefix(r, "ov="):
			m.version = decode(r[3:])
		}
	}
	return m
}

// buildHosts sorts services and assembles the final host map from the
// per-host accumulator maps that both scan() and scanOperational() maintain.
func buildHosts(svcsMap map[string][]Service, addrMap, productMap, versionMap map[string]string,
	vvHosts, legHosts, mgmtHosts map[string]bool) map[string]Host {
	hosts := make(map[string]Host)
	for link, svcs := range svcsMap {
		sort.SliceStable(svcs, func(i, j int) bool {
			oi, oj := typeOrder[svcs[i].Type], typeOrder[svcs[j].Type]
			if oi == 0 {
				oi = 999
			}
			if oj == 0 {
				oj = 999
			}
			return oi < oj
		})
		isInfix := (vvHosts[link] && mgmtHosts[link]) || legHosts[link]
		hosts[link] = Host{
			Addr:    addrMap[link],
			Product: productMap[link],
			Version: versionMap[link],
			Other:   !isInfix,
			Svcs:    svcs,
		}
	}
	return hosts
}

// hasK checks whether avahi-browse supports the -k flag.
func hasK() bool {
	out, err := exec.Command("avahi-browse", "--help").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "-k")
}

// scan runs avahi-browse, parses the output, and returns discovered
// hosts with their services and metadata.
func scan() map[string]Host {
	args := "-tarp"
	if hasK() {
		args += "k"
	}

	out, err := exec.Command("avahi-browse", args).Output()
	if err != nil {
		log.Printf("avahi-browse: %v", err)
		return nil
	}

	svcsMap    := make(map[string][]Service)
	addrMap    := make(map[string]string) // preferred IP per host (IPv4 wins)
	productMap := make(map[string]string)
	versionMap := make(map[string]string)
	vvHosts    := make(map[string]bool) // has vv=1 TXT record
	legHosts   := make(map[string]bool) // has on=Infix TXT record (legacy)
	mgmtHosts  := make(map[string]bool) // has at least one management service type

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ";")
		if len(parts) <= 9 || parts[0] != "=" {
			continue
		}

		family      := parts[2]
		serviceName := parts[3]
		serviceType := parts[4]
		link        := parts[6]
		address     := parts[7]
		port        := parts[8]
		txt         := strings.Join(parts[9:], ";")

		if family != "IPv4" && family != "IPv6" {
			continue
		}

		info, known := knownServices[serviceType]
		if known {
			mgmtHosts[link] = true
		}

		// vv=1 is the platform marker set by confd/services.c, survives OS
		// rebranding. We require it together with a management service type
		// (ssh, web, netconf, restconf) to avoid false positives from Apple
		// devices, which also use vv=1 in their AirPlay/RAOP TXT records.
		// on=Infix is kept as a fallback for older firmware predating vv=1.

		// Prefer real IPv4; skip loopback and link-local.
		// Loopback (127.x / ::1) appears when avahi resolves local-machine
		// services from the same host — the address is useless for display.
		isLoopback  := address == "127.0.0.1" || address == "::1" || strings.HasPrefix(address, "127.")
		isLinkLocal := strings.HasPrefix(address, "fe80:")
		if !isLoopback && !isLinkLocal {
			if family == "IPv4" {
				addrMap[link] = address // IPv4 always wins
			} else if addrMap[link] == "" {
				addrMap[link] = address // IPv6 fallback
			}
		}

		// Parse TXT records.
		// avahi-browse -p quotes records containing spaces, e.g.
		//   "ty=Brother DCP-L3550CDW series" "adminurl=http://..."
		// Split on the between-record boundary `" "` (close-quote space
		// open-quote) to keep each record intact, then trim outer quotes.
		var records []string
		for _, rec := range strings.Split(txt, "\" \"") {
			records = append(records, strings.Trim(rec, "\""))
		}
		meta := parseTxt(records)
		if meta.vv1 {
			vvHosts[link] = true
		}
		if meta.legacy {
			legHosts[link] = true
		}

		// IPP/Bonjour printers encode product as "(Name)" — strip the parens.
		product := strings.TrimPrefix(strings.TrimSuffix(meta.product, ")"), "(")
		if product != "" && productMap[link] == "" {
			productMap[link] = product
		}
		if meta.version != "" && versionMap[link] == "" {
			versionMap[link] = meta.version
		}

		displayName := info.displayName
		if !known {
			displayName = serviceType
		}

		var url string
		if meta.adminurl != "" {
			url = meta.adminurl
		} else if info.urlTemplate != "" {
			url = strings.NewReplacer(
				"{address}", address,
				"{port}", port,
				"{path}", meta.path,
			).Replace(info.urlTemplate)
		}

		svc := Service{
			Type: displayName,
			Name: decode(serviceName),
			URL:  url,
		}

		// Deduplicate: avahi-browse reports each service once per
		// (interface, protocol) combination, so the same entry can appear
		// for both eth0/IPv4 and eth0/IPv6.
		dup := false
		for _, existing := range svcsMap[link] {
			if existing.Type == svc.Type && existing.Name == svc.Name && existing.URL == svc.URL {
				dup = true
				break
			}
		}
		if !dup {
			svcsMap[link] = append(svcsMap[link], svc)
		}
	}

	return buildHosts(svcsMap, addrMap, productMap, versionMap, vvHosts, legHosts, mgmtHosts)
}

// JSON types for the operational-state backend.
type opRoot struct {
	MDNS struct {
		Enabled   bool `json:"enabled"`
		Neighbors struct {
			Neighbor []opNeighbor `json:"neighbor"`
		} `json:"neighbors"`
	} `json:"infix-services:mdns"`
}

type opNeighbor struct {
	Hostname  string      `json:"hostname"`
	Addresses []string    `json:"address"`
	Services  []opService `json:"service"`
}

type opService struct {
	Name string   `json:"name"`
	Type string   `json:"type"`
	Port uint16   `json:"port"`
	Txt  []string `json:"txt"`
}

// parseOperational builds a host map from an already-decoded opRoot.
func parseOperational(root *opRoot) map[string]Host {
	svcsMap    := make(map[string][]Service)
	addrMap    := make(map[string]string)
	productMap := make(map[string]string)
	versionMap := make(map[string]string)
	vvHosts    := make(map[string]bool)
	legHosts   := make(map[string]bool)
	mgmtHosts  := make(map[string]bool)

	for _, n := range root.MDNS.Neighbors.Neighbor {
		link := n.Hostname

		// Pick best address: prefer IPv4, skip link-local.
		// Loopback is already excluded by statd before storing.
		for _, a := range n.Addresses {
			if strings.HasPrefix(a, "fe80:") {
				continue
			}
			if !strings.Contains(a, ":") {
				addrMap[link] = a // IPv4 wins, stop looking
				break
			}
			if addrMap[link] == "" {
				addrMap[link] = a // IPv6 fallback
			}
		}

		for _, svc := range n.Services {
			info, known := knownServices[svc.Type]
			if known {
				mgmtHosts[link] = true
			}

			meta := parseTxt(svc.Txt)
			if meta.vv1 {
				vvHosts[link] = true
			}
			if meta.legacy {
				legHosts[link] = true
			}

			// IPP/Bonjour printers encode product as "(Name)" — strip the parens.
			product := strings.TrimPrefix(strings.TrimSuffix(meta.product, ")"), "(")
			if product != "" && productMap[link] == "" {
				productMap[link] = product
			}
			if meta.version != "" && versionMap[link] == "" {
				versionMap[link] = meta.version
			}

			displayName := info.displayName
			if !known {
				displayName = svc.Type
			}

			addr := addrMap[link]
			if addr == "" {
				addr = n.Hostname // fall back to hostname if no usable address
			}

			var url string
			if meta.adminurl != "" {
				url = meta.adminurl
			} else if info.urlTemplate != "" {
				url = strings.NewReplacer(
					"{address}", addr,
					"{port}", fmt.Sprintf("%d", svc.Port),
					"{path}", meta.path,
				).Replace(info.urlTemplate)
			}

			svcsMap[link] = append(svcsMap[link], Service{
				Type: displayName,
				Name: svc.Name,
				URL:  url,
			})
		}
	}

	return buildHosts(svcsMap, addrMap, productMap, versionMap, vvHosts, legHosts, mgmtHosts)
}

// fetchOpRoot runs `copy operational-state -x /mdns` and decodes the result.
// Returns nil on any error (command failure or JSON parse error).
func fetchOpRoot() *opRoot {
	out, err := exec.Command("copy", "operational-state", "-x", "/mdns").Output()
	if err != nil {
		log.Printf("copy operational-state: %v", err)
		return nil
	}
	var root opRoot
	if err := json.Unmarshal(out, &root); err != nil {
		log.Printf("copy operational-state: json: %v", err)
		return nil
	}
	return &root
}

// scanOperational fetches the mDNS neighbor table from the sysrepo
// operational datastore via `copy operational-state -x /mdns` and returns
// the same host map as scan().
func scanOperational() map[string]Host {
	root := fetchOpRoot()
	if root == nil {
		return nil
	}
	return parseOperational(root)
}

// scanAuto tries the operational backend first.  If the `copy` command is
// unavailable or mDNS is disabled in the operational state, it falls back
// to the avahi-browse backend transparently.
func scanAuto() map[string]Host {
	root := fetchOpRoot()
	if root == nil || !root.MDNS.Enabled {
		return scan()
	}
	return parseOperational(root)
}

// decode handles avahi's DNS-SD escape sequences in service names:
//   - \DDD  decimal value 0-255, e.g. \058 → ':'
//   - \X    literal character X, e.g. \. → '.'
func decode(name string) string {
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		if name[i] != '\\' || i+1 >= len(name) {
			b.WriteByte(name[i])
			continue
		}
		if i+3 < len(name) &&
			name[i+1] >= '0' && name[i+1] <= '9' &&
			name[i+2] >= '0' && name[i+2] <= '9' &&
			name[i+3] >= '0' && name[i+3] <= '9' {
			val := int(name[i+1]-'0')*100 + int(name[i+2]-'0')*10 + int(name[i+3]-'0')
			if val <= 255 {
				b.WriteByte(byte(val))
				i += 3
				continue
			}
		}
		// \X where X is not a 3-digit decimal: output X literally
		b.WriteByte(name[i+1])
		i++
	}
	return b.String()
}
