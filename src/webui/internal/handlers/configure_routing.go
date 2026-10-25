// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"

	"github.com/kernelkit/infix/src/features"
)

// The routing protocols on Configure > Routing: one row per protocol and
// address family, like the CLI has them, with the global settings in a
// fold-out and, for OSPF, one row per area under it.  Dialogs add a
// protocol or an area with the basics, the advanced editor has the rest.

const (
	cppSchemaPath     = "/ietf-routing:routing/control-plane-protocols/control-plane-protocol"
	cppConfigPath     = "/ietf-routing:routing/control-plane-protocols"
	ospfBFDSchemaPath = cppSchemaPath + "/ietf-ospf:ospf/areas/area/interfaces/interface/bfd/enabled"
	ospfAreaTypePath  = cppSchemaPath + "/ietf-ospf:ospf/areas/area/area-type"
)

// ─── RESTCONF JSON types ──────────────────────────────────────────────────────

type rtProtoWrapper struct {
	Routing struct {
		CPPs struct {
			CPP []rtProtoJSON `json:"control-plane-protocol"`
		} `json:"control-plane-protocols"`
	} `json:"ietf-routing:routing"`
}

type rtProtoJSON struct {
	Type         string           `json:"type"`
	Name         string           `json:"name"`
	StaticRoutes staticRoutesJSON `json:"static-routes"`
	OSPF         *cfgOSPFJSON     `json:"ietf-ospf:ospf"`
	RIP          *cfgRIPJSON      `json:"ietf-rip:rip"`
}

type rtRedistJSON struct {
	Redistribute []struct {
		Protocol string `json:"protocol"`
	} `json:"redistribute"`
}

type rtEnabledJSON struct {
	Enabled *bool `json:"enabled"`
}

type cfgOSPFJSON struct {
	RouterID string `json:"explicit-router-id"`
	Areas    struct {
		Area []cfgOSPFAreaJSON `json:"area"`
	} `json:"areas"`
	Redistribute rtRedistJSON  `json:"infix-routing:redistribute"`
	DefaultRoute rtEnabledJSON `json:"infix-routing:default-route-advertise"`
}

type cfgOSPFAreaJSON struct {
	ID         string `json:"area-id"`
	Type       string `json:"area-type"`
	Interfaces struct {
		Interface []cfgOSPFIfaceJSON `json:"interface"`
	} `json:"interfaces"`
}

type cfgOSPFIfaceJSON struct {
	Name string        `json:"name"`
	BFD  rtEnabledJSON `json:"bfd"`
}

type cfgRIPJSON struct {
	Interfaces struct {
		Interface []struct {
			Interface string `json:"interface"`
		} `json:"interface"`
	} `json:"interfaces"`
	Redistribute rtRedistJSON  `json:"infix-routing:redistribute"`
	DefaultRoute rtEnabledJSON `json:"originate-default-route"`
}

// routable reports whether a routing protocol can run on the interface.
// Ports of a bridge or LAG route through it, not on their own.
func routable(i ifaceJSON) bool {
	return (i.BridgePort == nil || i.BridgePort.Bridge == "") && (i.LagPort == nil || i.LagPort.LAG == "")
}

// ifaceChoices lists the routable interfaces, and those already in use.
func ifaceChoices(ifaces []ifaceJSON, used []string) []string {
	var names []string
	for _, i := range ifaces {
		if routable(i) || slices.Contains(used, i.Name) {
			names = append(names, i.Name)
		}
	}
	return names
}

// ─── Protocols ────────────────────────────────────────────────────────────────

// rtProto is one routing protocol for one address family.
type rtProto struct {
	Key    string // "ospfv2", "ospfv3", "ripv2" or "ripng", in forms and URLs
	Label  string
	Family string // "ipv4" or "ipv6"
	Type   string // control-plane-protocol type
	OSPF   bool   // ietf-ospf, else ietf-rip
}

var rtProtos = []rtProto{
	{"ospfv2", "OSPFv2", "ipv4", "infix-routing:ospfv2", true},
	{"ospfv3", "OSPFv3", "ipv6", "infix-routing:ospfv3", true},
	{"ripv2", "RIPv2", "ipv4", "infix-routing:ripv2", false},
	{"ripng", "RIPng", "ipv6", "infix-routing:ripng", false},
}

func protoByKey(key string) (rtProto, bool) {
	for _, p := range rtProtos {
		if p.Key == key {
			return p, true
		}
	}
	return rtProto{}, false
}

// FamilyLabel is "IPv4" or "IPv6".
func (p rtProto) FamilyLabel() string {
	return strings.ToUpper(p.Family[:2]) + p.Family[2:]
}

// instancePath is the RESTCONF path of a protocol instance.
func instancePath(typ, name string) string {
	return cppConfigPath + "/control-plane-protocol=" + restconf.EscapeKey(typ) + "," + restconf.EscapeKey(name)
}

// findInstance returns the instance of a protocol type, or nil.
func findInstance(cpps []rtProtoJSON, typ string) *rtProtoJSON {
	for i := range cpps {
		if cpps[i].Type == typ {
			return &cpps[i]
		}
	}
	return nil
}

func hasRedist(r rtRedistJSON, proto string) bool {
	for _, e := range r.Redistribute {
		if e.Protocol == proto {
			return true
		}
	}
	return false
}

func hasOSPFBFD(mgr *schema.Manager) bool {
	if mgr == nil {
		return false
	}
	_, err := mgr.NodeAt(ospfBFDSchemaPath)
	return err == nil
}

// ─── Setups: what the fold-out shows ──────────────────────────────────────────

// ospfSetup is what the fold-out shows of an OSPF instance.
type ospfSetup struct {
	RouterID     string
	Areas        []ospfAreaSetup
	BFD          bool // on for every interface
	BFDMixed     bool // on for some interfaces only
	Connected    bool
	Static       bool
	DefaultRoute bool
}

type ospfAreaSetup struct {
	ID     string
	Type   string // area type identity without module, e.g. "stub-area"
	Ifaces []string
}

// rtAreaType is an area type the model has, an identity derived from
// ietf-ospf:area-type.
type rtAreaType struct {
	Value    string // the identity, e.g. "ietf-ospf:stub-area"
	Label    string
	Selected bool
}

// areaTypes lists the area types of the model, with the current one, or
// the model's default, selected.
func areaTypes(opts []schema.IdentityOption, current string) []rtAreaType {
	var types []rtAreaType
	for _, o := range opts {
		_, name := splitModPrefix(o.Value)
		types = append(types, rtAreaType{
			Value:    o.Value,
			Label:    areaTypeLabel(o.Value),
			Selected: name == current || (current == "" && o.IsDefault),
		})
	}
	return types
}

// areaTypeLabel names an area type, e.g. "ietf-ospf:nssa-area" is "NSSA".
func areaTypeLabel(identity string) string {
	_, name := splitModPrefix(identity)
	return strings.ReplaceAll(strings.TrimSuffix(name, "-area"), "nssa", "NSSA")
}

func isNormalArea(identity string) bool {
	_, name := splitModPrefix(identity)
	return name == "normal-area"
}

func ospfSetupOf(o *cfgOSPFJSON) ospfSetup {
	s := ospfSetup{
		RouterID:     o.RouterID,
		Connected:    hasRedist(o.Redistribute, "connected"),
		Static:       hasRedist(o.Redistribute, "static"),
		DefaultRoute: o.DefaultRoute.Enabled != nil && *o.DefaultRoute.Enabled,
	}
	on, all := 0, 0
	for _, a := range o.Areas.Area {
		_, typ := splitModPrefix(a.Type)
		area := ospfAreaSetup{ID: a.ID, Type: typ}
		for _, i := range a.Interfaces.Interface {
			area.Ifaces = append(area.Ifaces, i.Name)
			all++
			if i.BFD.Enabled != nil && *i.BFD.Enabled {
				on++
			}
		}
		slices.Sort(area.Ifaces)
		s.Areas = append(s.Areas, area)
	}
	slices.SortFunc(s.Areas, func(a, b ospfAreaSetup) int { return strings.Compare(a.ID, b.ID) })
	s.BFD = all > 0 && on == all
	s.BFDMixed = on > 0 && on < all
	return s
}

// Interfaces lists the interfaces in all areas.
func (s ospfSetup) Interfaces() []string {
	var all []string
	for _, a := range s.Areas {
		all = append(all, a.Ifaces...)
	}
	slices.Sort(all)
	return all
}

// ripSetup is what the fold-out shows of a RIP instance.
type ripSetup struct {
	Ifaces       []string
	IfSet        map[string]bool
	Connected    bool
	Static       bool
	DefaultRoute bool
}

func ripSetupOf(r *cfgRIPJSON) ripSetup {
	s := ripSetup{
		IfSet:        map[string]bool{},
		Connected:    hasRedist(r.Redistribute, "connected"),
		Static:       hasRedist(r.Redistribute, "static"),
		DefaultRoute: r.DefaultRoute.Enabled != nil && *r.DefaultRoute.Enabled,
	}
	for _, i := range r.Interfaces.Interface {
		s.Ifaces = append(s.Ifaces, i.Interface)
		s.IfSet[i.Interface] = true
	}
	slices.Sort(s.Ifaces)
	return s
}

// ─── Template data ────────────────────────────────────────────────────────────

// rtRow is a configured protocol: its row in the table, its fold-out with
// the global settings, and for OSPF the rows of its areas.
type rtRow struct {
	rtProto
	Name      string   // instance name
	TreeNode  string   // advanced editor node of the instance
	Ifaces    []string // interfaces to choose from, for RIP
	BFD       bool     // the model has OSPF BFD
	Desc      map[string]string
	OSPF      *ospfSetup
	Areas     []rtAreaRow
	AreaTypes []rtAreaType    // for a new area
	NewArea   []rtIfaceChoice // interfaces for a new area
	RIP       *ripSetup
}

// rtAreaRow is an OSPF area: its row under the protocol and its fold-out.
type rtAreaRow struct {
	ospfAreaSetup
	Key     string // the protocol's
	Last    bool   // the last area, closes the tree line
	Types   []rtAreaType
	Choices []rtIfaceChoice
	Desc    map[string]string
}

// rtIfaceChoice is an interface an area can have, unless it is in another
// area of the same instance.
type rtIfaceChoice struct {
	Name    string
	Checked bool
	Other   string // the area it is in, if another
}

// Interfaces sums up the interfaces for the protocol's row.
func (r rtRow) Interfaces() string {
	if r.OSPF != nil {
		return strings.Join(r.OSPF.Interfaces(), ", ")
	}
	return strings.Join(r.RIP.Ifaces, ", ")
}

// Label names the area with its type, e.g. "Area 0.0.0.1 (stub)".
func (a rtAreaRow) Label() string {
	if isNormalArea(a.Type) {
		return "Area " + a.ID
	}
	return "Area " + a.ID + " (" + areaTypeLabel(a.Type) + ")"
}

type rtNoForwarding struct {
	Name     string
	Families []string // "ipv4", "ipv6"
}

// areaChoices lists the interfaces for one area: those in it, the free
// ones, and those in another area, which can not be chosen.
func areaChoices(ifaces []ifaceJSON, s *ospfSetup, area string) []rtIfaceChoice {
	in := map[string]string{}
	for _, a := range s.Areas {
		for _, n := range a.Ifaces {
			in[n] = a.ID
		}
	}
	var out []rtIfaceChoice
	for _, name := range ifaceChoices(ifaces, s.Interfaces()) {
		c := rtIfaceChoice{Name: name, Checked: area != "" && in[name] == area}
		if in[name] != "" && in[name] != area {
			c.Other = in[name]
		}
		out = append(out, c)
	}
	return out
}

// routingDescriptions picks the YANG descriptions shown as hover text.
func routingDescriptions(mgr *schema.Manager) map[string]string {
	ospf := cppSchemaPath + "/ietf-ospf:ospf"
	rip := cppSchemaPath + "/ietf-rip:rip"
	return map[string]string{
		"proto-type":        schema.DescriptionOf(mgr, cppSchemaPath+"/type"),
		"router-id":         schema.DescriptionOf(mgr, ospf+"/explicit-router-id"),
		"bfd":               schema.DescriptionOf(mgr, ospfBFDSchemaPath),
		"redistribute":      schema.DescriptionOf(mgr, ospf+"/infix-routing:redistribute/redistribute"),
		"default-route":     schema.DescriptionOf(mgr, ospf+"/infix-routing:default-route-advertise/enabled"),
		"area-id":           schema.DescriptionOf(mgr, ospf+"/areas/area/area-id"),
		"area-type":         schema.DescriptionOf(mgr, ospf+"/areas/area/area-type"),
		"area-ifaces":       schema.DescriptionOf(mgr, ospf+"/areas/area/interfaces/interface"),
		"rip-ifaces":        schema.DescriptionOf(mgr, rip+"/interfaces/interface"),
		"rip-redistribute":  schema.DescriptionOf(mgr, rip+"/infix-routing:redistribute/redistribute"),
		"rip-default-route": schema.DescriptionOf(mgr, rip+"/originate-default-route/enabled"),
	}
}

// buildRoutingProtocols fills the protocol rows and what the add dialog
// offers, for the protocols the device has.
func buildRoutingProtocols(mgr *schema.Manager, cpps []rtProtoJSON, ifaces []ifaceJSON, data *cfgRoutesPageData) {
	_, ospfErr := mgr.NodeAt(cppSchemaPath + "/ietf-ospf:ospf")
	_, ripErr := mgr.NodeAt(cppSchemaPath + "/ietf-rip:rip")
	bfd := hasOSPFBFD(mgr)
	types := schema.OptionsFor(mgr, ospfAreaTypePath)
	desc := routingDescriptions(mgr)
	for k, v := range desc {
		data.Desc[k] = v
	}
	inUse := map[string][]string{} // interface → families routed on it

	for _, p := range rtProtos {
		if (p.OSPF && ospfErr != nil) || (!p.OSPF && ripErr != nil) {
			continue
		}
		inst := findInstance(cpps, p.Type)
		if inst == nil {
			data.Addable = append(data.Addable, p)
			continue
		}
		row := rtRow{rtProto: p, Name: inst.Name, TreeNode: instancePath(inst.Type, inst.Name), BFD: bfd, Desc: desc}
		var used []string
		if p.OSPF {
			o := inst.OSPF
			if o == nil {
				o = &cfgOSPFJSON{}
			}
			s := ospfSetupOf(o)
			row.OSPF = &s
			for i, a := range s.Areas {
				row.Areas = append(row.Areas, rtAreaRow{ospfAreaSetup: a, Key: p.Key, Last: i == len(s.Areas)-1,
					Types: areaTypes(types, a.Type), Choices: areaChoices(ifaces, &s, a.ID), Desc: desc})
			}
			row.AreaTypes = areaTypes(types, "")
			row.NewArea = areaChoices(ifaces, &s, "")
			used = s.Interfaces()
		} else {
			r := inst.RIP
			if r == nil {
				r = &cfgRIPJSON{}
			}
			s := ripSetupOf(r)
			row.RIP = &s
			row.Ifaces = ifaceChoices(ifaces, s.Ifaces)
			used = s.Ifaces
		}
		for _, n := range used {
			inUse[n] = append(inUse[n], p.Family)
		}
		data.Protocols = append(data.Protocols, row)
	}
	data.NoForwarding = noForwarding(ifaces, inUse)
}

// noForwarding lists the interfaces in use that do not forward a family
// routed on them.  Loopback never forwards.
func noForwarding(ifaces []ifaceJSON, inUse map[string][]string) []rtNoForwarding {
	var out []rtNoForwarding
	for _, i := range ifaces {
		families := inUse[i.Name]
		if len(families) == 0 || typeSlug(i.Type) == "loopback" {
			continue
		}
		nf := rtNoForwarding{Name: i.Name}
		for _, family := range []string{"ipv4", "ipv6"} {
			if !slices.Contains(families, family) {
				continue
			}
			ip := i.IPv4
			if family == "ipv6" {
				ip = i.IPv6
			}
			if ip == nil || !ip.Forwarding {
				nf.Families = append(nf.Families, family)
			}
		}
		if len(nf.Families) > 0 {
			out = append(out, nf)
		}
	}
	return out
}

// ─── Reading ──────────────────────────────────────────────────────────────────

// fetchProtocols reads the control-plane protocols from the candidate,
// falling back to running.
func (h *ConfigureRoutesHandler) fetchProtocols(ctx context.Context) ([]rtProtoJSON, error) {
	var wrap rtProtoWrapper
	err := h.RC.Get(ctx, candidatePath+cppConfigPath, &wrap)
	if err != nil && restconf.IsNotFound(err) {
		err = h.RC.Get(ctx, "/data"+cppConfigPath, &wrap)
	}
	if err != nil && !restconf.IsNotFound(err) {
		return nil, err
	}
	return wrap.Routing.CPPs.CPP, nil
}

// fetchRoutingIfaces reads the interfaces from the candidate, falling
// back to running.
func (h *ConfigureRoutesHandler) fetchRoutingIfaces(ctx context.Context) []ifaceJSON {
	var wrap interfacesWrapper
	if err := h.RC.Get(ctx, candidatePath+"/ietf-interfaces:interfaces", &wrap); restconf.IsNotFound(err) {
		h.RC.Get(ctx, "/data/ietf-interfaces:interfaces", &wrap) //nolint:errcheck
	}
	ifaces := wrap.Interfaces.Interface
	slices.SortFunc(ifaces, func(a, b ifaceJSON) int { return strings.Compare(a.Name, b.Name) })
	return ifaces
}

// ─── Writing ──────────────────────────────────────────────────────────────────

// patchInstance adds the edit of one instance to p: the removals first,
// so an interface can move between areas, then the merge of body, the
// protocol container, e.g. "ietf-ospf:ospf".
func patchInstance(p *restconf.YangPatch, typ, name string, removes []string, body map[string]any) {
	for _, path := range removes {
		p.Remove(path)
	}
	inst := map[string]any{"type": typ, "name": name}
	for k, v := range body {
		inst[k] = v
	}
	p.Merge(instancePath(typ, name), map[string]any{"ietf-routing:control-plane-protocol": []map[string]any{inst}})
}

// redistEdit returns the protocols to add to the redistribute list, and
// appends the removal of those shown but not to be redistributed.  Any
// other redistributed protocol is left alone.
func redistEdit(removes *[]string, base string, cur rtRedistJSON, want map[string]bool) []map[string]any {
	var list []map[string]any
	for _, proto := range []string{"connected", "static"} {
		switch {
		case want[proto]:
			list = append(list, map[string]any{"protocol": proto})
		case hasRedist(cur, proto):
			*removes = append(*removes, base+"/infix-routing:redistribute/redistribute="+proto)
		}
	}
	return list
}

// routerID checks an OSPF router ID, which OSPFv3 must have.
func routerID(s string, p rtProto) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		if p.OSPF && p.Family == "ipv6" {
			return "", fmt.Errorf("%s needs a router ID, there is no IPv4 address to take one from", p.Label)
		}
		return "", nil
	}
	if ip := net.ParseIP(s); ip == nil || ip.To4() == nil {
		return "", fmt.Errorf("router ID %q is not an IPv4 address, e.g. 10.0.0.1", s)
	}
	return s, nil
}

// areaID accepts an area as a dotted quad, or as a number like "0".
func areaID(s string) (string, error) {
	if ip := net.ParseIP(s); ip != nil && ip.To4() != nil && strings.Count(s, ".") == 3 {
		return ip.To4().String(), nil
	}
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return "", fmt.Errorf("area %q is not a number or a dotted quad, e.g. 0.0.0.0", s)
	}
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n)).String(), nil
}

// ospfGlobalForm is the posted global settings of an OSPF instance.
type ospfGlobalForm struct {
	RouterID     string
	BFD          bool
	BFDChanged   bool
	Connected    bool
	Static       bool
	DefaultRoute bool
}

func parseOSPFGlobal(f url.Values, p rtProto) (ospfGlobalForm, error) {
	o := ospfGlobalForm{
		BFD:          f.Get("bfd") != "",
		Connected:    f.Get("redist-connected") != "",
		Static:       f.Get("redist-static") != "",
		DefaultRoute: f.Get("default-route") != "",
	}
	o.BFDChanged = o.BFD != (f.Get("bfd-was") == "on")
	var err error
	o.RouterID, err = routerID(f.Get("router-id"), p)
	return o, err
}

// ospfGlobalEdit turns the global settings into the removals and the body
// of an OSPF instance.  A BFD change reaches every interface in every
// area, the areas are otherwise left alone.
func ospfGlobalEdit(base string, cur *cfgOSPFJSON, f ospfGlobalForm, bfd bool) (removes []string, body map[string]any) {
	if cur == nil {
		cur = &cfgOSPFJSON{}
	}
	o := map[string]any{}
	ob := base + "/ietf-ospf:ospf"

	if f.RouterID != "" {
		o["explicit-router-id"] = f.RouterID
	} else if cur.RouterID != "" {
		removes = append(removes, ob+"/explicit-router-id")
	}
	if bfd && f.BFDChanged {
		var areas []map[string]any
		for _, a := range cur.Areas.Area {
			var ifs []map[string]any
			for _, i := range a.Interfaces.Interface {
				ifs = append(ifs, map[string]any{"name": i.Name, "bfd": map[string]any{"enabled": f.BFD}})
			}
			if len(ifs) > 0 {
				areas = append(areas, map[string]any{"area-id": a.ID, "interfaces": map[string]any{"interface": ifs}})
			}
		}
		if len(areas) > 0 {
			o["areas"] = map[string]any{"area": areas}
		}
	}
	if list := redistEdit(&removes, ob, cur.Redistribute, map[string]bool{"connected": f.Connected, "static": f.Static}); len(list) > 0 {
		o["infix-routing:redistribute"] = map[string]any{"redistribute": list}
	}
	if f.DefaultRoute || cur.DefaultRoute.Enabled != nil {
		o["infix-routing:default-route-advertise"] = map[string]any{"enabled": f.DefaultRoute}
	}
	return removes, map[string]any{"ietf-ospf:ospf": o}
}

// ospfArea is a posted area: the add dialog or an area's fold-out.
type ospfArea struct {
	ID     string
	Type   string // area type identity, e.g. "ietf-ospf:stub-area"
	Ifaces []string
}

// parseOSPFArea checks a posted area against the instance and the model:
// a valid new area, or an existing one, an area type the model has, and
// interfaces that are not in another area.
func parseOSPFArea(f url.Values, cur *cfgOSPFJSON, existing string, types []schema.IdentityOption) (ospfArea, error) {
	a := ospfArea{ID: existing, Type: f.Get("area-type"), Ifaces: f["ifaces"]}
	if !slices.ContainsFunc(types, func(o schema.IdentityOption) bool { return o.Value == a.Type }) {
		return a, fmt.Errorf("unknown area type %q", a.Type)
	}
	if existing == "" {
		id, err := areaID(strings.TrimSpace(f.Get("area-id")))
		if err != nil {
			return a, err
		}
		for _, c := range cur.Areas.Area {
			if c.ID == id {
				return a, fmt.Errorf("area %s already exists", id)
			}
		}
		a.ID = id
	}
	if a.ID == "0.0.0.0" && !isNormalArea(a.Type) {
		return a, fmt.Errorf("the backbone area 0.0.0.0 can not be a %s area", areaTypeLabel(a.Type))
	}
	for _, c := range cur.Areas.Area {
		if c.ID == a.ID {
			continue
		}
		for _, i := range c.Interfaces.Interface {
			if slices.Contains(a.Ifaces, i.Name) {
				return a, fmt.Errorf("%s is in area %s, an interface can only be in one area", i.Name, c.ID)
			}
		}
	}
	slices.Sort(a.Ifaces)
	return a, nil
}

// ospfAreaEdit turns an area into the removals and the body of its OSPF
// instance.  An interface new to the area is enabled, with BFD when the
// instance runs BFD on all its interfaces.
func ospfAreaEdit(base string, cur *cfgOSPFJSON, a ospfArea, bfdOn bool) (removes []string, body map[string]any) {
	ob := base + "/ietf-ospf:ospf"
	ab := ob + "/areas/area=" + restconf.EscapeKey(a.ID)
	old := map[string]bool{}
	for _, c := range cur.Areas.Area {
		if c.ID != a.ID {
			continue
		}
		for _, i := range c.Interfaces.Interface {
			old[i.Name] = true
			if !slices.Contains(a.Ifaces, i.Name) {
				removes = append(removes, ab+"/interfaces/interface="+restconf.EscapeKey(i.Name))
			}
		}
	}
	var ifs []map[string]any
	for _, name := range a.Ifaces {
		i := map[string]any{"name": name}
		if !old[name] {
			i["enabled"] = true
			if bfdOn {
				i["bfd"] = map[string]any{"enabled": true}
			}
		}
		ifs = append(ifs, i)
	}
	area := map[string]any{"area-id": a.ID, "area-type": a.Type}
	if len(ifs) > 0 {
		area["interfaces"] = map[string]any{"interface": ifs}
	}
	return removes, map[string]any{"ietf-ospf:ospf": map[string]any{"areas": map[string]any{"area": []map[string]any{area}}}}
}

// ripEdit turns the fold-out into the removals and the body of a RIP
// instance.
func ripEdit(base string, cur *cfgRIPJSON, ifaces []string, connected, static, defRoute bool) (removes []string, body map[string]any) {
	if cur == nil {
		cur = &cfgRIPJSON{}
	}
	rb := base + "/ietf-rip:rip"
	o := map[string]any{}
	for _, i := range cur.Interfaces.Interface {
		if !slices.Contains(ifaces, i.Interface) {
			removes = append(removes, rb+"/interfaces/interface="+restconf.EscapeKey(i.Interface))
		}
	}
	var list []map[string]any
	for _, name := range ifaces {
		list = append(list, map[string]any{"interface": name})
	}
	if len(list) > 0 {
		o["interfaces"] = map[string]any{"interface": list}
	}
	if rl := redistEdit(&removes, rb, cur.Redistribute, map[string]bool{"connected": connected, "static": static}); len(rl) > 0 {
		o["infix-routing:redistribute"] = map[string]any{"redistribute": rl}
	}
	if defRoute || cur.DefaultRoute.Enabled != nil {
		o["originate-default-route"] = map[string]any{"enabled": defRoute}
	}
	return removes, map[string]any{"ietf-rip:rip": o}
}

// addBody is the body of a new instance from the add dialog: the chosen
// interfaces, in area 0.0.0.0 for OSPF, and a router ID.
func addBody(p rtProto, ifaces []string, routerID string) map[string]any {
	if !p.OSPF {
		rip := map[string]any{}
		var list []map[string]any
		for _, name := range ifaces {
			list = append(list, map[string]any{"interface": name})
		}
		if len(list) > 0 {
			rip["interfaces"] = map[string]any{"interface": list}
		}
		return map[string]any{"ietf-rip:rip": rip}
	}
	o := map[string]any{}
	if routerID != "" {
		o["explicit-router-id"] = routerID
	}
	if len(ifaces) > 0 {
		var list []map[string]any
		for _, name := range ifaces {
			list = append(list, map[string]any{"name": name, "enabled": true})
		}
		o["areas"] = map[string]any{"area": []map[string]any{{
			"area-id": "0.0.0.0", "area-type": "ietf-ospf:normal-area",
			"interfaces": map[string]any{"interface": list},
		}}}
	}
	return map[string]any{"ietf-ospf:ospf": o}
}

// protoRequest checks a protocol request: the device has FRR, and the
// protocol named in the form or URL is one of ours.
func protoRequest(w http.ResponseWriter, r *http.Request, key string) (rtProto, bool) {
	if !CapabilitiesFromContext(r.Context()).Has(features.Routing) {
		http.NotFound(w, r)
		return rtProto{}, false
	}
	p, ok := protoByKey(key)
	if !ok {
		http.NotFound(w, r)
	}
	return p, ok
}

// AddProtocol creates a protocol instance from the add dialog.
// POST /configure/routes/protocol
func (h *ConfigureRoutesHandler) AddProtocol(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	p, ok := protoRequest(w, r, r.PostForm.Get("proto"))
	if !ok {
		return
	}
	id, err := routerID(r.PostForm.Get("router-id"), p)
	if err != nil {
		renderSaveError(w, err)
		return
	}
	patch := restconf.NewYangPatch(candidatePath)
	patchInstance(patch, p.Type, "default", nil, addBody(p, r.PostForm["ifaces"], id))
	if err := h.RC.YangPatch(r.Context(), patch); err != nil {
		log.Printf("configure routing add %s: %v", p.Key, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, p.Label+" added", "/configure/routes")
}

// SaveProtocol writes a protocol's global settings to the candidate in
// one YANG Patch.
// PUT /configure/routes/protocol/{key}
func (h *ConfigureRoutesHandler) SaveProtocol(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	p, ok := protoRequest(w, r, r.PathValue("key"))
	if !ok {
		return
	}
	cpps, err := h.fetchProtocols(r.Context())
	if err != nil {
		renderSaveError(w, err)
		return
	}
	inst := findInstance(cpps, p.Type)
	if inst == nil {
		renderSaveError(w, fmt.Errorf("%s is not set up", p.Label))
		return
	}
	base := instancePath(inst.Type, inst.Name)
	var removes []string
	var body map[string]any
	if p.OSPF {
		f, err := parseOSPFGlobal(r.PostForm, p)
		if err != nil {
			renderSaveError(w, err)
			return
		}
		removes, body = ospfGlobalEdit(base, inst.OSPF, f, hasOSPFBFD(h.Schema.Manager()))
	} else {
		f := r.PostForm
		removes, body = ripEdit(base, inst.RIP, f["ifaces"],
			f.Get("redist-connected") != "", f.Get("redist-static") != "", f.Get("default-route") != "")
	}
	patch := restconf.NewYangPatch(candidatePath)
	patchInstance(patch, inst.Type, inst.Name, removes, body)
	if err := h.RC.YangPatch(r.Context(), patch); err != nil {
		log.Printf("configure routing %s: %v", p.Key, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, p.Label+" saved", "/configure/routes")
}

// ospfInstance finds the OSPF instance of a protocol request, for the
// area handlers.
func (h *ConfigureRoutesHandler) ospfInstance(w http.ResponseWriter, r *http.Request) (rtProto, *rtProtoJSON, bool) {
	p, ok := protoRequest(w, r, r.PathValue("key"))
	if !ok || !p.OSPF {
		if ok {
			http.NotFound(w, r)
		}
		return p, nil, false
	}
	cpps, err := h.fetchProtocols(r.Context())
	if err != nil {
		renderSaveError(w, err)
		return p, nil, false
	}
	inst := findInstance(cpps, p.Type)
	if inst == nil {
		renderSaveError(w, fmt.Errorf("%s is not set up", p.Label))
		return p, nil, false
	}
	if inst.OSPF == nil {
		inst.OSPF = &cfgOSPFJSON{}
	}
	return p, inst, true
}

// saveArea writes a new or changed area of an OSPF instance.
func (h *ConfigureRoutesHandler) saveArea(w http.ResponseWriter, r *http.Request, existing string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	p, inst, ok := h.ospfInstance(w, r)
	if !ok {
		return
	}
	mgr := h.Schema.Manager()
	a, err := parseOSPFArea(r.PostForm, inst.OSPF, existing, schema.OptionsFor(mgr, ospfAreaTypePath))
	if err != nil {
		renderSaveError(w, err)
		return
	}
	bfdOn := hasOSPFBFD(mgr) && ospfSetupOf(inst.OSPF).BFD
	removes, body := ospfAreaEdit(instancePath(inst.Type, inst.Name), inst.OSPF, a, bfdOn)
	patch := restconf.NewYangPatch(candidatePath)
	patchInstance(patch, inst.Type, inst.Name, removes, body)
	if err := h.RC.YangPatch(r.Context(), patch); err != nil {
		log.Printf("configure routing %s area %s: %v", p.Key, a.ID, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, p.Label+" area "+a.ID+" saved", "/configure/routes")
}

// AddArea adds an area, with its interfaces, to an OSPF instance.
// POST /configure/routes/protocol/{key}/areas
func (h *ConfigureRoutesHandler) AddArea(w http.ResponseWriter, r *http.Request) {
	h.saveArea(w, r, "")
}

// SaveArea writes an area's fold-out.
// PUT /configure/routes/protocol/{key}/areas/{area}
func (h *ConfigureRoutesHandler) SaveArea(w http.ResponseWriter, r *http.Request) {
	h.saveArea(w, r, r.PathValue("area"))
}

// DeleteArea removes an area from an OSPF instance.
// DELETE /configure/routes/protocol/{key}/areas/{area}
func (h *ConfigureRoutesHandler) DeleteArea(w http.ResponseWriter, r *http.Request) {
	p, inst, ok := h.ospfInstance(w, r)
	if !ok {
		return
	}
	area := r.PathValue("area")
	patch := restconf.NewYangPatch(candidatePath)
	patch.Remove(instancePath(inst.Type, inst.Name) + "/ietf-ospf:ospf/areas/area=" + restconf.EscapeKey(area))
	if err := h.RC.YangPatch(r.Context(), patch); err != nil {
		log.Printf("configure routing %s delete area %s: %v", p.Key, area, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, p.Label+" area "+area+" deleted", "/configure/routes")
}

// DeleteProtocol removes a protocol instance.
// DELETE /configure/routes/protocol/{key}
func (h *ConfigureRoutesHandler) DeleteProtocol(w http.ResponseWriter, r *http.Request) {
	p, ok := protoRequest(w, r, r.PathValue("key"))
	if !ok {
		return
	}
	cpps, err := h.fetchProtocols(r.Context())
	if err != nil {
		renderSaveError(w, err)
		return
	}
	patch := restconf.NewYangPatch(candidatePath)
	if inst := findInstance(cpps, p.Type); inst != nil {
		patch.Remove(instancePath(inst.Type, inst.Name))
	}
	if err := h.RC.YangPatch(r.Context(), patch); err != nil {
		log.Printf("configure routing delete %s: %v", p.Key, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, p.Label+" deleted", "/configure/routes")
}

// EnableForwarding turns on forwarding for the posted interfaces, each
// value "name/family", e.g. "lan1/ipv4".
// POST /configure/routes/forwarding
func (h *ConfigureRoutesHandler) EnableForwarding(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	byIface := map[string]map[string]any{}
	var order []string
	for _, v := range r.PostForm["forward"] {
		name, family, ok := strings.Cut(v, "/")
		if !ok || (family != "ipv4" && family != "ipv6") || name == "" {
			continue
		}
		if byIface[name] == nil {
			byIface[name] = map[string]any{"name": name}
			order = append(order, name)
		}
		byIface[name]["ietf-ip:"+family] = map[string]any{"forwarding": true}
	}
	if len(order) == 0 {
		renderSaveError(w, fmt.Errorf("no interfaces to forward on"))
		return
	}
	var list []map[string]any
	for _, name := range order {
		list = append(list, byIface[name])
	}
	body := map[string]any{"ietf-interfaces:interfaces": map[string]any{"interface": list}}
	if err := h.RC.Patch(r.Context(), candidatePath+"/ietf-interfaces:interfaces", body); err != nil {
		log.Printf("configure routing forwarding: %v", err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Forwarding enabled on "+strings.Join(order, ", "), "/configure/routes")
}
