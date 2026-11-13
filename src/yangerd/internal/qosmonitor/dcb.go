package qosmonitor

import (
	"encoding/binary"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

// Attributes of an IEEE DCB reply, see linux/dcbnl.h.
const (
	dcbCmdIEEEGet     = 21
	dcbAttrIEEE       = 13
	dcbAttrAppTable   = 3
	dcbAttrTrustTable = 11
	dcbAttrRewrTable  = 12
	dcbSelectorEther  = 1
	dcbSelectorDSCP   = 5
	dcbSelectorPCP    = 255
)

// DCB reads the DCB trust order, application and rewrite tables of a
// port over rtnetlink.  dcb(8) cannot serve as a batch helper for this: it writes
// one JSON document for the life of the process.  Only the monitor's
// worker calls it.
type DCB struct {
	conn *netlink.Conn
}

// DialDCB opens the rtnetlink socket the tables are read from.
func DialDCB() (*DCB, error) {
	conn, err := netlink.Dial(unix.NETLINK_ROUTE, nil)
	if err != nil {
		return nil, err
	}
	return &DCB{conn: conn}, nil
}

// dcbApp is one struct dcb_app: for the application table the priority a
// protocol value gets, for the rewrite table the value a priority gets.
type dcbApp struct {
	Selector uint8
	Priority uint8
	Protocol uint16
}

// dcbTables is what a port's driver holds for classification and
// remarking.
type dcbTables struct {
	HasTrust bool    // the driver keeps a trust order, possibly empty
	Trust    []uint8 // selectors in the order they are trusted
	App      []dcbApp
	Rewr     []dcbApp
}

// Tables reads the port's trust order, its application table and its
// rewrite table.
func (d *DCB) Tables(ifname string) (dcbTables, error) {
	ae := netlink.NewAttributeEncoder()
	ae.String(dcbAttrIfname, ifname)
	attrs, err := ae.Encode()
	if err != nil {
		return dcbTables{}, err
	}
	req := netlink.Message{
		Header: netlink.Header{Type: unix.RTM_GETDCB, Flags: netlink.Request},
		Data:   append([]byte{unix.AF_UNSPEC, dcbCmdIEEEGet, 0, 0}, attrs...),
	}

	msgs, err := d.conn.Execute(req)
	if err != nil {
		return dcbTables{}, err
	}
	var t dcbTables
	for _, msg := range msgs {
		parseIEEE(msg.Data, &t)
	}
	return t, nil
}

func parseIEEE(data []byte, t *dcbTables) {
	if len(data) < sizeofDcbmsg {
		return
	}
	ad, err := netlink.NewAttributeDecoder(data[sizeofDcbmsg:])
	if err != nil {
		return
	}
	for ad.Next() {
		if ad.Type() != dcbAttrIEEE {
			continue
		}
		ad.Nested(func(nad *netlink.AttributeDecoder) error {
			for nad.Next() {
				switch nad.Type() {
				case dcbAttrAppTable:
					t.App = append(t.App, apps(nad.Bytes())...)
				case dcbAttrRewrTable:
					t.Rewr = append(t.Rewr, apps(nad.Bytes())...)
				case dcbAttrTrustTable:
					t.HasTrust = true
					t.Trust = append(t.Trust, selectors(nad.Bytes())...)
				}
			}
			return nil
		})
	}
}

// apps decodes a table of struct dcb_app attributes.
func apps(table []byte) []dcbApp {
	ad, err := netlink.NewAttributeDecoder(table)
	if err != nil {
		return nil
	}
	var out []dcbApp
	for ad.Next() {
		if b := ad.Bytes(); len(b) >= 4 {
			out = append(out, dcbApp{b[0], b[1], binary.NativeEndian.Uint16(b[2:4])})
		}
	}
	return out
}

// selectors decodes the trust table, one selector per attribute.
func selectors(table []byte) []uint8 {
	ad, err := netlink.NewAttributeDecoder(table)
	if err != nil {
		return nil
	}
	var out []uint8
	for ad.Next() {
		if b := ad.Bytes(); len(b) > 0 {
			out = append(out, b[0])
		}
	}
	return out
}

// hasPrioMap tells whether a table holds a PCP or DSCP entry.
func hasPrioMap(table []dcbApp) bool {
	for _, a := range table {
		if a.Selector == dcbSelectorDSCP || a.Selector == dcbSelectorPCP {
			return true
		}
	}
	return false
}
