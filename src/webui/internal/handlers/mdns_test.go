// SPDX-License-Identifier: MIT

package handlers

import "testing"

func TestMDNSWebURL(t *testing.T) {
	cases := []struct {
		name string
		txt  []string
		addr string
		host string
		port uint16
		want string
	}{
		{"synthesized", nil, "10.0.0.5", "sw.local", 8080, "https://10.0.0.5:8080"},
		{"ipv6 bracketed", nil, "fe80::1", "sw.local", 443, "https://[fe80::1]:443"},
		{"path from txt", []string{"path=/admin"}, "10.0.0.5", "sw.local", 443, "https://10.0.0.5:443/admin"},
		{"path cannot change host", []string{"path=@evil.example/"}, "10.0.0.5", "sw.local", 443, "https://10.0.0.5:443/@evil.example/"},
		{"adminurl rebased", []string{"adminurl=https://stale.local:8443/ui"}, "10.0.0.5", "sw.local", 443, "https://sw.local:8443/ui"},
		{"adminurl javascript ignored", []string{"adminurl=javascript:alert(1)"}, "10.0.0.5", "sw.local", 443, "https://10.0.0.5:443"},
		{"adminurl data ignored", []string{"adminurl=data:text/html,hi"}, "10.0.0.5", "sw.local", 443, "https://10.0.0.5:443"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mdnsWebURL("https", mdnsParseTxt(c.txt), c.addr, c.host, c.port)
			if got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}
