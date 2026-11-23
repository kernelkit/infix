// SPDX-License-Identifier: MIT

package handlers

import "testing"

func TestNACMGroupsOf(t *testing.T) {
	data := []byte(`{"ietf-netconf-acm:nacm":{"groups":{"group":[
		{"name":"admin","user-name":["admin"]},
		{"name":"guest","user-name":["jane","joe"]},
		{"name":"empty"},
		{"name":"service","user-name":["jane"]}]}}}`)

	got, err := nacmGroupsOf(data, "jane")
	if err != nil || len(got) != 2 || got[0] != "guest" || got[1] != "service" {
		t.Errorf("jane: got %v, %v", got, err)
	}
	if got, err := nacmGroupsOf(nil, "jane"); err != nil || len(got) != 0 {
		t.Errorf("no data: got %v, %v", got, err)
	}
	if _, err := nacmGroupsOf([]byte("{"), "jane"); err == nil {
		t.Error("bad data: no error")
	}
}
