package ptpmonitor

import (
	"fmt"
	"strconv"

	ptp "github.com/facebook/time/ptp/protocol"
)

// clockClassIdentity maps IEEE 1588 clockClass values to
// ieee1588-ptp-tt identity names (identityref, not uint8).
var clockClassIdentity = map[uint8]string{
	6:   "ieee1588-ptp-tt:cc-primary-sync",
	7:   "ieee1588-ptp-tt:cc-primary-sync-lost",
	13:  "ieee1588-ptp-tt:cc-application-specific-sync",
	14:  "ieee1588-ptp-tt:cc-application-specific-sync-lost",
	52:  "ieee1588-ptp-tt:cc-primary-sync-alternative-a",
	58:  "ieee1588-ptp-tt:cc-application-specific-alternative-a",
	187: "ieee1588-ptp-tt:cc-primary-sync-alternative-b",
	193: "ieee1588-ptp-tt:cc-application-specific-alternative-b",
	248: "ieee1588-ptp-tt:cc-default",
	255: "ieee1588-ptp-tt:cc-time-receiver-only",
}

// clockAccuracyIdentity maps IEEE 1588 clockAccuracy values to
// ieee1588-ptp-tt identity names.  0xfe (unknown) has no identity and
// is omitted.
var clockAccuracyIdentity = map[uint8]string{
	0x17: "ieee1588-ptp-tt:ca-time-accurate-to-1000-fs",
	0x18: "ieee1588-ptp-tt:ca-time-accurate-to-2500-fs",
	0x19: "ieee1588-ptp-tt:ca-time-accurate-to-10-ps",
	0x1a: "ieee1588-ptp-tt:ca-time-accurate-to-25ps",
	0x1b: "ieee1588-ptp-tt:ca-time-accurate-to-100-ps",
	0x1c: "ieee1588-ptp-tt:ca-time-accurate-to-250-ps",
	0x1d: "ieee1588-ptp-tt:ca-time-accurate-to-1000-ps",
	0x1e: "ieee1588-ptp-tt:ca-time-accurate-to-2500-ps",
	0x1f: "ieee1588-ptp-tt:ca-time-accurate-to-10-ns",
	0x20: "ieee1588-ptp-tt:ca-time-accurate-to-25-ns",
	0x21: "ieee1588-ptp-tt:ca-time-accurate-to-100-ns",
	0x22: "ieee1588-ptp-tt:ca-time-accurate-to-250-ns",
	0x23: "ieee1588-ptp-tt:ca-time-accurate-to-1000-ns",
	0x24: "ieee1588-ptp-tt:ca-time-accurate-to-2500-ns",
	0x25: "ieee1588-ptp-tt:ca-time-accurate-to-10-us",
	0x26: "ieee1588-ptp-tt:ca-time-accurate-to-25-us",
	0x27: "ieee1588-ptp-tt:ca-time-accurate-to-100-us",
	0x28: "ieee1588-ptp-tt:ca-time-accurate-to-250-us",
	0x29: "ieee1588-ptp-tt:ca-time-accurate-to-1000-us",
	0x2a: "ieee1588-ptp-tt:ca-time-accurate-to-2500-us",
	0x2b: "ieee1588-ptp-tt:ca-time-accurate-to-10-ms",
	0x2c: "ieee1588-ptp-tt:ca-time-accurate-to-25-ms",
	0x2d: "ieee1588-ptp-tt:ca-time-accurate-to-100-ms",
	0x2e: "ieee1588-ptp-tt:ca-time-accurate-to-250-ms",
	0x2f: "ieee1588-ptp-tt:ca-time-accurate-to-1-s",
	0x30: "ieee1588-ptp-tt:ca-time-accurate-to-10-s",
	0x31: "ieee1588-ptp-tt:ca-time-accurate-to-gt-10-s",
}

var timeSourceIdentity = map[uint8]string{
	0x10: "ieee1588-ptp-tt:atomic-clock",
	0x20: "ieee1588-ptp-tt:gnss",
	0x30: "ieee1588-ptp-tt:terrestrial-radio",
	0x39: "ieee1588-ptp-tt:serial-time-code",
	0x40: "ieee1588-ptp-tt:ptp",
	0x50: "ieee1588-ptp-tt:ntp",
	0x60: "ieee1588-ptp-tt:hand-set",
	0x90: "ieee1588-ptp-tt:other",
	0xa0: "ieee1588-ptp-tt:internal-oscillator",
}

// portStateName maps IEEE 1588 portState values to YANG enum names.
var portStateName = map[uint8]string{
	1: "initializing",
	2: "faulty",
	3: "disabled",
	4: "listening",
	5: "pre-time-transmitter",
	6: "time-transmitter",
	7: "passive",
	8: "uncalibrated",
	9: "time-receiver",
}

func timeSourceName(ts uint8) string {
	if s, ok := timeSourceIdentity[ts]; ok {
		return s
	}
	return "ieee1588-ptp-tt:internal-oscillator"
}

func delayMechanismName(dm uint8) string {
	switch dm {
	case 2:
		return "p2p"
	case 0: /* linuxptp DM_AUTO */
		return "no-mechanism"
	default:
		return "e2e"
	}
}

// fmtClockIdentity renders a clock identity in the YANG clock-identity
// pattern [0-9A-F]{2}(-[0-9A-F]{2}){7}, e.g. "00-51-82-FF-FE-11-22-02".
func fmtClockIdentity(cid ptp.ClockIdentity) string {
	b := make([]byte, 0, 23)
	for i := 7; i >= 0; i-- {
		if len(b) > 0 {
			b = append(b, '-')
		}
		b = append(b, fmt.Sprintf("%02X", uint8(uint64(cid)>>(8*i)))...)
	}
	return string(b)
}

func fmtPortIdentity(pid ptp.PortIdentity) map[string]interface{} {
	return map[string]interface{}{
		"clock-identity": fmtClockIdentity(pid.ClockIdentity),
		"port-number":    int(pid.PortNumber),
	}
}

// timeIntervalString renders a raw IEEE 1588 TimeInterval (nanoseconds
// scaled by 2^16) for a YANG time-interval leaf.  RFC 7951 requires
// int64 to be JSON-encoded as a string.
func timeIntervalString(ti ptp.TimeInterval) string {
	return strconv.FormatInt(int64(ti), 10)
}

func clockQualityMap(cq ptp.ClockQuality) map[string]interface{} {
	out := map[string]interface{}{}
	if cc, ok := clockClassIdentity[uint8(cq.ClockClass)]; ok {
		out["clock-class"] = cc
	}
	if ca, ok := clockAccuracyIdentity[uint8(cq.ClockAccuracy)]; ok {
		out["clock-accuracy"] = ca
	}
	out["offset-scaled-log-variance"] = int(cq.OffsetScaledLogVariance)
	return out
}

func defaultDSMap(d *ptp.DefaultDataSetTLV, instanceType string) map[string]interface{} {
	ds := map[string]interface{}{
		"clock-identity":     fmtClockIdentity(d.ClockIdentity),
		"number-ports":       int(d.NumberPorts),
		"clock-quality":      clockQualityMap(d.ClockQuality),
		"priority1":          int(d.Priority1),
		"priority2":          int(d.Priority2),
		"domain-number":      int(d.DomainNumber),
		"time-receiver-only": d.SoTSC&flagDefaultDSSOnly != 0,
	}
	if instanceType != "" {
		ds["instance-type"] = instanceType
	}
	return ds
}

func currentDSMap(d *ptp.CurrentDataSetTLV) map[string]interface{} {
	return map[string]interface{}{
		"steps-removed":                int(d.StepsRemoved),
		"offset-from-time-transmitter": timeIntervalString(d.OffsetFromMaster),
		"mean-delay":                   timeIntervalString(d.MeanPathDelay),
	}
}

func parentDSMap(d *ptp.ParentDataSetTLV) map[string]interface{} {
	return map[string]interface{}{
		"parent-port-identity": fmtPortIdentity(d.ParentPortIdentity),
		"parent-stats":         d.PS&1 != 0,
		"observed-parent-offset-scaled-log-variance": int(d.ObservedParentOffsetScaledLogVariance),
		"observed-parent-clock-phase-change-rate":    int64(int32(d.ObservedParentClockPhaseChangeRate)),
		"grandmaster-identity":                       fmtClockIdentity(d.GrandmasterIdentity),
		"grandmaster-clock-quality":                  clockQualityMap(d.GrandmasterClockQuality),
		"grandmaster-priority1":                      int(d.GrandmasterPriority1),
		"grandmaster-priority2":                      int(d.GrandmasterPriority2),
	}
}

func timePropertiesDSMap(d *timePropertiesDataSetTLV) map[string]interface{} {
	ds := map[string]interface{}{
		"leap61":                   d.Flags&flagLeap61 != 0,
		"leap59":                   d.Flags&flagLeap59 != 0,
		"current-utc-offset-valid": d.Flags&flagUtcOffValid != 0,
		"ptp-timescale":            d.Flags&flagPtpTimescale != 0,
		"time-traceable":           d.Flags&flagTimeTraceable != 0,
		"frequency-traceable":      d.Flags&flagFreqTraceable != 0,
		"time-source":              timeSourceName(uint8(d.TimeSource)),
	}
	// current-utc-offset has a when-condition on
	// current-utc-offset-valid = 'true'
	if d.Flags&flagUtcOffValid != 0 {
		ds["current-utc-offset"] = int(d.CurrentUtcOffset)
	}
	return ds
}

func portDSMap(d *portDataSetTLV) map[string]interface{} {
	state, ok := portStateName[d.PortState]
	if !ok {
		state = "disabled"
	}
	return map[string]interface{}{
		"port-identity":               fmtPortIdentity(d.PortIdentity),
		"port-state":                  state,
		"log-min-delay-req-interval":  int(d.LogMinDelayReqInterval),
		"mean-link-delay":             timeIntervalString(d.PeerMeanPathDelay),
		"log-announce-interval":       int(d.LogAnnounceInterval),
		"announce-receipt-timeout":    int(d.AnnounceReceiptTimeout),
		"log-sync-interval":           int(d.LogSyncInterval),
		"delay-mechanism":             delayMechanismName(d.DelayMechanism),
		"log-min-pdelay-req-interval": int(d.LogMinPdelayReqInterval),
		"version-number":              int(d.VersionNumber),
	}
}

// IEEE 1588 message type indices in the PORT_STATS_NP counter arrays.
const (
	msgSync               = 0x0
	msgPdelayReq          = 0x2
	msgPdelayResp         = 0x3
	msgFollowUp           = 0x8
	msgPdelayRespFollowUp = 0xA
	msgAnnounce           = 0xB
)

// portStatsMap maps PORT_STATS_NP counters to the ieee802-dot1as-gptp
// port-statistics-ds counters.
func portStatsMap(s *ptp.PortStatsNPTLV) map[string]interface{} {
	rx := s.PortStats.RXMsgType
	tx := s.PortStats.TXMsgType
	return map[string]interface{}{
		"rx-sync-count":                  rx[msgSync],
		"rx-follow-up-count":             rx[msgFollowUp],
		"rx-pdelay-req-count":            rx[msgPdelayReq],
		"rx-pdelay-resp-count":           rx[msgPdelayResp],
		"rx-pdelay-resp-follow-up-count": rx[msgPdelayRespFollowUp],
		"rx-announce-count":              rx[msgAnnounce],
		"tx-sync-count":                  tx[msgSync],
		"tx-follow-up-count":             tx[msgFollowUp],
		"tx-pdelay-req-count":            tx[msgPdelayReq],
		"tx-pdelay-resp-count":           tx[msgPdelayResp],
		"tx-pdelay-resp-follow-up-count": tx[msgPdelayRespFollowUp],
		"tx-announce-count":              tx[msgAnnounce],
	}
}
