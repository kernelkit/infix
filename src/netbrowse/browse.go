// SPDX-License-Identifier: MIT
package main

import (
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

		family := parts[2]
		serviceName := parts[3]
		serviceType := parts[4]
		link := parts[6]
		address := parts[7]
		port := parts[8]
		txt := parts[9]

		if family != "IPv4" && family != "IPv6" {
			continue
		}

		info, known := knownServices[serviceType]
		displayName := info.displayName
		urlTemplate := info.urlTemplate
		if !known {
			displayName = serviceType
		}

		// vv=1 is the platform marker set by confd/services.c, survives OS
		// rebranding. We require it together with a management service type
		// (ssh, web, netconf, restconf) to avoid false positives from Apple
		// devices, which also use vv=1 in their AirPlay/RAOP TXT records.
		// on=Infix is kept as a fallback for older firmware predating vv=1.
		if known {
			mgmtHosts[link] = true
		}

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
		var path, adminurl, product, version string
		for _, record := range strings.Split(txt, "\" \"") {
			stripped := strings.Trim(record, "\"")
			switch {
			case stripped == "vv=1":
				vvHosts[link] = true
			case stripped == "on=Infix":
				legHosts[link] = true
			case path == "" && strings.HasPrefix(stripped, "path="):
				path = stripped[5:]
			case adminurl == "" && strings.HasPrefix(stripped, "adminurl="):
				adminurl = stripped[9:]
			case product == "" && strings.HasPrefix(stripped, "product="):
				product = stripped[8:]
			case version == "" && strings.HasPrefix(stripped, "ov="):
				version = stripped[3:]
			}
		}
		// IPP/Bonjour printers encode product as "(Name)" — strip the parens.
		product = strings.TrimPrefix(strings.TrimSuffix(product, ")"), "(")
		if product != "" && productMap[link] == "" {
			productMap[link] = product
		}
		if version != "" && versionMap[link] == "" {
			versionMap[link] = version
		}

		var url string
		if adminurl != "" {
			url = adminurl
		} else if urlTemplate != "" {
			url = strings.NewReplacer(
				"{address}", address,
				"{port}", port,
				"{path}", path,
			).Replace(urlTemplate)
		}

		svc := Service{
			Type: displayName,
			Name: decode(serviceName),
			URL:  url,
		}

		// Deduplicate
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

	// Sort services per host
	for link := range svcsMap {
		sort.SliceStable(svcsMap[link], func(i, j int) bool {
			oi := typeOrder[svcsMap[link][i].Type]
			oj := typeOrder[svcsMap[link][j].Type]
			if oi == 0 {
				oi = 999
			}
			if oj == 0 {
				oj = 999
			}
			return oi < oj
		})
	}

	// Build final host map. Default view shows only Infix devices: a host
	// qualifies if it has vv=1 on a management service (to exclude Apple
	// AirPlay collisions), or on=Infix for older firmware predating vv=1.
	hosts := make(map[string]Host)
	for link, svcs := range svcsMap {
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
