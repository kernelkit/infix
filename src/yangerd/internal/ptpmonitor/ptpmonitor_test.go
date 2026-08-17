package ptpmonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

// reply wraps a response TLV in a management packet and marshals it to
// wire format, the way ptp4l would.
func reply(t *testing.T, tlv ptp.ManagementTLV) []byte {
	t.Helper()

	headerSize := uint16(48) /* binary.Size(ManagementMsgHead{}) */
	pkt := &ptp.Management{
		ManagementMsgHead: ptp.ManagementMsgHead{
			Header: ptp.Header{
				SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageManagement, 0),
				Version:            ptp.Version,
				MessageLength:      headerSize,
				LogMessageInterval: ptp.MgmtLogMessageInterval,
			},
			TargetPortIdentity: ptp.DefaultTargetPortIdentity,
			ActionField:        ptp.RESPONSE,
		},
		TLV: tlv,
	}
	b, err := pkt.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal reply: %v", err)
	}
	return b
}

func defaultDSFixture() *ptp.DefaultDataSetTLV {
	size := uint16(24 + 4) /* payload + TLV head, unused by decode */
	return &ptp.DefaultDataSetTLV{
		ManagementTLVHead: ptp.ManagementTLVHead{
			TLVHead:      ptp.TLVHead{TLVType: ptp.TLVManagement, LengthField: size},
			ManagementID: ptp.IDDefaultDataSet,
		},
		SoTSC:       2, /* slaveOnly */
		NumberPorts: 1,
		Priority1:   128,
		Priority2:   127,
		ClockQuality: ptp.ClockQuality{
			ClockClass:              ptp.ClockClass(248),
			ClockAccuracy:           ptp.ClockAccuracy(0x21),
			OffsetScaledLogVariance: 0xfffe,
		},
		ClockIdentity: 0x005182FFFE112202,
		DomainNumber:  0,
	}
}

func TestDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		tlv  ptp.ManagementTLV
	}{
		{"default-ds", defaultDSFixture()},
		{
			"current-ds",
			&ptp.CurrentDataSetTLV{
				ManagementTLVHead: ptp.ManagementTLVHead{
					TLVHead:      ptp.TLVHead{TLVType: ptp.TLVManagement},
					ManagementID: ptp.IDCurrentDataSet,
				},
				StepsRemoved:     1,
				OffsetFromMaster: ptp.TimeInterval(42 << 16),
				MeanPathDelay:    ptp.TimeInterval(1000 << 16),
			},
		},
		{
			"port-ds",
			&portDataSetTLV{
				ManagementTLVHead: ptp.ManagementTLVHead{
					TLVHead:      ptp.TLVHead{TLVType: ptp.TLVManagement},
					ManagementID: ptp.IDPortDataSet,
				},
				PortIdentity: ptp.PortIdentity{
					ClockIdentity: 0x005182FFFE112202,
					PortNumber:    1,
				},
				PortState:      9, /* SLAVE */
				DelayMechanism: 1, /* E2E */
				VersionNumber:  2,
			},
		},
		{
			"time-properties-ds",
			&timePropertiesDataSetTLV{
				ManagementTLVHead: ptp.ManagementTLVHead{
					TLVHead:      ptp.TLVHead{TLVType: ptp.TLVManagement},
					ManagementID: ptp.IDTimePropertiesDataSet,
				},
				CurrentUtcOffset: 37,
				Flags:            flagUtcOffValid | flagPtpTimescale,
				TimeSource:       ptp.TimeSource(0xa0),
			},
		},
		{
			"time-status-np",
			&ptp.TimeStatusNPTLV{
				ManagementTLVHead: ptp.ManagementTLVHead{
					TLVHead:      ptp.TLVHead{TLVType: ptp.TLVManagement},
					ManagementID: ptp.IDTimeStatusNP,
				},
				MasterOffsetNS: -1234,
				GMPresent:      1,
				GMIdentity:     0x005182FFFE112202,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodePacket(reply(t, tc.tlv))
			if err != nil {
				t.Fatalf("decodePacket: %v", err)
			}
			if got.MgmtID() != tc.tlv.MgmtID() {
				t.Fatalf("management ID: got 0x%04x, want 0x%04x",
					uint16(got.MgmtID()), uint16(tc.tlv.MgmtID()))
			}
		})
	}
}

func TestDecodePortStatsEndianness(t *testing.T) {
	tlv := &ptp.PortStatsNPTLV{
		ManagementTLVHead: ptp.ManagementTLVHead{
			TLVHead:      ptp.TLVHead{TLVType: ptp.TLVManagement},
			ManagementID: ptp.IDPortStatsNP,
		},
		PortIdentity: ptp.PortIdentity{ClockIdentity: 1, PortNumber: 1},
	}
	tlv.PortStats.RXMsgType[msgSync] = 42
	tlv.PortStats.TXMsgType[msgAnnounce] = 7

	// PortStatsNPTLV.MarshalBinary writes counters in host byte
	// order, exactly like ptp4l
	got, err := decodePacket(reply(t, tlv))
	if err != nil {
		t.Fatalf("decodePacket: %v", err)
	}
	stats, ok := got.(*ptp.PortStatsNPTLV)
	if !ok {
		t.Fatalf("got %T", got)
	}
	if stats.PortStats.RXMsgType[msgSync] != 42 || stats.PortStats.TXMsgType[msgAnnounce] != 7 {
		t.Fatalf("counters mangled: rx=%d tx=%d",
			stats.PortStats.RXMsgType[msgSync], stats.PortStats.TXMsgType[msgAnnounce])
	}
}

func TestFmtClockIdentity(t *testing.T) {
	got := fmtClockIdentity(ptp.ClockIdentity(0x005182FFFE112202))
	want := "00-51-82-FF-FE-11-22-02"
	if got != want {
		t.Fatalf("fmtClockIdentity: got %q, want %q", got, want)
	}
}

func TestCurrentDSMapRFC7951(t *testing.T) {
	ds := currentDSMap(&ptp.CurrentDataSetTLV{
		StepsRemoved:     1,
		OffsetFromMaster: ptp.TimeInterval(-42 << 16),
		MeanPathDelay:    ptp.TimeInterval(1000 << 16),
	})

	// int64 leafs must be JSON strings (RFC 7951)
	if ds["offset-from-time-transmitter"] != fmt.Sprint(-42<<16) {
		t.Fatalf("offset: got %v", ds["offset-from-time-transmitter"])
	}
	if ds["mean-delay"] != fmt.Sprint(1000<<16) {
		t.Fatalf("mean-delay: got %v", ds["mean-delay"])
	}
	if ds["steps-removed"] != 1 {
		t.Fatalf("steps-removed: got %v", ds["steps-removed"])
	}
}

func TestPortDSMapStates(t *testing.T) {
	tests := []struct {
		state uint8
		mech  uint8
		want  string
		mechS string
	}{
		{6, 1, "time-transmitter", "e2e"},
		{9, 2, "time-receiver", "p2p"},
		{4, 0, "listening", "no-mechanism"},
		{99, 1, "disabled", "e2e"}, /* unknown state */
	}

	for _, tc := range tests {
		ds := portDSMap(&portDataSetTLV{PortState: tc.state, DelayMechanism: tc.mech})
		if ds["port-state"] != tc.want {
			t.Fatalf("state %d: got %v, want %v", tc.state, ds["port-state"], tc.want)
		}
		if ds["delay-mechanism"] != tc.mechS {
			t.Fatalf("mech %d: got %v, want %v", tc.mech, ds["delay-mechanism"], tc.mechS)
		}
	}
}

func TestTimePropertiesUtcOffsetWhenCondition(t *testing.T) {
	// current-utc-offset must be omitted unless
	// current-utc-offset-valid is true (YANG when-condition)
	ds := timePropertiesDSMap(&timePropertiesDataSetTLV{CurrentUtcOffset: 37})
	if _, ok := ds["current-utc-offset"]; ok {
		t.Fatal("current-utc-offset present despite valid=false")
	}

	ds = timePropertiesDSMap(&timePropertiesDataSetTLV{
		CurrentUtcOffset: 37,
		Flags:            flagUtcOffValid,
	})
	if ds["current-utc-offset"] != 37 {
		t.Fatalf("current-utc-offset: got %v", ds["current-utc-offset"])
	}
}

func TestInstanceTypeFromConf(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		conf string
		want string
	}{
		{"[global]\nclockType  BOUNDARY_CLOCK\n[e1]\n[e2]\n", "bc"},
		{"[global]\nclockType  E2E_TC\n", "e2e-tc"},
		{"[global]\nclockType  P2P_TC\n", "p2p-tc"},
		{"[global]\nuds_address /var/run/ptp4l-0\n[e1]\n", ""},
	}

	for i, tc := range tests {
		path := filepath.Join(dir, fmt.Sprintf("ptp4l-%d.conf", i))
		os.WriteFile(path, []byte(tc.conf), 0644)
		if got := instanceTypeFromConf(path); got != tc.want {
			t.Fatalf("conf %d: got %q, want %q", i, got, tc.want)
		}
	}
}

func TestPortInterfaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ptp4l-0.conf")
	os.WriteFile(path, []byte("[global]\nfoo 1\n[e1]\n[e2]\nbar 2\n"), 0644)

	got := portInterfaces(path)
	if len(got) != 2 || got[0] != "e1" || got[1] != "e2" {
		t.Fatalf("portInterfaces: got %v", got)
	}
}

// fakePtp4l answers management requests with canned replies, mimicking
// a running ptp4l instance.  Like the real thing, it silently drops
// requests whose transport-specific nibble does not match sdo.
func fakePtp4l(t *testing.T, sock string, sdo uint8) {
	t.Helper()

	addr := &net.UnixAddr{Name: sock, Net: "unixgram"}
	conn, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		t.Fatalf("fake ptp4l listen: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	go func() {
		buf := make([]byte, 2048)
		for {
			n, raddr, err := conn.ReadFromUnix(buf)
			if err != nil {
				return
			}
			if n < 1 || buf[0]>>4 != sdo {
				continue
			}
			req, err := decodePacket(buf[:n])
			if err != nil {
				continue
			}

			var resp ptp.ManagementTLV
			switch req.MgmtID() {
			case ptp.IDDefaultDataSet:
				resp = defaultDSFixture()
			case ptp.IDCurrentDataSet:
				resp = &ptp.CurrentDataSetTLV{
					ManagementTLVHead: ptp.ManagementTLVHead{
						TLVHead:      ptp.TLVHead{TLVType: ptp.TLVManagement},
						ManagementID: ptp.IDCurrentDataSet,
					},
					StepsRemoved:     1,
					OffsetFromMaster: ptp.TimeInterval(42 << 16),
				}
			case ptp.IDPortDataSet:
				resp = &portDataSetTLV{
					ManagementTLVHead: ptp.ManagementTLVHead{
						TLVHead:      ptp.TLVHead{TLVType: ptp.TLVManagement},
						ManagementID: ptp.IDPortDataSet,
					},
					PortIdentity: ptp.PortIdentity{
						ClockIdentity: 0x005182FFFE112202,
						PortNumber:    1,
					},
					PortState:      9,
					DelayMechanism: 1,
					VersionNumber:  2,
				}
			case idSubscribeEventsNP:
				resp = &subscribeEventsNPTLV{
					ManagementTLVHead: ptp.ManagementTLVHead{
						TLVHead:      ptp.TLVHead{TLVType: ptp.TLVManagement},
						ManagementID: idSubscribeEventsNP,
					},
				}
			default:
				continue
			}

			pkt := &ptp.Management{
				ManagementMsgHead: ptp.ManagementMsgHead{
					Header: ptp.Header{
						SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageManagement, 0),
						Version:            ptp.Version,
						LogMessageInterval: ptp.MgmtLogMessageInterval,
					},
					TargetPortIdentity: ptp.DefaultTargetPortIdentity,
					ActionField:        ptp.RESPONSE,
				},
				TLV: resp,
			}
			b, err := pkt.MarshalBinary()
			if err != nil {
				continue
			}
			conn.WriteToUnix(b, raddr)
		}
	}()
}

func TestTransportSpecificFromConf(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		conf string
		want uint8
	}{
		{"[global]\ntransportSpecific       1\n[e1]\n", 1},
		{"[global]\ntransportSpecific       0x1\n[e1]\n", 1},
		{"[global]\ntransportSpecific       0\n[e1]\n", 0},
		{"[global]\n[e1]\n", 0},
	}

	for i, tc := range tests {
		path := filepath.Join(dir, fmt.Sprintf("ptp4l-%d.conf", i))
		os.WriteFile(path, []byte(tc.conf), 0644)
		if got := transportSpecificFromConf(path); got != tc.want {
			t.Fatalf("conf %d: got %d, want %d", i, got, tc.want)
		}
	}
}

func monitorEndToEnd(t *testing.T, transportSpecific string, sdo uint8) {
	dir := t.TempDir()
	oldConf, oldSock := confDir, sockDir
	confDir, sockDir = dir, dir
	t.Cleanup(func() { confDir, sockDir = oldConf, oldSock })

	os.WriteFile(filepath.Join(dir, "ptp4l-0.conf"),
		[]byte("[global]\ntransportSpecific "+transportSpecific+
			"\nuds_address "+dir+"/ptp4l-0\n[e1]\n"), 0644)
	fakePtp4l(t, filepath.Join(dir, "ptp4l-0"), sdo)

	tr := tree.New()
	m := New(tr, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw := tr.Get(treeKey)
		if raw == nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		var out struct {
			Instances struct {
				Instance []struct {
					InstanceIndex int `json:"instance-index"`
					DefaultDS     struct {
						ClockIdentity string `json:"clock-identity"`
						InstanceType  string `json:"instance-type"`
					} `json:"default-ds"`
					CurrentDS struct {
						Offset string `json:"offset-from-time-transmitter"`
					} `json:"current-ds"`
					Ports struct {
						Port []struct {
							PortIndex  int    `json:"port-index"`
							Underlying string `json:"underlying-interface"`
							PortDS     struct {
								PortState string `json:"port-state"`
							} `json:"port-ds"`
						} `json:"port"`
					} `json:"ports"`
				} `json:"instance"`
			} `json:"instances"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		insts := out.Instances.Instance
		if len(insts) == 1 &&
			insts[0].DefaultDS.ClockIdentity == "00-51-82-FF-FE-11-22-02" &&
			len(insts[0].Ports.Port) == 1 {
			inst := insts[0]
			if inst.DefaultDS.InstanceType != "oc" {
				t.Fatalf("instance-type: got %q", inst.DefaultDS.InstanceType)
			}
			if inst.CurrentDS.Offset != fmt.Sprint(42<<16) {
				t.Fatalf("offset: got %q", inst.CurrentDS.Offset)
			}
			port := inst.Ports.Port[0]
			if port.PortDS.PortState != "time-receiver" {
				t.Fatalf("port-state: got %q", port.PortDS.PortState)
			}
			if port.Underlying != "e1" {
				t.Fatalf("underlying-interface: got %q", port.Underlying)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("tree never converged, last: %s", tr.Get(treeKey))
}

func TestMonitorEndToEnd(t *testing.T) {
	monitorEndToEnd(t, "0", 0)
}

// gPTP instances only answer management messages carrying the 0x1
// transport-specific nibble.
func TestMonitorEndToEndGPTP(t *testing.T) {
	monitorEndToEnd(t, "1", 1)
}
