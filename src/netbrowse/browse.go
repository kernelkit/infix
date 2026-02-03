// SPDX-License-Identifier: MIT
package main

import (
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strings"
)

// Service represents an mDNS service discovered on the network.
type Service struct {
	Type  string
	Name  string
	URL   string
	Other bool
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

		// Parse TXT records for path= and adminurl=
		var path, adminurl string
		for _, record := range strings.Split(txt, " ") {
			stripped := strings.Trim(record, "\"")
			if strings.Contains(stripped, "path=") {
				path = stripped[strings.LastIndex(stripped, "path=")+5:]
				break
			}
			if strings.Contains(stripped, "adminurl=") {
				adminurl = stripped[strings.LastIndex(stripped, "adminurl=")+9:]
				break
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

	return hosts
}

// decode handles avahi escape sequences in service names.
func decode(name string) string {
	name = strings.ReplaceAll(name, `\032`, " ")
	name = strings.ReplaceAll(name, `\040`, "(")
	name = strings.ReplaceAll(name, `\041`, ")")

	// Handle remaining \NNN octal escapes
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		if i+3 < len(name) && name[i] == '\\' &&
			name[i+1] >= '0' && name[i+1] <= '3' &&
			name[i+2] >= '0' && name[i+2] <= '7' &&
			name[i+3] >= '0' && name[i+3] <= '7' {
			val := (int(name[i+1]-'0') << 6) | (int(name[i+2]-'0') << 3) | int(name[i+3]-'0')
			b.WriteByte(byte(val))
			i += 3
		} else {
			b.WriteByte(name[i])
		}
	}
	return fmt.Sprintf("%s", b.String())
}
