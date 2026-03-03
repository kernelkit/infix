// SPDX-License-Identifier: MIT
package main

import (
	"log"
	"os/exec"
	"sort"
	"strings"
)

// Service represents an mDNS service discovered on the network.
type Service struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	URL   string `json:"url"`
	Other bool   `json:"other"`
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
// services grouped by link (hostname), sorted per host.
func scan() map[string][]Service {
	args := "-tarp"
	if hasK() {
		args += "k"
	}

	out, err := exec.Command("avahi-browse", args).Output()
	if err != nil {
		log.Printf("avahi-browse: %v", err)
		return nil
	}

	hosts := make(map[string][]Service)
	vvHosts   := make(map[string]bool) // has vv=1 TXT record
	legHosts  := make(map[string]bool) // has on=Infix TXT record (legacy)
	mgmtHosts := make(map[string]bool) // has at least one management service type

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
		other := !known
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

		// Parse TXT records
		var path, adminurl string
		for _, record := range strings.Split(txt, " ") {
			stripped := strings.Trim(record, "\"")
			switch {
			case stripped == "vv=1":
				vvHosts[link] = true
			case stripped == "on=Infix":
				legHosts[link] = true
			case path == "" && strings.Contains(stripped, "path="):
				path = stripped[strings.LastIndex(stripped, "path=")+5:]
			case adminurl == "" && strings.Contains(stripped, "adminurl="):
				adminurl = stripped[strings.LastIndex(stripped, "adminurl=")+9:]
			}
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
			Type:  displayName,
			Name:  decode(serviceName),
			URL:   url,
			Other: other,
		}

		// Deduplicate
		dup := false
		for _, existing := range hosts[link] {
			if existing.Type == svc.Type && existing.Name == svc.Name && existing.URL == svc.URL {
				dup = true
				break
			}
		}
		if !dup {
			hosts[link] = append(hosts[link], svc)
		}
	}

	// Sort services per host
	for link := range hosts {
		sort.SliceStable(hosts[link], func(i, j int) bool {
			oi := typeOrder[hosts[link][i].Type]
			oj := typeOrder[hosts[link][j].Type]
			if oi == 0 {
				oi = 999
			}
			if oj == 0 {
				oj = 999
			}
			return oi < oj
		})
	}

	// Default view shows only Infix devices. A host qualifies if it has
	// vv=1 on a management service (to exclude Apple AirPlay collisions),
	// or on=Infix for older firmware that predates vv=1.
	for link := range hosts {
		if len(hosts[link]) > 0 {
			isInfix := (vvHosts[link] && mgmtHosts[link]) || legHosts[link]
			hosts[link][0].Other = !isInfix
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
