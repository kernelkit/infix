package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

// NTPCollector gathers ietf-ntp operational data by running chronyc
// commands (sources, sourcestats, tracking, serverstats) and ss to
// detect the NTP listening port.
type NTPCollector struct {
	cmd      CommandRunner
	interval time.Duration
}

// NewNTPCollector creates an NTPCollector with the given dependencies.
func NewNTPCollector(cmd CommandRunner, interval time.Duration) *NTPCollector {
	return &NTPCollector{cmd: cmd, interval: interval}
}

// Name implements Collector.
func (c *NTPCollector) Name() string { return "ntp" }

// Interval implements Collector.
func (c *NTPCollector) Interval() time.Duration { return c.interval }

// Collect implements Collector.  It produces two tree keys:
//   - "ietf-ntp:ntp" — associations, clock state, server status, and
//     server statistics (RFC 9249).
//   - "ietf-system:system-state" — merged infix-system:ntp/sources/source
//     list with address, mode, state, stratum and poll for each chrony
//     source (Infix augmentation of ietf-system).
func (c *NTPCollector) Collect(ctx context.Context, t *tree.Tree) error {
	// Run chronyc sources once and share between addAssociations and addSources.
	sourcesOut, _ := c.cmd.Run(ctx, "chronyc", "-c", "sources")

	ntp := make(map[string]interface{})

	c.addAssociations(ctx, ntp, sourcesOut)
	c.addClockState(ctx, ntp)
	c.addServerStatus(ctx, ntp)
	c.addServerStats(ctx, ntp)

	if len(ntp) == 0 {
		return nil
	}

	if data, err := json.Marshal(ntp); err == nil {
		t.Set("ietf-ntp:ntp", data)
	}

	// Populate the Infix NTP sources augmentation under system-state.
	if sources := c.addSources(sourcesOut); sources != nil {
		if data, err := json.Marshal(map[string]interface{}{
			"infix-system:ntp": sources,
		}); err == nil {
			t.Merge("ietf-system:system-state", data)
		}
	}

	return nil
}

// addAssociations parses chronyc sources and sourcestats CSV output
// into the associations/association list.
//
// chronyc -c sources format (comma-separated):
//
//	[0] Mode: ^ server, = peer, # refclock (skipped)
//	[1] State: * selected, + candidate, - outlier, ? unusable, x falseticker, ~ unstable
//	[2] Address (IP)
//	[3] Stratum
//	[4] Poll interval (log2 seconds)
//	[5] Reach (octal reachability register)
//	[6] LastRx (seconds since last response)
//	[7] Last offset (seconds)
//	[8] Offset at last update (seconds)
//	[9] Error estimate (seconds)
//
// chronyc -c sourcestats format:
//
//	[0] Address
//	[1] NP
//	[2] NR
//	[3] Span
//	[4] Frequency (ppm)
//	[5] Freq Skew (ppm)
//	[6] Offset (seconds)
//	[7] Std Dev (seconds)
func (c *NTPCollector) addAssociations(ctx context.Context, ntp map[string]interface{}, sourcesOut []byte) {
	if len(sourcesOut) == 0 {
		return
	}

	// Build stats map from sourcestats for offset/dispersion
	statsMap := make(map[string]map[string]string)
	statsOut, err := c.cmd.Run(ctx, "chronyc", "-c", "sourcestats")
	if err == nil {
		for _, line := range splitLines(string(statsOut)) {
			parts := strings.Split(line, ",")
			if len(parts) >= 8 {
				statsMap[parts[0]] = map[string]string{
					"offset":  parts[6],
					"std_dev": parts[7],
				}
			}
		}
	}

	modeMap := map[string]string{
		"^": "ietf-ntp:client",
		"=": "ietf-ntp:active",
		"#": "ietf-ntp:broadcast-client",
	}

	var associations []interface{}
	for _, line := range splitLines(string(sourcesOut)) {
		parts := strings.Split(line, ",")
		if len(parts) < 10 {
			continue
		}

		modeIndicator := parts[0]
		// Skip reference clocks — they have names like "GPS", not IP addresses
		if modeIndicator == "#" {
			continue
		}

		stateIndicator := parts[1]
		address := parts[2]
		stratum, err := strconv.Atoi(parts[3])
		if err != nil {
			continue
		}
		// YANG requires stratum 1..16
		if stratum < 1 || stratum > 16 {
			continue
		}

		assoc := map[string]interface{}{
			"address":      address,
			"local-mode":   modeMap[modeIndicator],
			"isconfigured": true,
			"stratum":      stratum,
		}
		if assoc["local-mode"] == nil {
			assoc["local-mode"] = "ietf-ntp:client"
		}

		// Current sync source
		if stateIndicator == "*" {
			assoc["prefer"] = true
		}

		// Reachability register (octal → decimal)
		if reach, err := strconv.ParseInt(parts[5], 8, 32); err == nil {
			assoc["reach"] = int(reach)
		}

		// Poll interval (log2 seconds)
		if poll, err := strconv.Atoi(parts[4]); err == nil {
			assoc["poll"] = poll
		}

		// Time since last packet
		if now, err := strconv.Atoi(parts[6]); err == nil {
			assoc["now"] = now
		}

		// Offset: prefer sourcestats if available, else sources[7]
		// Convert seconds → milliseconds with 3 fraction digits
		if stats, ok := statsMap[address]; ok {
			if offsetSec, err := strconv.ParseFloat(stats["offset"], 64); err == nil {
				assoc["offset"] = fmt.Sprintf("%.3f", offsetSec*1000.0)
			}
		} else if offsetSec, err := strconv.ParseFloat(parts[7], 64); err == nil {
			assoc["offset"] = fmt.Sprintf("%.3f", offsetSec*1000.0)
		}

		// Delay: error estimate from sources[9], seconds → milliseconds
		if delaySec, err := strconv.ParseFloat(parts[9], 64); err == nil {
			assoc["delay"] = fmt.Sprintf("%.3f", math.Abs(delaySec)*1000.0)
		}

		// Dispersion: std_dev from sourcestats, seconds → milliseconds
		if stats, ok := statsMap[address]; ok {
			if dispSec, err := strconv.ParseFloat(stats["std_dev"], 64); err == nil {
				assoc["dispersion"] = fmt.Sprintf("%.3f", dispSec*1000.0)
			}
		}

		associations = append(associations, assoc)
	}

	if len(associations) > 0 {
		ntp["associations"] = map[string]interface{}{
			"association": associations,
		}
	}
}

// sourceStateMap maps chronyc source-state indicators to YANG
// infix-system source-state enum values.
var sourceStateMap = map[string]string{
	"*": "selected",
	"+": "candidate",
	"-": "outlier",
	"?": "unusable",
	"x": "falseticker",
	"~": "unstable",
}

// sourceModeMap maps chronyc mode indicators to YANG
// infix-system source-mode enum values.
var sourceModeMap = map[string]string{
	"^": "server",
	"=": "peer",
	"#": "local-clock",
}

// addSources builds the infix-system:ntp/sources/source list from
// chronyc -c sources output.  Reference clocks (mode #) and sources
// with invalid stratum are skipped, matching the Python yanger
// ietf_system.py add_ntp() behaviour.
func (c *NTPCollector) addSources(sourcesOut []byte) map[string]interface{} {
	if len(sourcesOut) == 0 {
		return nil
	}

	var sources []interface{}
	for _, line := range splitLines(string(sourcesOut)) {
		parts := strings.Split(line, ",")
		if len(parts) < 10 {
			continue
		}

		modeIndicator := parts[0]
		if modeIndicator == "#" {
			continue
		}

		stratum, err := strconv.Atoi(parts[3])
		if err != nil || stratum > 16 {
			continue
		}

		mode := sourceModeMap[modeIndicator]
		if mode == "" {
			mode = "server"
		}
		state := sourceStateMap[parts[1]]
		if state == "" {
			continue
		}

		src := map[string]interface{}{
			"address": parts[2],
			"mode":    mode,
			"state":   state,
			"stratum": stratum,
		}
		if poll, err := strconv.Atoi(parts[4]); err == nil {
			src["poll"] = poll
		}

		sources = append(sources, src)
	}

	if len(sources) == 0 {
		return nil
	}

	return map[string]interface{}{
		"sources": map[string]interface{}{
			"source": sources,
		},
	}
}

// addClockState parses chronyc tracking CSV output into the clock-state
// container.
//
// chronyc -c tracking format (comma-separated):
//
//	[0]  Ref-ID (hex IP, e.g. "C0A80101")
//	[1]  Ref-ID name (e.g. "router.local")
//	[2]  Stratum
//	[3]  Ref time (seconds since epoch)
//	[4]  System time offset (seconds)
//	[5]  Last offset (seconds)
//	[6]  RMS offset (seconds)
//	[7]  Frequency (ppm)
//	[8]  Residual frequency (ppm)
//	[9]  Skew (ppm)
//	[10] Root delay (seconds)
//	[11] Root dispersion (seconds)
//	[12] Update interval (seconds)
//	[13] Leap status (e.g. "Normal", "Not synchronised")
func (c *NTPCollector) addClockState(ctx context.Context, ntp map[string]interface{}) {
	out, err := c.cmd.Run(ctx, "chronyc", "-c", "tracking")
	if err != nil || len(out) == 0 {
		return
	}

	lines := splitLines(string(out))
	if len(lines) == 0 {
		return
	}

	parts := strings.Split(lines[0], ",")
	if len(parts) < 14 {
		return
	}

	ss := make(map[string]interface{})

	// Stratum: chronyd uses 0 for "not synchronized", YANG requires 1-16
	stratumRaw, _ := strconv.Atoi(parts[2])
	stratum := stratumRaw
	if stratum == 0 {
		stratum = 16
	}

	if stratum == 16 {
		ss["clock-state"] = "ietf-ntp:unsynchronized"
	} else {
		ss["clock-state"] = "ietf-ntp:synchronized"
	}
	ss["clock-stratum"] = stratum

	// Reference ID
	refidIP := parts[0]
	refidName := parts[1]
	if refidName != "" {
		// NTP refids are always 4 bytes; pad/truncate to exactly 4 chars
		padded := refidName + "    "
		ss["clock-refid"] = padded[:4]
	} else if len(refidIP) == 8 {
		a, e1 := strconv.ParseInt(refidIP[0:2], 16, 32)
		b, e2 := strconv.ParseInt(refidIP[2:4], 16, 32)
		cv, e3 := strconv.ParseInt(refidIP[4:6], 16, 32)
		d, e4 := strconv.ParseInt(refidIP[6:8], 16, 32)
		if e1 == nil && e2 == nil && e3 == nil && e4 == nil {
			ss["clock-refid"] = fmt.Sprintf("%d.%d.%d.%d", a, b, cv, d)
		} else {
			ss["clock-refid"] = refidIP
		}
	} else if refidIP != "" {
		ss["clock-refid"] = refidIP
	} else {
		ss["clock-refid"] = "0.0.0.0"
	}

	// Frequencies (ppm → Hz with nominal 1GHz)
	if freqPPM, err := strconv.ParseFloat(parts[7], 64); err == nil {
		nominal := 1000000000.0
		actual := nominal * (1.0 + freqPPM/1000000.0)
		ss["nominal-freq"] = fmt.Sprintf("%.4f", nominal)
		ss["actual-freq"] = fmt.Sprintf("%.4f", actual)
	}

	// Clock precision (fixed estimate, ~1µs)
	ss["clock-precision"] = -20

	// Clock offset (system-time column[4], seconds → milliseconds)
	if offsetSec, err := strconv.ParseFloat(parts[4], 64); err == nil {
		ss["clock-offset"] = fmt.Sprintf("%.3f", offsetSec*1000.0)
	}

	// Root delay (seconds → milliseconds)
	if rootDelay, err := strconv.ParseFloat(parts[10], 64); err == nil {
		ss["root-delay"] = fmt.Sprintf("%.3f", rootDelay*1000.0)
	}

	// Root dispersion (seconds → milliseconds)
	if rootDisp, err := strconv.ParseFloat(parts[11], 64); err == nil {
		ss["root-dispersion"] = fmt.Sprintf("%.3f", rootDisp*1000.0)
	}

	// Reference time (epoch seconds → ISO 8601)
	if refTime, err := strconv.ParseFloat(parts[3], 64); err == nil && refTime > 0 {
		sec := int64(refTime)
		nsec := int64((refTime - float64(sec)) * 1e9)
		t := time.Unix(sec, nsec).UTC()
		ss["reference-time"] = t.Format("2006-01-02T15:04:05.000") + "Z"
	}

	// Sync state based on leap status
	leapStatus := strings.TrimSpace(parts[13])
	if leapStatus == "Not synchronised" || stratum == 16 {
		ss["sync-state"] = "ietf-ntp:clock-never-set"
	} else {
		ss["sync-state"] = "ietf-ntp:clock-synchronized"
	}

	// Infix augmentations
	if lastOffset, err := strconv.ParseFloat(parts[5], 64); err == nil {
		ss["infix-ntp:last-offset"] = fmt.Sprintf("%.9f", lastOffset)
	}
	if rmsOffset, err := strconv.ParseFloat(parts[6], 64); err == nil {
		ss["infix-ntp:rms-offset"] = fmt.Sprintf("%.9f", rmsOffset)
	}
	if residualFreq, err := strconv.ParseFloat(parts[8], 64); err == nil {
		ss["infix-ntp:residual-freq"] = fmt.Sprintf("%.3f", residualFreq)
	}
	if skew, err := strconv.ParseFloat(parts[9], 64); err == nil {
		ss["infix-ntp:skew"] = fmt.Sprintf("%.3f", skew)
	}
	if updateInterval, err := strconv.ParseFloat(parts[12], 64); err == nil {
		ss["infix-ntp:update-interval"] = fmt.Sprintf("%.1f", updateInterval)
	}

	ntp["clock-state"] = map[string]interface{}{
		"system-status": ss,
	}
}

// addServerStatus adds the refclock-master stratum and listening port.
// Must be called after addClockState so clock-state is available.
func (c *NTPCollector) addServerStatus(ctx context.Context, ntp map[string]interface{}) {
	// Reuse stratum from clock-state if already populated
	if cs, ok := ntp["clock-state"].(map[string]interface{}); ok {
		if ss, ok := cs["system-status"].(map[string]interface{}); ok {
			if stratum, ok := ss["clock-stratum"]; ok {
				ntp["refclock-master"] = map[string]interface{}{
					"master-stratum": stratum,
				}
			}
		}
	}

	// Detect NTP listening port via ss
	ssOut, err := c.cmd.Run(ctx, "ss", "-ulnp")
	if err != nil {
		return
	}

	for _, line := range splitLines(string(ssOut)) {
		if !strings.Contains(line, "chronyd") {
			continue
		}
		// Skip loopback (command socket)
		if strings.Contains(line, "127.0.0.1") || strings.Contains(line, "[::1]") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 5 {
			localAddr := fields[3]
			idx := strings.LastIndex(localAddr, ":")
			if idx >= 0 {
				portStr := localAddr[idx+1:]
				if port, err := strconv.Atoi(portStr); err == nil {
					ntp["port"] = port
					break
				}
			}
		}
	}
}

// addServerStats parses chronyc serverstats CSV into ntp-statistics.
//
// chronyc -c serverstats format:
//
//	[0] NTP packets received
//	[1] NTP packets dropped
//	[2] Cmd packets received
//	[3] Cmd packets dropped
//	[4] Client log size active
//	[5] Client log memory
//	[6] Rate limit drops
//	[7] NTP packets sent
//	[8] NTP packets send fail
func (c *NTPCollector) addServerStats(ctx context.Context, ntp map[string]interface{}) {
	out, err := c.cmd.Run(ctx, "chronyc", "-c", "serverstats")
	if err != nil || len(out) == 0 {
		return
	}

	lines := splitLines(string(out))
	if len(lines) == 0 {
		return
	}

	parts := strings.Split(lines[0], ",")
	if len(parts) < 9 {
		return
	}

	stats := make(map[string]interface{})
	if v, err := strconv.Atoi(parts[0]); err == nil {
		stats["packet-received"] = v
	}
	if v, err := strconv.Atoi(parts[1]); err == nil {
		stats["packet-dropped"] = v
	}
	if v, err := strconv.Atoi(parts[7]); err == nil {
		stats["packet-sent"] = v
	}
	if v, err := strconv.Atoi(parts[8]); err == nil {
		stats["packet-sent-fail"] = v
	}

	if len(stats) > 0 {
		ntp["ntp-statistics"] = stats
	}
}

// splitLines splits text into non-empty lines.
func splitLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
