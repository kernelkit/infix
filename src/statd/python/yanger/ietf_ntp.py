import subprocess

from .common import insert
from .host import HOST


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


def add_ntp_associations(out):
    """Add NTP association information from chronyc sources"""
    try:
        data = HOST.run_multiline(["chronyc", "-c", "sources"], [])
        if not data:
            return

        associations = []
        # Map chronyd mode indicators to ietf-ntp association-mode identities
        mode_map = {
            "^": "ietf-ntp:client",            # We're client to this server
            "=": "ietf-ntp:active",            # Peer mode (symmetric active)
            "#": "ietf-ntp:broadcast-client"   # Local refclock (closest match)
        }

        for line in data:
            parts = line.split(',')
            if len(parts) < 5:
                continue

            mode_indicator = parts[0]
            stratum = int(parts[3])

            # Skip sources with invalid stratum (0 means unreachable/not yet synced)
            # YANG model requires stratum to be in range 1..16
            if stratum < 1 or stratum > 16:
                continue

            assoc = {}
            assoc["address"] = parts[2]
            assoc["local-mode"] = mode_map.get(mode_indicator, "ietf-ntp:client")
            assoc["isconfigured"] = True  # Sources from config
            assoc["stratum"] = stratum
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
        # Format: Ref-ID,Stratum,Ref-time,System-time,Last-offset,RMS-offset,
        #         Frequency,Residual-freq,Skew,Root-delay,Root-dispersion,
        #         Update-interval,Leap-status
        parts = data[0].split(',')
        if len(parts) < 13:
            return

        clock_state = {}
        system_status = {}

        # Determine clock-state based on stratum
        stratum = int(parts[1])
        if stratum == 16:
            system_status["clock-state"] = "ietf-ntp:unsynchronized"
        else:
            system_status["clock-state"] = "ietf-ntp:synchronized"

        system_status["clock-stratum"] = stratum
        system_status["clock-refid"] = parts[0]

        # Add clock frequencies (in Hz)
        # chronyd reports frequency offset in ppm, need to convert to Hz
        # Nominal frequency is typically 1000000000 Hz for system clock
        # We'll use a simplified approach: report the frequency offset
        try:
            freq_ppm = float(parts[6])  # Frequency offset in ppm
            # For simplicity, use nominal as base and actual with offset applied
            nominal = 1000000000.0  # 1 GHz nominal
            system_status["nominal-freq"] = nominal
            system_status["actual-freq"] = nominal * (1.0 + freq_ppm / 1000000.0)
        except (ValueError, IndexError):
            pass

        # Clock precision (use skew as approximation, converted to log2 seconds)
        # chronyd reports skew in ppm, we'll use a fixed precision value
        # Most systems have precision around -6 to -20 (2^-6 to 2^-20 seconds)
        system_status["clock-precision"] = -20  # ~1 microsecond precision

        # Clock offset (System-time column, already in seconds)
        try:
            offset_sec = float(parts[3])
            system_status["clock-offset"] = offset_sec * 1000.0  # Convert to milliseconds
        except (ValueError, IndexError):
            pass

        # Root delay (in seconds, convert to milliseconds)
        try:
            root_delay_sec = float(parts[9])
            system_status["root-delay"] = root_delay_sec * 1000.0
        except (ValueError, IndexError):
            pass

        # Root dispersion (in seconds, convert to milliseconds)
        try:
            root_disp_sec = float(parts[10])
            system_status["root-dispersion"] = root_disp_sec * 1000.0
        except (ValueError, IndexError):
            pass

        # Reference time (Ref-time in seconds since epoch)
        # YANG expects ntp-date-and-time format, but we'll provide Unix timestamp
        try:
            ref_time = float(parts[2])
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
            leap_status = int(parts[12])
            if leap_status == 3 or stratum == 16:
                system_status["sync-state"] = "ietf-ntp:freq-not-determined"
            else:
                system_status["sync-state"] = "ietf-ntp:synchronized"
        except (ValueError, IndexError):
            # Default based on stratum
            if stratum == 16:
                system_status["sync-state"] = "ietf-ntp:freq-not-determined"
            else:
                system_status["sync-state"] = "ietf-ntp:synchronized"

        clock_state["system-status"] = system_status
        insert(out, "ietf-ntp:ntp", "clock-state", clock_state)
    except Exception:
        # NTP not running, silently skip
        pass


def add_ntp_makestep(out):
    """Add makestep configuration as operational state"""
    try:
        # Read chronyd config to check if makestep is configured
        with open("/etc/chrony/conf.d/ntp-server.conf", "r") as f:
            config = f.read()

        # Parse makestep directive if present
        for line in config.split('\n'):
            line = line.strip()
            if line.startswith("makestep "):
                parts = line.split()
                if len(parts) >= 3:
                    try:
                        # Keep as strings for YANG decimal64 and int32 encoding
                        makestep_data = {
                            "threshold": parts[1],  # decimal64 must be string-encoded
                            "limit": int(parts[2])
                        }
                        insert(out, "ietf-ntp:ntp", "infix-ntp:makestep", makestep_data)
                        break
                    except (ValueError, IndexError):
                        pass
    except Exception:
        # Config file doesn't exist or can't be read, silently skip
        pass


def operational():
    """Get operational state for ietf-ntp module"""
    out = {}
    add_ntp_server_stats(out)
    add_ntp_associations(out)
    add_ntp_clock_state(out)
    add_ntp_makestep(out)
    return out
