// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"infix/webui/internal/schema"
	"infix/webui/internal/testutil"
)

// The reference shapes of the real models: leaf-lists, list keys, plain
// leaves, and mandatory leaves both on an interface and in a list entry.
var ifaceRefSchemaYang = map[string]string{
	"ietf-interfaces.yang": `module ietf-interfaces {
  yang-version 1.1; namespace "urn:test:if"; prefix if;
  typedef interface-ref { type leafref { path "/if:interfaces/if:interface/if:name"; } }
  container interfaces { list interface { key name; leaf name { type string; } leaf type { type string; } } }
}`,
	"infix-interfaces.yang": `module infix-interfaces {
  yang-version 1.1; namespace "urn:test:infix-if"; prefix infix-if;
  import ietf-interfaces { prefix if; }
  augment "/if:interfaces/if:interface" {
    container bridge-port { leaf bridge { type if:interface-ref; mandatory true; } leaf pvid { type uint16; } }
    container lag-port { leaf lag { type if:interface-ref; mandatory true; } }
    container vlan { leaf lower-layer-if { type if:interface-ref; mandatory true; } leaf id { type uint16; } }
    container veth { leaf peer { type if:interface-ref; mandatory true; } }
    container bridge { presence bridge;
      container vlans { list vlan { key vid; leaf vid { type uint16; }
        leaf-list untagged { type if:interface-ref; } leaf-list tagged { type if:interface-ref; } } }
      container multicast-filters { list multicast-filter { key group; leaf group { type string; }
        list ports { key port; leaf port { type if:interface-ref; } } } }
      container forwarding-database { list entry { key "address vid";
        leaf address { type string; } leaf vid { type uint16; } leaf port { type if:interface-ref; mandatory true; } } }
    }
  }
}`,
	"infix-firewall.yang": `module infix-firewall {
  yang-version 1.1; namespace "urn:test:fw"; prefix fw;
  import ietf-interfaces { prefix if; }
  container firewall { list zone { key name; leaf name { type string; } leaf-list interface { type if:interface-ref; } } }
}`,
	"infix-dhcp-server.yang": `module infix-dhcp-server {
  yang-version 1.1; namespace "urn:test:dhcp"; prefix dhcp;
  import ietf-interfaces { prefix if; }
  container dhcp-server { list subnet { key subnet; leaf subnet { type string; } leaf if-name { type if:interface-ref; } } }
}`,
	"ieee802-dot1ab-lldp.yang": `module ieee802-dot1ab-lldp {
  yang-version 1.1; namespace "urn:test:lldp"; prefix lldp;
  import ietf-interfaces { prefix if; }
  container lldp { list port { key name; leaf name { type if:interface-ref; } leaf enabled { type boolean; } } }
}`,
}

const ifaceRefCandidate = `{
"ietf-interfaces:interfaces":{"interface":[
 {"name":"br0","type":"bridge","infix-interfaces:bridge":{
   "vlans":{"vlan":[
     {"vid":1,"untagged":["lan1"],"tagged":["br0"]},
     {"vid":99,"untagged":["wds-tauri"],"tagged":["wds-jaffa","wds-tauri","br0"]}]},
   "multicast-filters":{"multicast-filter":[
     {"group":"01:00:5e:01:01:01","ports":[{"port":"lan1"},{"port":"wds-tauri"}]}]},
   "forwarding-database":{"entry":[
     {"address":"00:11:22:33:44:55","vid":99,"port":"wds-tauri"},
     {"address":"00:11:22:33:44:66","vid":1,"port":"lan1"}]}}},
 {"name":"lan1","type":"ethernet","infix-interfaces:bridge-port":{"bridge":"br0","pvid":1}},
 {"name":"lan1.10","type":"vlan","infix-interfaces:vlan":{"lower-layer-if":"lan1","id":10}},
 {"name":"wds-jaffa","type":"wifi","infix-interfaces:bridge-port":{"bridge":"br0"}},
 {"name":"wds-tauri","type":"wifi","infix-interfaces:bridge-port":{"bridge":"br0"}},
 {"name":"veth-a","type":"veth","infix-interfaces:veth":{"peer":"veth-b"}},
 {"name":"veth-b","type":"veth","infix-interfaces:veth":{"peer":"veth-a"}}
]},
"infix-firewall:firewall":{"zone":[{"name":"lan","interface":["lan1","wds-tauri"]}]},
"infix-dhcp-server:dhcp-server":{"subnet":[{"subnet":"10.0.0.0/24","if-name":"wds-tauri"}]},
"ieee802-dot1ab-lldp:lldp":{"port":[{"name":"wds-tauri","enabled":true},{"name":"lan1"}]}
}`

func ifaceRefHandler(t *testing.T) (*ConfigureInterfacesHandler, *testutil.MockFetcher) {
	t.Helper()
	dir := t.TempDir()
	for name, text := range ifaceRefSchemaYang {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c := schema.NewCache(nil, dir, "")
	if err := c.LoadFromCache(); err != nil {
		t.Fatal(err)
	}
	if c.Manager() == nil {
		t.Fatal("schema not loaded")
	}
	rc := testutil.NewMockFetcher()
	rc.Datastores = map[string]json.RawMessage{"candidate": json.RawMessage(ifaceRefCandidate)}
	var wrap any
	if err := json.Unmarshal([]byte(ifaceRefCandidate), &wrap); err != nil {
		t.Fatal(err)
	}
	rc.SetResponse(ifaceCandPath, wrap)
	return &ConfigureInterfacesHandler{RC: rc, Schema: c}, rc
}

func deleteInterface(t *testing.T, h *ConfigureInterfacesHandler, name string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/configure/interfaces/"+name, nil)
	req.SetPathValue("name", name)
	w := httptest.NewRecorder()
	h.DeleteInterface(w, req)
	return w
}

func removedTargets(t *testing.T, rc *testutil.MockFetcher) []string {
	t.Helper()
	if len(rc.Patches) != 1 {
		t.Fatalf("patches %+v", rc.Patches)
	}
	var out []string
	for _, e := range rc.Patches[0].Edits {
		if e.Op != "remove" {
			t.Fatalf("edit %+v", e)
		}
		out = append(out, e.Target)
	}
	return out
}

func TestDeleteInterfaceDropsEveryReference(t *testing.T) {
	h, rc := ifaceRefHandler(t)
	if w := deleteInterface(t, h, "wds-tauri"); w.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	want := []string{
		"/ieee802-dot1ab-lldp:lldp/port=wds-tauri",
		"/ietf-interfaces:interfaces/interface=br0/infix-interfaces:bridge/forwarding-database/entry=00:11:22:33:44:55,99",
		"/ietf-interfaces:interfaces/interface=br0/infix-interfaces:bridge/multicast-filters/multicast-filter=01:00:5e:01:01:01/ports=wds-tauri",
		"/ietf-interfaces:interfaces/interface=br0/infix-interfaces:bridge/vlans/vlan=99/tagged=wds-tauri",
		"/ietf-interfaces:interfaces/interface=br0/infix-interfaces:bridge/vlans/vlan=99/untagged=wds-tauri",
		"/infix-dhcp-server:dhcp-server/subnet=10.0.0.0%2F24/if-name",
		"/infix-firewall:firewall/zone=lan/interface=wds-tauri",
	}
	if got := removedTargets(t, rc); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("removed\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestDeleteBridgeDetachesPorts(t *testing.T) {
	h, rc := ifaceRefHandler(t)
	if w := deleteInterface(t, h, "br0"); w.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	want := []string{
		"/ietf-interfaces:interfaces/interface=lan1/infix-interfaces:bridge-port",
		"/ietf-interfaces:interfaces/interface=wds-jaffa/infix-interfaces:bridge-port",
		"/ietf-interfaces:interfaces/interface=wds-tauri/infix-interfaces:bridge-port",
	}
	if got := removedTargets(t, rc); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("removed\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestDeleteInterfaceUnderVLANIsRefused(t *testing.T) {
	h, rc := ifaceRefHandler(t)
	w := deleteInterface(t, h, "lan1")
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "lan1.10") {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if len(rc.Patches) != 0 {
		t.Fatalf("candidate written despite refusal: %+v", rc.Patches)
	}
}

func TestDeleteVethTakesPeer(t *testing.T) {
	h, rc := ifaceRefHandler(t)
	if w := deleteInterface(t, h, "veth-a"); w.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	want := []string{"/ietf-interfaces:interfaces/interface=veth-b"}
	if got := removedTargets(t, rc); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("removed %v", got)
	}
}

func TestDeleteUnreferencedInterfaceWritesNoPatch(t *testing.T) {
	h, rc := ifaceRefHandler(t)
	if w := deleteInterface(t, h, "lan2"); w.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if len(rc.Patches) != 0 {
		t.Fatalf("patches %+v", rc.Patches)
	}
}
