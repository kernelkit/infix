import subprocess

from .common import insert
from .host import HOST


def add_ntp_associations(out):
    """Add NTP association information from chronyc sources and sourcestats.

    The chronyc -c sources output is CSV with the following fields:
      [0] Mode indicator:
          ^  server (we're a client to this source)
          =  peer (symmetric mode)
          #  local reference clock (GPS, PPS, etc.) - skipped, no IP address
      [1] State indicator:
          *  selected (current sync source)
          +  candidate
          -  outlier
          ?  unusable
          x  falseticker
          ~  unstable
      [2] Address (IP address or refclock name like "GPS")
      [3] Stratum
      [4] Poll interval (log2 seconds)
      [5] Reach (octal reachability register)
      [6] LastRx (seconds since last response)
      [7] Last offset (seconds)
      [8] Offset at last update (seconds)
      [9] Error estimate (seconds)

    The chronyc -c sourcestats output is CSV with:
      [0] Address
      [1] NP (number of sample points)
      [2] NR (number of runs)
      [3] Span (seconds)
      [4] Frequency (ppm)
      [5] Freq Skew (ppm)
      [6] Offset (seconds)
      [7] Std Dev (seconds)
    """
    try:
        # Get basic source information
        sources_data = HOST.run_multiline(["chronyc", "-c", "sources"], [])
        if not sources_data:
            return

        # Get statistical information (offset, dispersion)
        stats_data = HOST.run_multiline(["chronyc", "-c", "sourcestats"], [])

        # Build a map of address -> stats for quick lookup
        stats_map = {}
        if stats_data:
            for line in stats_data:
                parts = line.split(',')
                if len(parts) >= 8:
                    address = parts[0]
                    stats_map[address] = {
                        "offset": parts[6],      # Estimated offset in seconds
                        "std_dev": parts[7]      # Standard deviation in seconds
                    }

        associations = []
        # Map chronyd mode indicators to ietf-ntp association-mode identities
        mode_map = {
            "^": "ietf-ntp:client",            # We're client to this server
            "=": "ietf-ntp:active",            # Peer mode (symmetric active)
            "#": "ietf-ntp:broadcast-client"   # Local refclock (closest match)
        }

        # chronyc -c sources format:
        # [0]=Mode, [1]=State, [2]=Address, [3]=Stratum, [4]=Poll, [5]=Reach,
        # [6]=LastRx, [7]=LastOffset, [8]=OffsetAtLastUpdate, [9]=Error
        for line in sources_data:
            parts = line.split(',')
            if len(parts) < 10:
                continue

            mode_indicator = parts[0]
            # Skip reference clocks (mode "#") as they have names like "GPS" instead of IP addresses
            if mode_indicator == "#":
                continue

            state_indicator = parts[1]
            address = parts[2]
            stratum = int(parts[3])

            # Skip sources with invalid stratum (0 means unreachable/not yet synced)
            # YANG model requires stratum to be in range 1..16
            if stratum < 1 or stratum > 16:
                continue

            assoc = {}
            assoc["address"] = address
            assoc["local-mode"] = mode_map.get(mode_indicator, "ietf-ntp:client")
            assoc["isconfigured"] = True
            assoc["stratum"] = stratum

            # Prefer indicator: * means current sync source
            if state_indicator == "*":
                assoc["prefer"] = True

            # Reachability register (octal string to decimal uint8)
            try:
                reach_octal = parts[5]
                assoc["reach"] = int(reach_octal, 8)
            except (ValueError, IndexError):
                pass

            # Poll interval (already in log2 seconds)
            try:
                assoc["poll"] = int(parts[4])
            except (ValueError, IndexError):
                pass

            # Time since last packet (now)
            try:
                assoc["now"] = int(parts[6])
            except (ValueError, IndexError):
                pass

            # Offset: prefer sourcestats data if available, otherwise use sources
            # Convert from seconds to milliseconds with 3 fraction-digits
            try:
                if address in stats_map:
                    offset_sec = float(stats_map[address]["offset"])
                else:
                    # Use last offset from sources output (parts[7])
                    offset_sec = float(parts[7])
                assoc["offset"] = f"{offset_sec * 1000.0:.3f}"
            except (ValueError, IndexError):
                pass

            # Delay: use error estimate from sources (parts[9])
            # Convert from seconds to milliseconds with 3 fraction-digits
            try:
                delay_sec = float(parts[9])
                # chronyd reports this as error bound, use absolute value
                assoc["delay"] = f"{abs(delay_sec) * 1000.0:.3f}"
            except (ValueError, IndexError):
                pass

            # Dispersion: use standard deviation from sourcestats
            # Convert from seconds to milliseconds with 3 fraction-digits
            try:
                if address in stats_map:
                    disp_sec = float(stats_map[address]["std_dev"])
                    assoc["dispersion"] = f"{disp_sec * 1000.0:.3f}"
            except (ValueError, IndexError):
                pass

            associations.append(assoc)

        if associations:
            insert(out, "ietf-ntp:ntp", "associations", "association", associations)
    except Exception:
        # NTP not running or no sources configured, silently skip
        pass


def add_ntp_clock_state(out):
    """Add NTP clock state from chronyc tracking"""
    try:
        data = HOST.run_multiline(["chronyc", "-c", "tracking"], [])
        if not data or len(data) == 0:
            return

        # Parse tracking output (CSV format)
        # Format: Ref-ID(IP),Ref-ID(name),Stratum,Ref-time,System-time,Last-offset,
        #         RMS-offset,Frequency,Residual-freq,Skew,Root-delay,Root-dispersion,
        #         Update-interval,Leap-status
        parts = data[0].split(',')
        if len(parts) < 14:
            return

        clock_state = {}
        system_status = {}

        # chronyd uses stratum 0 for "not synchronized", YANG requires 1-16
        stratum_raw = int(parts[2])
        stratum = 16 if stratum_raw == 0 else stratum_raw

        if stratum == 16:
            system_status["clock-state"] = "ietf-ntp:unsynchronized"
        else:
            system_status["clock-state"] = "ietf-ntp:synchronized"

        system_status["clock-stratum"] = stratum

        # Convert hex Ref-ID to IPv4 dotted notation
        # "00000000" -> "0.0.0.0", "7F7F0101" -> "127.127.1.1"
        refid_ip = parts[0]
        refid_name = parts[1]

        if refid_name:
            # NTP refids are always 4 bytes; chronyc strips trailing padding.
            # YANG typedef 'refid' requires exactly length 4 for strings.
            system_status["clock-refid"] = refid_name.ljust(4)[:4]
        elif refid_ip and len(refid_ip) == 8:
            try:
                a = int(refid_ip[0:2], 16)
                b = int(refid_ip[2:4], 16)
                c = int(refid_ip[4:6], 16)
                d = int(refid_ip[6:8], 16)
                system_status["clock-refid"] = f"{a}.{b}.{c}.{d}"
            except ValueError:
                system_status["clock-refid"] = refid_ip if refid_ip else "0.0.0.0"
        else:
            system_status["clock-refid"] = refid_ip if refid_ip else "0.0.0.0"

        # Add clock frequencies (in Hz)
        # chronyd reports frequency offset in ppm, need to convert to Hz
        # Nominal frequency is typically 1000000000 Hz for system clock
        # Format with fraction-digits 4 to avoid scientific notation
        try:
            freq_ppm = float(parts[7])
            nominal = 1000000000.0
            actual = nominal * (1.0 + freq_ppm / 1000000.0)
            system_status["nominal-freq"] = f"{nominal:.4f}"
            system_status["actual-freq"] = f"{actual:.4f}"
        except (ValueError, IndexError):
            pass

        # Clock precision (use skew as approximation, converted to log2 seconds)
        # chronyd reports skew in ppm, we'll use a fixed precision value
        # Most systems have precision around -6 to -20 (2^-6 to 2^-20 seconds)
        system_status["clock-precision"] = -20  # ~1 microsecond precision

        # Clock offset (System-time column, already in seconds)
        # Convert to milliseconds with fraction-digits 3 to match YANG
        try:
            offset_sec = float(parts[4])
            system_status["clock-offset"] = f"{offset_sec * 1000.0:.3f}"
        except (ValueError, IndexError):
            pass

        # Root delay (in seconds, convert to milliseconds)
        # Format with fraction-digits 3 to match YANG
        try:
            root_delay_sec = float(parts[10])
            system_status["root-delay"] = f"{root_delay_sec * 1000.0:.3f}"
        except (ValueError, IndexError):
            pass

        # Root dispersion (in seconds, convert to milliseconds)
        # Format with fraction-digits 3 to match YANG
        try:
            root_disp_sec = float(parts[11])
            system_status["root-dispersion"] = f"{root_disp_sec * 1000.0:.3f}"
        except (ValueError, IndexError):
            pass

        # Reference time (Ref-time in seconds since epoch)
        # YANG expects ntp-date-and-time format, but we'll provide Unix timestamp
        try:
            ref_time = float(parts[3])
            if ref_time > 0:
                # Convert to ISO 8601 timestamp
                from datetime import datetime
                dt = datetime.utcfromtimestamp(ref_time)
                system_status["reference-time"] = dt.strftime("%Y-%m-%dT%H:%M:%S.%fZ")[:-4] + "Z"
        except (ValueError, IndexError, OSError):
            pass

        # Sync state based on leap status
        # chronyd leap status: 0=Normal, 1=Insert second, 2=Delete second, 3=Not synchronized
        try:
            leap_status_str = parts[13].strip()
            if leap_status_str == "Not synchronised" or stratum == 16:
                system_status["sync-state"] = "ietf-ntp:clock-never-set"
            else:
                system_status["sync-state"] = "ietf-ntp:clock-synchronized"
        except (ValueError, IndexError):
            # Default based on stratum
            if stratum == 16:
                system_status["sync-state"] = "ietf-ntp:clock-never-set"
            else:
                system_status["sync-state"] = "ietf-ntp:clock-synchronized"

        # Infix-specific augments: additional chronyd operational data
        # Last offset (parts[5], in seconds, 9 fraction-digits)
        try:
            last_offset = float(parts[5])
            system_status["infix-ntp:last-offset"] = f"{last_offset:.9f}"
        except (ValueError, IndexError):
            pass

        # RMS offset (parts[6], in seconds, 9 fraction-digits)
        try:
            rms_offset = float(parts[6])
            system_status["infix-ntp:rms-offset"] = f"{rms_offset:.9f}"
        except (ValueError, IndexError):
            pass

        # Residual frequency (parts[8], in ppm, 3 fraction-digits)
        try:
            residual_freq = float(parts[8])
            system_status["infix-ntp:residual-freq"] = f"{residual_freq:.3f}"
        except (ValueError, IndexError):
            pass

        # Skew (parts[9], in ppm, 3 fraction-digits)
        try:
            skew = float(parts[9])
            system_status["infix-ntp:skew"] = f"{skew:.3f}"
        except (ValueError, IndexError):
            pass

        # Update interval (parts[12], in seconds, 1 fraction-digit)
        try:
            update_interval = float(parts[12])
            system_status["infix-ntp:update-interval"] = f"{update_interval:.1f}"
        except (ValueError, IndexError):
            pass

        clock_state["system-status"] = system_status
        insert(out, "ietf-ntp:ntp", "clock-state", clock_state)
    except Exception:
        # NTP not running, silently skip
        pass


def add_ntp_server_status(out):
    """Add NTP server operational status (port and stratum)

    Note: This must be called after add_ntp_clock_state() so we can
          reuse the stratum already extracted from chronyc tracking.
    """
    try:
        ntp_data = out.get("ietf-ntp:ntp", {})
        clock_state = ntp_data.get("clock-state", {})
        system_status = clock_state.get("system-status", {})
        stratum = system_status.get("clock-stratum")

        if stratum is not None:
            # Populate refclock-master with operational stratum
            # This shows what stratum we're actually operating at
            refclock = {
                "master-stratum": stratum
            }
            insert(out, "ietf-ntp:ntp", "refclock-master", refclock)

        # Get actual listening port, excluding loopback (command port)
        # UNCONN  0  0  0.0.0.0:123  0.0.0.0:*  users:(("chronyd",pid=5441))
        # UNCONN  0  0        *:123        *:*  users:(("chronyd",pid=5441))
        ss_lines = HOST.run_multiline(["ss", "-ulnp"], [])
        for line in ss_lines:
            if "chronyd" not in line:
                continue
            if "127.0.0.1" in line or "[::1]" in line:
                continue

            parts = line.split()
            if len(parts) >= 5:
                local_addr = parts[3]
                port_str = local_addr.split(':')[-1]
                if port_str.isdigit():
                    insert(out, "ietf-ntp:ntp", "port", int(port_str))
                    break

    except Exception:
        # NTP server not running, silently skip
        pass


def add_ntp_server_stats(out):
    """Add NTP server statistics if ietf-ntp is active"""
    try:
        # Get server statistics from chronyd
        data = HOST.run_multiline(["chronyc", "-c", "serverstats"], [])
        if not data or len(data) == 0:
            return

        # Parse serverstats output (CSV format)
        # Format: NTPpacketsreceived,NTPpacketsdropped,Cmdpacketsreceived,
        #         Cmdpacketsdropped,Clientlogsizeactive,Clientlogmemory,
        #         Ratelimitdrops,NTPpktsresp,NTPpktsresp-fail
        parts = data[0].split(',')
        if len(parts) < 9:
            return

        stats = {}
        stats["packet-received"] = int(parts[0])
        stats["packet-dropped"] = int(parts[1])
        stats["packet-sent"] = int(parts[7])
        stats["packet-sent-fail"] = int(parts[8])

        insert(out, "ietf-ntp:ntp", "ntp-statistics", stats)
    except Exception:
        # NTP server not running or not configured, silently skip
        pass


def operational():
    """Get operational state for ietf-ntp module"""
    out = {}
    add_ntp_associations(out)
    add_ntp_clock_state(out)
    add_ntp_server_status(out)
    add_ntp_server_stats(out)

    return out
