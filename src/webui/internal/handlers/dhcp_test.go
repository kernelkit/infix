// SPDX-License-Identifier: MIT

package handlers

import (
	"reflect"
	"testing"
)

func TestGroupDHCPLeases(t *testing.T) {
	subnets := []DHCPSubnet{
		{Subnet: "192.168.10.0/24", Description: "guest"},
		{Subnet: "192.168.2.0/24"},
		{Subnet: "10.0.0.0/24"},
	}
	leases := []DHCPLease{
		{Address: "192.168.2.100"},
		{Address: "192.168.10.5"},
		{Address: "172.16.0.1"},
		{Address: "192.168.2.20"},
		{Address: "192.168.2.3"},
	}

	got := groupDHCPLeases(subnets, leases)

	var order []string
	for _, g := range got {
		order = append(order, g.Subnet+":")
		for _, l := range g.Leases {
			order = append(order, l.Address)
		}
	}
	want := []string{
		"192.168.2.0/24:", "192.168.2.3", "192.168.2.20", "192.168.2.100",
		"192.168.10.0/24:", "192.168.10.5",
		":", "172.16.0.1",
	}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("want %v, got %v", want, order)
	}
}
