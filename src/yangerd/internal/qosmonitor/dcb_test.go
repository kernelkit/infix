package qosmonitor

import (
	"reflect"
	"testing"

	"github.com/mdlayher/netlink"
)

// ieeeReply is an RTM_GETDCB reply carrying app and rewrite tables of
// struct dcb_app: selector, priority, protocol.
func ieeeReply(t *testing.T, app, rewr [][]byte) []byte {
	t.Helper()
	table := func(entries [][]byte) func(*netlink.AttributeEncoder) error {
		return func(ae *netlink.AttributeEncoder) error {
			for _, e := range entries {
				ae.Bytes(1, e)
			}
			return nil
		}
	}
	ae := netlink.NewAttributeEncoder()
	ae.String(dcbAttrIfname, "e1")
	ae.Nested(dcbAttrIEEE, func(nae *netlink.AttributeEncoder) error {
		nae.Bytes(1, make([]byte, 59)) // ETS, skipped
		nae.Nested(dcbAttrAppTable, table(app))
		nae.Nested(dcbAttrTrustTable, func(tae *netlink.AttributeEncoder) error {
			tae.Uint8(1, dcbSelectorDSCP)
			tae.Uint8(1, dcbSelectorPCP)
			return nil
		})
		nae.Nested(dcbAttrRewrTable, table(rewr))
		return nil
	})
	attrs, err := ae.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return append([]byte{0, dcbCmdIEEEGet, 0, 0}, attrs...)
}

func TestParseIEEE(t *testing.T) {
	dscp := []byte{dcbSelectorDSCP, 5, 46, 0}
	pcp := []byte{dcbSelectorPCP, 3, 3, 0}
	ethertype := []byte{1, 3, 0x8e, 0x88}

	for _, tc := range []struct {
		name      string
		app, rewr [][]byte
		wantApp   bool
		wantRewr  bool
	}{
		{"empty", nil, nil, false, false},
		{"dscp classification", [][]byte{dscp}, nil, true, false},
		{"pcp remarking", nil, [][]byte{pcp}, false, true},
		{"ethertype only", [][]byte{ethertype}, nil, false, false},
		{"both", [][]byte{ethertype, pcp}, [][]byte{dscp}, true, true},
	} {
		var got dcbTables
		parseIEEE(ieeeReply(t, tc.app, tc.rewr), &got)
		if hasPrioMap(got.App) != tc.wantApp || hasPrioMap(got.Rewr) != tc.wantRewr {
			t.Errorf("%s: got %+v", tc.name, got)
		}
	}
}

func TestParseIEEETables(t *testing.T) {
	var got dcbTables
	parseIEEE(ieeeReply(t, [][]byte{{dcbSelectorDSCP, 5, 46, 0}, {dcbSelectorPCP, 3, 11, 0}},
		[][]byte{{dcbSelectorPCP, 3, 3, 0}}), &got)
	want := dcbTables{
		HasTrust: true,
		Trust:    []uint8{dcbSelectorDSCP, dcbSelectorPCP},
		App:      []dcbApp{{dcbSelectorDSCP, 5, 46}, {dcbSelectorPCP, 3, 11}},
		Rewr:     []dcbApp{{dcbSelectorPCP, 3, 3}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}
