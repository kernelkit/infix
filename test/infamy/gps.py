"""GPS/NMEA test helpers

Provides an NMEAGenerator class that writes NMEA sentences to a QEMU
pipe chardev FIFO, simulating a GPS receiver.  Also provides helpers
for querying GPS operational state via YANG.
"""

import os
import errno
import threading
import time
import pynmea2
class NMEAGenerator:
    """Write NMEA sentences to a QEMU pipe chardev FIFO.

    Sends a full cycle of NMEA sentences (like a real u-blox receiver)
    continuously at 1 Hz in a background thread.

    The pipe_path should be the base path (without .in/.out suffix),
    matching the QEMU ``-chardev pipe,path=...`` argument.

    Usage::

        with NMEAGenerator("/tmp/node-gps") as nmea:
            # NMEA data is being sent in background
            time.sleep(10)
    """

    def __init__(self, pipe_path, lat=48.1173, lon=11.5167, alt=545.4):
        self.pipe_path = pipe_path
        self._fd = -1
        self.lat = lat
        self.lon = lon
        self.alt = alt
        self._thread = None
        self._stop = threading.Event()

    def __enter__(self):
        self.start()
        return self
    def __exit__(self, _, __, ___):
        self.close()

    def start(self):
        self._stop.clear()
        self._thread = threading.Thread(target=self._send_loop, daemon=True)
        self._thread.start()
        print(f"NMEA: started sender thread for {self.pipe_path}")

    def close(self):
        self._stop.set()
        if self._thread:
            self._thread.join(timeout=5)
            self._thread = None
        if self._fd >= 0:
            try:
                os.close(self._fd)
            except OSError:
                pass
            self._fd = -1

    def _send_loop(self):
        print(f"NMEA: send_loop entered for {self.pipe_path}")
        while not self._stop.is_set():
            try:
                self._fd = os.open(f"{self.pipe_path}.in",
                                   os.O_WRONLY | os.O_NONBLOCK)
                print(f"NMEA: opened {self.pipe_path}.in (fd={self._fd})")
            except OSError as e:
                if e.errno in (errno.ENXIO, errno.ENOENT):
                    print(f"NMEA: {self.pipe_path}.in not ready ({e}), retrying ...")
                    self._stop.wait(0.5)
                    continue
                print(f"NMEA: {self.pipe_path}.in open failed: {e}")
                raise
            cycles = 0
            while not self._stop.is_set():
                try:
                    self._send_cycle()
                    cycles += 1
                    if cycles <= 3 or cycles % 10 == 0:
                        print(f"NMEA: {self.pipe_path} sent cycle {cycles}")
                except BlockingIOError:
                    print(f"NMEA: {self.pipe_path} write blocked (cycle {cycles})")
                except OSError as e:
                    print(f"NMEA: {self.pipe_path} write error: {e}, reconnecting")
                    break
                self._stop.wait(1)
            try:
                os.close(self._fd)
            except OSError:
                pass
            self._fd = -1

    def _send(self, sentence):
        data = (str(sentence) + "\r\n").encode()
        try:
            os.write(self._fd, data)
        except BlockingIOError:
            pass

    def _send_cycle(self):
        """Send a full NMEA cycle matching real u-blox GPS output."""
        now = time.gmtime()
        utc = time.strftime("%H%M%S.00", now)
        date = time.strftime("%d%m%y", now)

        lat_deg = int(abs(self.lat))
        lat_min = (abs(self.lat) - lat_deg) * 60
        lat_str = f"{lat_deg:02d}{lat_min:07.4f}"
        lat_ns = "N" if self.lat >= 0 else "S"

        lon_deg = int(abs(self.lon))
        lon_min = (abs(self.lon) - lon_deg) * 60
        lon_str = f"{lon_deg:03d}{lon_min:07.4f}"
        lon_ew = "E" if self.lon >= 0 else "W"

        # RMC - Recommended Minimum
        self._send(pynmea2.RMC("GP", "RMC", (
            utc, "A",
            lat_str, lat_ns,
            lon_str, lon_ew,
            "0.0", "0.0",
            date,
            "0.0", "E",
            "A",
        )))

        # VTG - Track Made Good and Ground Speed
        self._send(pynmea2.VTG("GP", "VTG", (
            "0.0", "T",
            "", "M",
            "0.0", "N",
            "0.0", "K",
            "A",
        )))

        # GGA - Fix Data
        self._send(pynmea2.GGA("GP", "GGA", (
            utc,
            lat_str, lat_ns,
            lon_str, lon_ew,
            "1", "08", "0.9",
            f"{self.alt:.1f}", "M",
            "47.0", "M",
            "", "",
        )))

        # GSA - DOP and Active Satellites (3D fix, 8 sats)
        self._send(pynmea2.GSA("GP", "GSA", (
            "A", "3",
            "01", "02", "03", "04", "05", "06", "07", "08",
            "", "", "", "",
            "1.5", "0.9", "1.2",
        )))

        # GSV - Satellites in View (4 sats per message, 2 messages)
        self._send(pynmea2.GSV("GP", "GSV", (
            "2", "1", "08",
            "01", "45", "045", "40",
            "02", "30", "090", "38",
            "03", "60", "135", "42",
            "04", "15", "180", "35",
        )))
        self._send(pynmea2.GSV("GP", "GSV", (
            "2", "2", "08",
            "05", "50", "225", "41",
            "06", "25", "270", "36",
            "07", "70", "315", "44",
            "08", "20", "000", "33",
        )))

        # GLL - Geographic Position
        self._send(pynmea2.GLL("GP", "GLL", (
            lat_str, lat_ns,
            lon_str, lon_ew,
            utc,
            "A",
            "A",
        )))

def _get_hardware(target):
    data = target.get_data("/ietf-hardware:hardware")
    if not data or "hardware" not in data:
        return {}
    return data["hardware"]


def get_gps_state(target, name="gps0"):
    """Get GPS receiver operational state for a named component."""
    hardware = _get_hardware(target)
    for component in hardware.get("component", []):
        if component.get("name") == name:
            return component.get("infix-hardware:gps-receiver",
                                 component.get("gps-receiver"))
    return None


def is_activated(target, name="gps0"):
    """Check if gpsd has activated the GPS device."""
    state = get_gps_state(target, name)
    return state.get("activated", False) if state else False


def has_fix(target, name="gps0"):
    """Check if GPS reports a fix (2D or 3D)."""
    state = get_gps_state(target, name)
    if not state:
        return False
    return state.get("fix-mode") in ("2d", "3d")


def has_position(target, name="gps0"):
    """Check if GPS has a fix and all position fields are populated."""
    state = get_gps_state(target, name)
    if not state:
        return False
    if state.get("fix-mode") not in ("2d", "3d"):
        return False
    return all(k in state for k in ("latitude", "longitude", "altitude", "satellites-used"))
