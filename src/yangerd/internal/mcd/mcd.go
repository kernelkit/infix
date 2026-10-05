// Package mcd reads the IGMP and MLD querier state from mcd's IPC
// socket and maps it to the bridge multicast nodes of infix-interfaces.
//
// mcd announces nothing, but answers `show interfaces json` from memory,
// so the querier state is read at GET time.
package mcd

import (
	"encoding/json"
	"io"
	"net"
	"time"
)

// SocketPath is mcd's IPC socket.
const SocketPath = "/run/mcd.sock"

// Querier is one row of `show interfaces json`, the querier state of a
// bridge, bridge VLAN, or VLAN interface for one address family.
type Querier struct {
	Interface string `json:"interface"`
	Family    string `json:"family"`
	VID       int    `json:"vid"`
	State     string `json:"state"`
	Address   string `json:"querier"`
	Timeout   *int   `json:"timeout"`
	Interval  int    `json:"interval"`
	Version   int    `json:"version"`
}

// Read asks mcd at path for its querier state.
func Read(path string, timeout time.Duration) ([]Querier, error) {
	conn, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	if _, err := conn.Write([]byte("show interfaces json")); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(conn)
	if err != nil {
		return nil, err
	}

	return Parse(data)
}

// Parse decodes the answer to `show interfaces json`.
func Parse(data []byte) ([]Querier, error) {
	var qs []Querier
	if err := json.Unmarshal(data, &qs); err != nil {
		return nil, err
	}
	return qs, nil
}

// find returns the querier of a bridge, or bridge VLAN, for one family.
// mcd runs on the bridge itself, or for a VLAN on the bridge's VLAN
// interface, upper.
func find(qs []Querier, bridge string, vid int, upper, family string) *Querier {
	for i := range qs {
		q := &qs[i]
		if q.Family != family {
			continue
		}
		if q.Interface == bridge && q.VID == vid {
			return q
		}
		if upper != "" && q.Interface == upper && q.VID == 0 {
			return q
		}
	}
	return nil
}

// Apply sets the querier mode, query interval and active queriers in
// mcast, the multicast node of a bridge (vid 0) or of a bridge VLAN.
func Apply(mcast map[string]any, qs []Querier, bridge string, vid int, upper string) {
	q4 := find(qs, bridge, vid, upper, "ipv4")

	mode := "off"
	if q4 != nil {
		mode = "auto"
		if q4.State == "proxy" {
			mode = "proxy"
		}
		if q4.Interval > 0 {
			mcast["query-interval"] = q4.Interval
		}
	}
	mcast["querier"] = mode

	var active []any
	for _, family := range []string{"ipv4", "ipv6"} {
		q := find(qs, bridge, vid, upper, family)
		if q == nil || (q.State != "up" && q.State != "proxy") {
			continue
		}
		aq := map[string]any{
			"family":         q.Family,
			"address":        q.Address,
			"local":          q.Timeout == nil,
			"version":        q.Version,
			"query-interval": q.Interval,
		}
		if q.Timeout != nil {
			aq["timeout"] = *q.Timeout
		}
		active = append(active, aq)
	}
	if len(active) > 0 {
		mcast["active-querier"] = active
	}
}
