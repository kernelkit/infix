// Package ctrnet reports the host interfaces that have been moved into
// a podman container's network namespace, under the name they have in
// the running configuration.
//
// When confd hands an interface to a container, podman moves it into the
// container's netns, renames it (eth0, eth1, ...) and keeps the original
// name in ifalias.  The host side no longer sees it, so the regular
// netlink monitor cannot report it.  Collect finds those interfaces by
// walking every named netns and matching ifalias against the networks
// podman lists for its containers, and builds an ietf-interfaces entry
// for each: name is the configured (ifalias) name, description is the
// in-container name, plus "infix-interfaces:container-network" with the
// list of containers using it.  Only the common part of an interface is
// emitted (type, status, phys-address, statistics, addresses), as the
// Python yanger did; augments such as veth peer refer to the host view
// and are left out.
//
// Collect is a pure function over command output and does not touch the
// tree.  It runs after every container change and every link or address
// event in a container netns, see Watcher, and the monitor appends what
// it returns to the host's interfaces, where by construction those names
// are absent.  A failure is returned rather than logged; with no podman
// on the box the caller should treat it as "no container interfaces".
package ctrnet

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kernelkit/infix/src/yangerd/internal/iface"
)

// RunFunc runs a command and returns its stdout, the shape of
// collector.CommandRunner.Run.
type RunFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

// commonKeys are the interface leaves the Python interface_common()
// produced; everything else from iface.Transform is dropped.
var commonKeys = []string{
	"type", "name", "if-index", "admin-status", "oper-status",
	"description", "phys-address", "statistics",
	"ietf-ip:ipv4", "ietf-ip:ipv6",
}

// Collect returns one ietf-interfaces interface object per container
// network whose interface was found in a netns, keyed by the configured
// interface name, and the netns they were found in.  It runs `podman ps -a --format json` (all containers,
// like the Python yanger, so a stopped container still shows up in the
// containers list of a network it shares), `ip -j netns list` and per
// netns `ip -n <ns> -s -d -j link show` and `ip -n <ns> -j addr show`.
func Collect(ctx context.Context, run RunFunc) (map[string]json.RawMessage, []string, error) {
	networks, err := podmanNetworks(ctx, run)
	if err != nil {
		return nil, nil, err
	}
	if len(networks) == 0 {
		return map[string]json.RawMessage{}, nil, nil
	}

	out, err := run(ctx, "ip", "-j", "netns", "list")
	if err != nil {
		return nil, nil, fmt.Errorf("ip netns list: %w", err)
	}
	var namespaces []struct {
		Name string `json:"name"`
	}
	if len(out) > 0 {
		if err := json.Unmarshal(out, &namespaces); err != nil {
			return nil, nil, fmt.Errorf("ip netns list: parse: %w", err)
		}
	}

	result := make(map[string]json.RawMessage, len(networks))
	var found []string
	for _, ns := range namespaces {
		if ns.Name == "" || len(result) == len(networks) {
			continue
		}

		links, err := run(ctx, "ip", "-n", ns.Name, "-s", "-d", "-j", "link", "show")
		if err != nil {
			// The container may have gone away between the
			// listing and the query; the rest still count.
			continue
		}
		var rows []json.RawMessage
		if err := json.Unmarshal(links, &rows); err != nil {
			continue
		}

		var addrs json.RawMessage
		for _, row := range rows {
			var head struct {
				IfName  string `json:"ifname"`
				IfAlias string `json:"ifalias"`
			}
			if err := json.Unmarshal(row, &head); err != nil || head.IfAlias == "" {
				continue
			}
			containers, ok := networks[head.IfAlias]
			if !ok {
				continue
			}
			if _, done := result[head.IfAlias]; done {
				continue
			}

			if addrs == nil {
				addrs, err = run(ctx, "ip", "-n", ns.Name, "-j", "addr", "show")
				if err != nil {
					addrs = json.RawMessage(`[]`)
				}
			}

			entry := containerInterface(row, addrs, head.IfAlias, containers)
			if entry == nil {
				continue
			}
			result[head.IfAlias] = entry
			if !contains(found, ns.Name) {
				found = append(found, ns.Name)
			}
		}
	}

	return result, found, nil
}

// podmanNetworks maps each network name to the containers using it, in
// podman's listing order and without duplicates.
func podmanNetworks(ctx context.Context, run RunFunc) (map[string][]string, error) {
	out, err := run(ctx, "podman", "ps", "-a", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("podman ps: %w", err)
	}

	var containers []struct {
		Names    []string `json:"Names"`
		Networks []string `json:"Networks"`
	}
	if len(out) > 0 {
		if err := json.Unmarshal(out, &containers); err != nil {
			return nil, fmt.Errorf("podman ps: parse: %w", err)
		}
	}

	networks := map[string][]string{}
	for _, c := range containers {
		name := "Unknown"
		if len(c.Names) > 0 {
			name = c.Names[0]
		}
		for _, net := range c.Networks {
			if net == "" || contains(networks[net], name) {
				continue
			}
			networks[net] = append(networks[net], name)
		}
	}
	return networks, nil
}

// containerInterface builds the interface entry for one link found in a
// container netns.  linkRow is the single `ip link` object, addrs the
// netns' `ip addr show` array.
func containerInterface(linkRow, addrs json.RawMessage, name string, containers []string) json.RawMessage {
	links, err := json.Marshal([]json.RawMessage{linkRow})
	if err != nil {
		return nil
	}

	var doc struct {
		Interface []map[string]json.RawMessage `json:"interface"`
	}
	if err := json.Unmarshal(iface.Transform(links, addrs, nil, nil), &doc); err != nil || len(doc.Interface) == 0 {
		return nil
	}
	full := doc.Interface[0]

	entry := make(map[string]json.RawMessage, len(commonKeys)+1)
	for _, key := range commonKeys {
		if v, ok := full[key]; ok {
			entry[key] = v
		}
	}

	// podman keeps the configured name in ifalias and renames the
	// interface; the user knows it by the configured name, so swap
	// them back.
	entry["description"] = full["name"]
	entry["name"] = mustMarshal(name)
	entry["infix-interfaces:container-network"] = mustMarshal(map[string]any{
		"containers": containers,
	})

	raw, err := json.Marshal(entry)
	if err != nil {
		return nil
	}
	return raw
}

func mustMarshal(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return raw
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
