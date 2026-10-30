package qosmonitor

import (
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

// Attributes of an IEEE DCB reply, see linux/dcbnl.h.
const (
	dcbCmdIEEEGet    = 21
	dcbAttrIEEE      = 13
	dcbAttrAppTable  = 3
	dcbAttrRewrTable = 12
	dcbSelectorDSCP  = 5
	dcbSelectorPCP   = 255
)

// DCB reads the DCB application and rewrite tables of a port over
// rtnetlink.  dcb(8) cannot serve as a batch helper for this: it writes
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

// Tables reports whether the port maps PCP or DSCP to priority in its
// application table, and priority to PCP or DSCP in its rewrite table.
func (d *DCB) Tables(ifname string) (app, rewr bool, err error) {
	ae := netlink.NewAttributeEncoder()
	ae.String(dcbAttrIfname, ifname)
	attrs, err := ae.Encode()
	if err != nil {
		return false, false, err
	}
	req := netlink.Message{
		Header: netlink.Header{Type: unix.RTM_GETDCB, Flags: netlink.Request},
		Data:   append([]byte{unix.AF_UNSPEC, dcbCmdIEEEGet, 0, 0}, attrs...),
	}

	msgs, err := d.conn.Execute(req)
	if err != nil {
		return false, false, err
	}
	for _, msg := range msgs {
		a, r := parseIEEE(msg.Data)
		app, rewr = app || a, rewr || r
	}
	return app, rewr, nil
}

func parseIEEE(data []byte) (app, rewr bool) {
	if len(data) < sizeofDcbmsg {
		return false, false
	}
	ad, err := netlink.NewAttributeDecoder(data[sizeofDcbmsg:])
	if err != nil {
		return false, false
	}
	for ad.Next() {
		if ad.Type() != dcbAttrIEEE {
			continue
		}
		ad.Nested(func(nad *netlink.AttributeDecoder) error {
			for nad.Next() {
				switch nad.Type() {
				case dcbAttrAppTable:
					app = hasPrioMap(nad.Bytes())
				case dcbAttrRewrTable:
					rewr = hasPrioMap(nad.Bytes())
				}
			}
			return nil
		})
	}
	return app, rewr
}

// hasPrioMap tells whether a table of struct dcb_app holds a PCP or DSCP
// entry.  Each entry leads with its selector.
func hasPrioMap(table []byte) bool {
	ad, err := netlink.NewAttributeDecoder(table)
	if err != nil {
		return false
	}
	for ad.Next() {
		if b := ad.Bytes(); len(b) > 0 && (b[0] == dcbSelectorDSCP || b[0] == dcbSelectorPCP) {
			return true
		}
	}
	return false
}
