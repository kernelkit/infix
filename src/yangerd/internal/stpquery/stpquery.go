// Package stpquery provides a native Go client for querying mstpd's
// operational data over its abstract Unix datagram socket.  It decodes
// the binary wire protocol directly — no subprocess, no CGo.
package stpquery

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	cmdGetCISTBridgeStatus = 101
	cmdGetCISTPortStatus   = 105
	serverSocketName       = ".mstp_server"
)

type ctlMsgHdr struct {
	Cmd, Lin, Lout, Llog, Res int32
}

const hdrSize = 20

type Client struct {
	fd int
}

// sockaddrUN is the full-size struct sockaddr_un used by mstpd.
// mstpd passes sizeof(struct sockaddr_un) to bind/connect, so the
// abstract name is zero-padded to fill the entire sun_path[108].
// Go's net package uses minimal length, which produces a different
// abstract socket name.  We must match mstpd's behavior exactly.
type sockaddrUN struct {
	Family uint16
	Path   [108]byte
}

func setSockAddr(sa *sockaddrUN, name string) {
	sa.Family = syscall.AF_UNIX
	copy(sa.Path[1:], name)
}

var connSeq atomic.Uint64

func New() (*Client, error) {
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return nil, fmt.Errorf("socket: %w", err)
	}

	seq := connSeq.Add(1)
	var local sockaddrUN
	setSockAddr(&local, fmt.Sprintf("MSTPCTL_%d_%d", os.Getpid(), seq))
	_, _, errno := syscall.Syscall(syscall.SYS_BIND, uintptr(fd),
		uintptr(unsafe.Pointer(&local)), unsafe.Sizeof(local))
	if errno != 0 {
		syscall.Close(fd)
		return nil, fmt.Errorf("bind: %w", errno)
	}

	var remote sockaddrUN
	setSockAddr(&remote, serverSocketName)
	_, _, errno = syscall.Syscall(syscall.SYS_CONNECT, uintptr(fd),
		uintptr(unsafe.Pointer(&remote)), unsafe.Sizeof(remote))
	if errno != 0 {
		syscall.Close(fd)
		return nil, fmt.Errorf("connect to mstpd: %w", errno)
	}

	return &Client{fd: fd}, nil
}

func (c *Client) Close() error {
	if c.fd >= 0 {
		err := syscall.Close(c.fd)
		c.fd = -1
		return err
	}
	return nil
}

func (c *Client) roundTrip(cmd int32, in []byte, outSize int) ([]byte, error) {
	hdr := ctlMsgHdr{
		Cmd:  cmd,
		Lin:  int32(len(in)),
		Lout: int32(outSize),
	}

	buf := make([]byte, hdrSize+len(in))
	binary.NativeEndian.PutUint32(buf[0:4], uint32(hdr.Cmd))
	binary.NativeEndian.PutUint32(buf[4:8], uint32(hdr.Lin))
	binary.NativeEndian.PutUint32(buf[8:12], uint32(hdr.Lout))
	binary.NativeEndian.PutUint32(buf[12:16], uint32(hdr.Llog))
	binary.NativeEndian.PutUint32(buf[16:20], uint32(hdr.Res))
	copy(buf[hdrSize:], in)

	tv := syscall.Timeval{Sec: 5}
	syscall.SetsockoptTimeval(c.fd, syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, &tv)
	syscall.SetsockoptTimeval(c.fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv)

	if err := syscall.Sendmsg(c.fd, buf, nil, nil, 0); err != nil {
		return nil, fmt.Errorf("write to mstpd: %w", err)
	}

	resp := make([]byte, hdrSize+outSize+4096)
	n, _, _, _, err := syscall.Recvmsg(c.fd, resp, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("read from mstpd: %w", err)
	}
	if n < hdrSize {
		return nil, fmt.Errorf("mstpd response too short: %d bytes", n)
	}

	resCode := int32(binary.NativeEndian.Uint32(resp[16:20]))
	if resCode != 0 {
		return nil, fmt.Errorf("mstpd error: res=%d", resCode)
	}

	lout := int(int32(binary.NativeEndian.Uint32(resp[8:12])))
	if n < hdrSize+lout {
		return nil, fmt.Errorf("mstpd response truncated: got %d, need %d", n, hdrSize+lout)
	}

	return resp[hdrSize : hdrSize+lout], nil
}

// CISTBridgeStatus holds decoded bridge-level STP data from mstpd.
type CISTBridgeStatus struct {
	BridgeID                BridgeID
	TimeSinceTopologyChange uint32
	TopologyChangeCount     uint32
	TopologyChange          bool
	TopologyChangePort      string // max 16 chars
	LastTopologyChangePort  string // max 16 chars
	DesignatedRoot          BridgeID
	RootPathCost            uint32
	RootPortID              PortID
	RootMaxAge              uint8
	RootForwardDelay        uint8
	BridgeMaxAge            uint8
	BridgeForwardDelay      uint8
	TxHoldCount             uint32
	ProtocolVersion         uint32
	RegionalRoot            BridgeID
	InternalPathCost        uint32
	Enabled                 bool
	AgeingTime              uint32
	MaxHops                 uint8
	BridgeHelloTime         uint8
	RootPortName            string // from get_cist_bridge_status_OUT tail
}

// CISTPortStatus holds decoded port-level STP data from mstpd.
type CISTPortStatus struct {
	Uptime                    uint32
	State                     uint32
	PortID                    PortID
	AdminExternalPortPathCost uint32
	ExternalPortPathCost      uint32
	DesignatedRoot            BridgeID
	DesignatedExternalCost    uint32
	DesignatedBridge          BridgeID
	DesignatedPort            PortID
	TcAck                     bool
	PortHelloTime             uint8
	AdminEdgePort             bool
	AutoEdgePort              bool
	OperEdgePort              bool
	Enabled                   bool
	AdminP2P                  uint32
	OperP2P                   bool
	RestrictedRole            bool
	RestrictedTCN             bool
	Role                      uint32
	Disputed                  bool
	DesignatedRegionalRoot    BridgeID
	DesignatedInternalCost    uint32
	AdminInternalPortPathCost uint32
	InternalPortPathCost      uint32
	BPDUGuardPort             bool
	BPDUGuardError            bool
	BPDUFilterPort            bool
	NetworkPort               bool
	BAInconsistent            bool
	NumRxBPDUFiltered         uint32
	NumRxBPDU                 uint32
	NumRxTCN                  uint32
	NumTxBPDU                 uint32
	NumTxTCN                  uint32
	NumTransFwd               uint32
	NumTransBlk               uint32
	RcvdBpdu                  bool
	RcvdRSTP                  bool
	RcvdSTP                   bool
	RcvdTcAck                 bool
	RcvdTcn                   bool
	SendRSTP                  bool
}

// BridgeID is an 8-byte STP bridge identifier.
type BridgeID [8]byte

// Priority returns the 4-bit priority value (0-15).
func (b BridgeID) Priority() int {
	return int(b[0]) >> 4
}

// SystemID returns the 12-bit system extension.
func (b BridgeID) SystemID() int {
	return (int(b[0])&0x0f)<<8 | int(b[1])
}

// Address returns the 6-byte MAC address as a colon-separated string.
func (b BridgeID) Address() string {
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", b[2], b[3], b[4], b[5], b[6], b[7])
}

// PortID is a 2-byte STP port identifier (big-endian on wire).
type PortID [2]byte

// Priority returns the 4-bit port priority (0-15).
func (p PortID) Priority() int {
	return int(p[0]) >> 4
}

// Number returns the 12-bit port number.
func (p PortID) Number() int {
	return (int(p[0])&0x0f)<<8 | int(p[1])
}

// GetBridgeStatus queries mstpd for CIST bridge status.
// brIndex is the kernel interface index of the bridge.
func (c *Client) GetBridgeStatus(brIndex int) (*CISTBridgeStatus, error) {
	// Input: 4-byte int32 br_index
	in := make([]byte, 4)
	binary.NativeEndian.PutUint32(in, uint32(int32(brIndex)))

	// Output: 128 bytes = 112 (CIST_BridgeStatus) + 16 (root_port_name)
	out, err := c.roundTrip(cmdGetCISTBridgeStatus, in, 128)
	if err != nil {
		return nil, err
	}
	if len(out) < 128 {
		return nil, fmt.Errorf("bridge status response too short: %d", len(out))
	}

	s := &CISTBridgeStatus{}
	copy(s.BridgeID[:], out[0:8])
	s.TimeSinceTopologyChange = binary.NativeEndian.Uint32(out[8:12])
	s.TopologyChangeCount = binary.NativeEndian.Uint32(out[12:16])
	s.TopologyChange = out[16] != 0
	s.TopologyChangePort = cString(out[17:33])
	s.LastTopologyChangePort = cString(out[33:49])
	copy(s.DesignatedRoot[:], out[56:64])
	s.RootPathCost = binary.NativeEndian.Uint32(out[64:68])
	s.RootPortID = PortID{out[68], out[69]}
	s.RootMaxAge = out[70]
	s.RootForwardDelay = out[71]
	s.BridgeMaxAge = out[72]
	s.BridgeForwardDelay = out[73]
	s.TxHoldCount = binary.NativeEndian.Uint32(out[76:80])
	s.ProtocolVersion = binary.NativeEndian.Uint32(out[80:84])
	copy(s.RegionalRoot[:], out[88:96])
	s.InternalPathCost = binary.NativeEndian.Uint32(out[96:100])
	s.Enabled = out[100] != 0
	s.AgeingTime = binary.NativeEndian.Uint32(out[104:108])
	s.MaxHops = out[108]
	s.BridgeHelloTime = out[109]
	// Bytes 112..127 = root_port_name[16]
	s.RootPortName = cString(out[112:128])

	return s, nil
}

// GetPortStatus queries mstpd for CIST port status.
// brIndex and portIndex are kernel interface indices.
func (c *Client) GetPortStatus(brIndex, portIndex int) (*CISTPortStatus, error) {
	// Input: 8 bytes = 2x int32
	in := make([]byte, 8)
	binary.NativeEndian.PutUint32(in[0:4], uint32(int32(brIndex)))
	binary.NativeEndian.PutUint32(in[4:8], uint32(int32(portIndex)))

	// Output: 136 bytes (CIST_PortStatus)
	out, err := c.roundTrip(cmdGetCISTPortStatus, in, 136)
	if err != nil {
		return nil, err
	}
	if len(out) < 136 {
		return nil, fmt.Errorf("port status response too short: %d", len(out))
	}

	s := &CISTPortStatus{}
	s.Uptime = binary.NativeEndian.Uint32(out[0:4])
	s.State = binary.NativeEndian.Uint32(out[4:8])
	s.PortID = PortID{out[8], out[9]}
	s.AdminExternalPortPathCost = binary.NativeEndian.Uint32(out[12:16])
	s.ExternalPortPathCost = binary.NativeEndian.Uint32(out[16:20])
	copy(s.DesignatedRoot[:], out[24:32])
	s.DesignatedExternalCost = binary.NativeEndian.Uint32(out[32:36])
	copy(s.DesignatedBridge[:], out[40:48])
	s.DesignatedPort = PortID{out[48], out[49]}
	s.TcAck = out[50] != 0
	s.PortHelloTime = out[51]
	s.AdminEdgePort = out[52] != 0
	s.AutoEdgePort = out[53] != 0
	s.OperEdgePort = out[54] != 0
	s.Enabled = out[55] != 0
	s.AdminP2P = binary.NativeEndian.Uint32(out[56:60])
	s.OperP2P = out[60] != 0
	s.RestrictedRole = out[61] != 0
	s.RestrictedTCN = out[62] != 0
	s.Role = binary.NativeEndian.Uint32(out[64:68])
	s.Disputed = out[68] != 0
	copy(s.DesignatedRegionalRoot[:], out[72:80])
	s.DesignatedInternalCost = binary.NativeEndian.Uint32(out[80:84])
	s.AdminInternalPortPathCost = binary.NativeEndian.Uint32(out[84:88])
	s.InternalPortPathCost = binary.NativeEndian.Uint32(out[88:92])
	s.BPDUGuardPort = out[92] != 0
	s.BPDUGuardError = out[93] != 0
	s.BPDUFilterPort = out[94] != 0
	s.NetworkPort = out[95] != 0
	s.BAInconsistent = out[96] != 0
	s.NumRxBPDUFiltered = binary.NativeEndian.Uint32(out[100:104])
	s.NumRxBPDU = binary.NativeEndian.Uint32(out[104:108])
	s.NumRxTCN = binary.NativeEndian.Uint32(out[108:112])
	s.NumTxBPDU = binary.NativeEndian.Uint32(out[112:116])
	s.NumTxTCN = binary.NativeEndian.Uint32(out[116:120])
	s.NumTransFwd = binary.NativeEndian.Uint32(out[120:124])
	s.NumTransBlk = binary.NativeEndian.Uint32(out[124:128])
	s.RcvdBpdu = out[128] != 0
	s.RcvdRSTP = out[129] != 0
	s.RcvdSTP = out[130] != 0
	s.RcvdTcAck = out[131] != 0
	s.RcvdTcn = out[132] != 0
	s.SendRSTP = out[133] != 0

	return s, nil
}

// cString extracts a NUL-terminated C string from a byte slice.
func cString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// protocolName maps mstpd protocol_version to YANG force-protocol value.
func protocolName(v uint32) string {
	switch v {
	case 0:
		return "stp"
	case 2:
		return "rstp"
	default:
		return "rstp"
	}
}

// roleName maps mstpd port role to YANG role value.
func roleName(v uint32) string {
	switch v {
	case 0:
		return "disabled"
	case 1:
		return "root"
	case 2:
		return "designated"
	case 3:
		return "alternate"
	case 4:
		return "backup"
	case 5:
		return "master"
	default:
		return "disabled"
	}
}

// bridgeIDMap returns a YANG bridge-id object.
func bridgeIDMap(b BridgeID) map[string]any {
	return map[string]any{
		"priority":  b.Priority(),
		"system-id": b.SystemID(),
		"address":   b.Address(),
	}
}

// portIDMap returns a YANG port-id object.
func portIDMap(p PortID) map[string]any {
	return map[string]any{
		"priority": p.Priority(),
		"port-id":  p.Number(),
	}
}

// IfIndexResolver looks up kernel interface indices by name.
type IfIndexResolver interface {
	IfIndex(name string) (int, bool)
}

// Query queries mstpd for STP data on all bridges found in the ip-json
// links data.  Returns per-bridge and per-port STP JSON fragments ready
// for merging into the YANG interface tree.
//
// Query connects to mstpd, queries STP data for all bridges in links,
// and returns per-bridge and per-port STP JSON fragments.  A fresh
// connection is established per call so that late-starting or restarted
// mstpd instances are handled gracefully.
func Query(links json.RawMessage, resolver IfIndexResolver) (bridgeSTP, portSTP map[string]json.RawMessage) {
	brs := findBridges(links)
	if len(brs) == 0 {
		return nil, nil
	}

	client, err := New()
	if err != nil {
		return nil, nil
	}
	defer client.Close()

	bridgeSTP = make(map[string]json.RawMessage)
	portSTP = make(map[string]json.RawMessage)

	for _, br := range brs {
		brIdx, ok := resolver.IfIndex(br.name)
		if !ok {
			continue
		}

		bs, err := client.GetBridgeStatus(brIdx)
		if err != nil {
			continue
		}

		stp := buildBridgeSTP(bs)
		if data, err := json.Marshal(stp); err == nil {
			bridgeSTP[br.name] = data
		}

		for _, port := range br.ports {
			portIdx, ok := resolver.IfIndex(port)
			if !ok {
				continue
			}
			ps, err := client.GetPortStatus(brIdx, portIdx)
			if err != nil {
				continue
			}
			pstp := buildPortSTP(ps)
			if data, err := json.Marshal(pstp); err == nil {
				portSTP[port] = data
			}
		}
	}

	return bridgeSTP, portSTP
}

func buildBridgeSTP(bs *CISTBridgeStatus) map[string]any {
	cist := map[string]any{
		"bridge-id": bridgeIDMap(bs.BridgeID),
		"root-id":   bridgeIDMap(bs.DesignatedRoot),
	}

	bid := bridgeIDMap(bs.BridgeID)
	if prio, ok := bid["priority"]; ok {
		cist["priority"] = prio
	}

	if bs.RootPortName != "" {
		cist["root-port"] = bs.RootPortName
	}

	if bs.TopologyChangeCount > 0 {
		tc := map[string]any{
			"count":       bs.TopologyChangeCount,
			"in-progress": bs.TopologyChange,
		}
		if bs.TopologyChangePort != "" {
			tc["port"] = bs.TopologyChangePort
		}
		if bs.TimeSinceTopologyChange > 0 {
			tc["time"] = time.Now().UTC().Add(-time.Duration(bs.TimeSinceTopologyChange) * time.Second).Format(time.RFC3339)
		}
		cist["topology-change"] = tc
	}

	stp := map[string]any{
		"force-protocol":      protocolName(bs.ProtocolVersion),
		"hello-time":          int(bs.BridgeHelloTime),
		"forward-delay":       int(bs.BridgeForwardDelay),
		"max-age":             int(bs.BridgeMaxAge),
		"transmit-hold-count": int(bs.TxHoldCount),
		"max-hops":            int(bs.MaxHops),
		"cist":                cist,
	}

	return stp
}

func buildPortSTP(ps *CISTPortStatus) map[string]any {
	cist := map[string]any{
		"port-id":            portIDMap(ps.PortID),
		"role":               roleName(ps.Role),
		"disputed":           ps.Disputed,
		"external-path-cost": int(ps.ExternalPortPathCost),
		"designated": map[string]any{
			"bridge-id": bridgeIDMap(ps.DesignatedBridge),
			"port-id":   portIDMap(ps.DesignatedPort),
		},
	}

	stp := map[string]any{
		"edge": ps.OperEdgePort,
		"cist": cist,
		"statistics": map[string]any{
			"in-bpdus":          strconv.FormatUint(uint64(ps.NumRxBPDU), 10),
			"in-bpdus-filtered": strconv.FormatUint(uint64(ps.NumRxBPDUFiltered), 10),
			"in-tcns":           strconv.FormatUint(uint64(ps.NumRxTCN), 10),
			"out-bpdus":         strconv.FormatUint(uint64(ps.NumTxBPDU), 10),
			"out-tcns":          strconv.FormatUint(uint64(ps.NumTxTCN), 10),
			"to-blocking":       strconv.FormatUint(uint64(ps.NumTransBlk), 10),
			"to-forwarding":     strconv.FormatUint(uint64(ps.NumTransFwd), 10),
		},
	}

	return stp
}

type bridgeInfo struct {
	name  string
	ports []string
}

func findBridges(links json.RawMessage) []bridgeInfo {
	var ifaces []map[string]any
	if json.Unmarshal(links, &ifaces) != nil {
		return nil
	}

	bridges := make(map[string]*bridgeInfo)
	for _, iface := range ifaces {
		linkinfo, _ := iface["linkinfo"].(map[string]any)
		if linkinfo == nil {
			continue
		}

		name, _ := iface["ifname"].(string)
		if name == "" {
			continue
		}

		if kind, _ := linkinfo["info_kind"].(string); kind == "bridge" {
			if bridges[name] == nil {
				bridges[name] = &bridgeInfo{name: name}
			}
		}

		if master, _ := iface["master"].(string); master != "" {
			br := bridges[master]
			if br == nil {
				br = &bridgeInfo{name: master}
				bridges[master] = br
			}
			br.ports = append(br.ports, name)
		}
	}

	var result []bridgeInfo
	for _, br := range bridges {
		if len(br.ports) > 0 {
			result = append(result, *br)
		}
	}
	return result
}

// LinksIfIndexResolver resolves interface names to indices from ip-json link data.
type LinksIfIndexResolver struct {
	idx map[string]int
}

// NewLinksIfIndexResolver builds a resolver from ip-json link data.
func NewLinksIfIndexResolver(links json.RawMessage) *LinksIfIndexResolver {
	r := &LinksIfIndexResolver{idx: make(map[string]int)}
	var ifaces []map[string]any
	if json.Unmarshal(links, &ifaces) != nil {
		return r
	}
	for _, iface := range ifaces {
		name, _ := iface["ifname"].(string)
		if name == "" {
			continue
		}
		switch v := iface["ifindex"].(type) {
		case float64:
			r.idx[name] = int(v)
		case int:
			r.idx[name] = v
		}
	}
	return r
}

// IfIndex returns the kernel interface index for the given name.
func (r *LinksIfIndexResolver) IfIndex(name string) (int, bool) {
	idx, ok := r.idx[name]
	return idx, ok
}

// FindBridges is exported for testing.  It extracts bridge info from
// ip-json link data.
func FindBridges(links json.RawMessage) []struct {
	Name  string
	Ports []string
} {
	brs := findBridges(links)
	out := make([]struct {
		Name  string
		Ports []string
	}, len(brs))
	for i, br := range brs {
		out[i].Name = br.name
		out[i].Ports = br.ports
	}
	return out
}
