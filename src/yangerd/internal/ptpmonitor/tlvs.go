package ptpmonitor

import (
	"bytes"
	"encoding/binary"
	"fmt"

	ptp "github.com/facebook/time/ptp/protocol"
)

// Management IDs not defined by the library.
const idSubscribeEventsNP ptp.ManagementID = 0xC003

// Event bit numbers in subscribe_events_np.bitmask (linuxptp
// notification.h).
const (
	notifyPortState = iota
	notifyTimeSync
	notifyParentDataSet
)

// portDataSetTLV mirrors linuxptp struct portDS (PORT_DATA_SET,
// IEEE 1588 Table 95).  Not implemented by the library.
type portDataSetTLV struct {
	ptp.ManagementTLVHead

	PortIdentity            ptp.PortIdentity
	PortState               uint8
	LogMinDelayReqInterval  int8
	PeerMeanPathDelay       ptp.TimeInterval
	LogAnnounceInterval     int8
	AnnounceReceiptTimeout  uint8
	LogSyncInterval         int8
	DelayMechanism          uint8
	LogMinPdelayReqInterval int8
	VersionNumber           uint8
}

// timePropertiesDataSetTLV mirrors linuxptp struct timePropertiesDS
// (TIME_PROPERTIES_DATA_SET, IEEE 1588 Table 92).  The flags byte packs
// the Announce flag-field bits (msg.h): LEAP_61, LEAP_59, UTC_OFF_VALID,
// PTP_TIMESCALE, TIME_TRACEABLE, FREQ_TRACEABLE.
type timePropertiesDataSetTLV struct {
	ptp.ManagementTLVHead

	CurrentUtcOffset int16
	Flags            uint8
	TimeSource       ptp.TimeSource
}

const (
	flagLeap61         = 1 << 0
	flagLeap59         = 1 << 1
	flagUtcOffValid    = 1 << 2
	flagPtpTimescale   = 1 << 3
	flagTimeTraceable  = 1 << 4
	flagFreqTraceable  = 1 << 5
	flagDefaultDSSOnly = 1 << 1 /* DDS_SLAVE_ONLY in DEFAULT_DATA_SET flags */
)

// subscribeEventsNPTLV mirrors linuxptp struct subscribe_events_np
// (SUBSCRIBE_EVENTS_NP, linuxptp tlv.h).
type subscribeEventsNPTLV struct {
	ptp.ManagementTLVHead

	Duration uint16
	Bitmask  [64]uint8
}

// tlvHead builds a management TLV head for the given ID and full TLV
// struct size.
func tlvHead(id ptp.ManagementID, size uint16) ptp.ManagementTLVHead {
	return ptp.ManagementTLVHead{
		TLVHead: ptp.TLVHead{
			TLVType:     ptp.TLVManagement,
			LengthField: size - uint16(binary.Size(ptp.TLVHead{})),
		},
		ManagementID: id,
	}
}

// request wraps a TLV in a management message.  sdoID carries the
// transport-specific nibble: ptp4l silently drops management messages
// whose nibble does not match its transportSpecific setting, so gPTP
// (802.1AS) instances require sdoID 1.
func request(sdoID uint8, action ptp.Action, tlv ptp.ManagementTLV, size uint16) *ptp.Management {
	headerSize := uint16(binary.Size(ptp.ManagementMsgHead{}))

	return &ptp.Management{
		ManagementMsgHead: ptp.ManagementMsgHead{
			Header: ptp.Header{
				SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageManagement, sdoID),
				Version:            ptp.Version,
				MessageLength:      headerSize + size,
				LogMessageInterval: ptp.MgmtLogMessageInterval,
			},
			TargetPortIdentity: ptp.DefaultTargetPortIdentity,
			ActionField:        action,
		},
		TLV: tlv,
	}
}

// getRequests builds the GET requests for all data sets the monitor
// tracks.  The payloads are zero-padded full TLV structs, mirroring the
// library's own request builders.
func getRequests(sdoID uint8) []*ptp.Management {
	var reqs []*ptp.Management

	for _, id := range []ptp.ManagementID{
		ptp.IDDefaultDataSet,
		ptp.IDCurrentDataSet,
		ptp.IDParentDataSet,
		ptp.IDTimePropertiesDataSet,
		ptp.IDPortDataSet,
		ptp.IDPortStatsNP,
	} {
		var tlv ptp.ManagementTLV
		var size uint16

		switch id {
		case ptp.IDDefaultDataSet:
			size = uint16(binary.Size(ptp.DefaultDataSetTLV{}))
			tlv = &ptp.DefaultDataSetTLV{ManagementTLVHead: tlvHead(id, size)}
		case ptp.IDCurrentDataSet:
			size = uint16(binary.Size(ptp.CurrentDataSetTLV{}))
			tlv = &ptp.CurrentDataSetTLV{ManagementTLVHead: tlvHead(id, size)}
		case ptp.IDParentDataSet:
			size = uint16(binary.Size(ptp.ParentDataSetTLV{}))
			tlv = &ptp.ParentDataSetTLV{ManagementTLVHead: tlvHead(id, size)}
		case ptp.IDTimePropertiesDataSet:
			size = uint16(binary.Size(timePropertiesDataSetTLV{}))
			tlv = &timePropertiesDataSetTLV{ManagementTLVHead: tlvHead(id, size)}
		case ptp.IDPortDataSet:
			size = uint16(binary.Size(portDataSetTLV{}))
			tlv = &portDataSetTLV{ManagementTLVHead: tlvHead(id, size)}
		case ptp.IDPortStatsNP:
			// Send just the TLV head, like pmc does
			size = uint16(binary.Size(ptp.ManagementTLVHead{}))
			h := tlvHead(id, size)
			tlv = &h
		}

		reqs = append(reqs, request(sdoID, ptp.GET, tlv, size))
	}

	return reqs
}

// subscribeRequest builds a SET SUBSCRIBE_EVENTS_NP request asking
// ptp4l to push notifications for the given event bits for duration
// seconds.
func subscribeRequest(sdoID uint8, duration uint16, events ...int) *ptp.Management {
	size := uint16(binary.Size(subscribeEventsNPTLV{}))

	tlv := &subscribeEventsNPTLV{
		ManagementTLVHead: tlvHead(idSubscribeEventsNP, size),
		Duration:          duration,
	}
	for _, ev := range events {
		tlv.Bitmask[ev/8] |= 1 << (ev % 8)
	}

	return request(sdoID, ptp.SET, tlv, size)
}

// mgmtError is a MANAGEMENT_ERROR_STATUS response.
type mgmtError struct {
	id  ptp.ManagementID
	err ptp.ManagementErrorID
}

func (e *mgmtError) Error() string {
	return fmt.Sprintf("management error for 0x%04x: %v", uint16(e.id), e.err)
}

// decodePacket parses a management message from ptp4l into one of the
// supported TLVs.  The library's own decoder is not extensible (its TLV
// registry is package-private), so all TLVs are decoded here.
func decodePacket(data []byte) (ptp.ManagementTLV, error) {
	var head ptp.ManagementMsgHead
	var tlvHead ptp.ManagementTLVHead

	r := bytes.NewReader(data)
	if err := binary.Read(r, binary.BigEndian, &head); err != nil {
		return nil, fmt.Errorf("management header: %w", err)
	}
	if err := binary.Read(r, binary.BigEndian, &tlvHead.TLVHead); err != nil {
		return nil, fmt.Errorf("TLV header: %w", err)
	}

	if tlvHead.TLVType == ptp.TLVManagementErrorStatus {
		e := &mgmtError{}
		if err := binary.Read(r, binary.BigEndian, &e.err); err != nil {
			return nil, fmt.Errorf("error status: %w", err)
		}
		if err := binary.Read(r, binary.BigEndian, &e.id); err != nil {
			return nil, fmt.Errorf("error status ID: %w", err)
		}
		return nil, e
	}
	if tlvHead.TLVType != ptp.TLVManagement {
		return nil, fmt.Errorf("unexpected TLV type 0x%04x", uint16(tlvHead.TLVType))
	}

	if err := binary.Read(r, binary.BigEndian, &tlvHead.ManagementID); err != nil {
		return nil, fmt.Errorf("management ID: %w", err)
	}

	// Rewind to the start of the TLV so the full struct (embedded
	// head included) can be read in one go.
	tlvStart := int64(binary.Size(head))
	tlvData := data[tlvStart:]
	tr := bytes.NewReader(tlvData)

	switch tlvHead.ManagementID {
	case ptp.IDDefaultDataSet:
		tlv := &ptp.DefaultDataSetTLV{}
		return tlv, binary.Read(tr, binary.BigEndian, tlv)
	case ptp.IDCurrentDataSet:
		tlv := &ptp.CurrentDataSetTLV{}
		return tlv, binary.Read(tr, binary.BigEndian, tlv)
	case ptp.IDParentDataSet:
		tlv := &ptp.ParentDataSetTLV{}
		return tlv, binary.Read(tr, binary.BigEndian, tlv)
	case ptp.IDTimePropertiesDataSet:
		tlv := &timePropertiesDataSetTLV{}
		return tlv, binary.Read(tr, binary.BigEndian, tlv)
	case ptp.IDPortDataSet:
		tlv := &portDataSetTLV{}
		return tlv, binary.Read(tr, binary.BigEndian, tlv)
	case ptp.IDTimeStatusNP:
		tlv := &ptp.TimeStatusNPTLV{}
		return tlv, binary.Read(tr, binary.BigEndian, tlv)
	case ptp.IDPortStatsNP:
		tlv := &ptp.PortStatsNPTLV{}
		if err := binary.Read(tr, binary.BigEndian, &tlv.ManagementTLVHead); err != nil {
			return nil, err
		}
		if err := binary.Read(tr, binary.BigEndian, &tlv.PortIdentity); err != nil {
			return nil, err
		}
		// linuxptp sends these counters in host byte order,
		// everything else is big-endian
		return tlv, binary.Read(tr, binary.NativeEndian, &tlv.PortStats)
	case idSubscribeEventsNP:
		tlv := &subscribeEventsNPTLV{}
		return tlv, binary.Read(tr, binary.BigEndian, tlv)
	}

	return nil, fmt.Errorf("unsupported management TLV 0x%04x", uint16(tlvHead.ManagementID))
}
