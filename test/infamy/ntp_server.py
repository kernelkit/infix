"""Start NTP server in the background"""
import contextlib
import os
import tempfile
import time


class Server:
    """chronyd serving its local clock, never touching it (-x).

    stratum and distance control what is advertised to clients: the
    stratum of the served time and the root distance (accuracy claim).
    An honest, wide distance keeps clients from flagging the source as
    unstable ('~') while their own clocks are still settling, which
    BusyBox ntpd provoked by claiming near-zero dispersion.
    """

    def __init__(self, netns, stratum=5, distance=1.0):
        self.process = None
        self.netns = netns
        self.stratum = stratum
        self.distance = distance
        self.rundir = None
        self.logfile = None
        self.pidfile = None

    def __enter__(self):
        self.start()
        return self

    def __exit__(self, _, __, ___):
        self.stop()

    def start(self):
        # Instances in different netns share the filesystem, so the
        # pidfile is per-instance.  It lives in /run/chrony because a
        # host AppArmor profile for chronyd, present when the host runs
        # chrony, attaches by binary path even inside the test container
        # and allows no other pidfile location.  The command socket and
        # port are disabled, nothing talks to chronyc here, and
        # -f /dev/null keeps the image's default config out of it.
        self.rundir = tempfile.TemporaryDirectory(prefix="chronyd-")
        log = f"{self.rundir.name}/chronyd.log"
        piddir = "/run/chrony"
        try:
            os.makedirs(piddir, exist_ok=True)
        except OSError:
            piddir = self.rundir.name
        self.pidfile = f"{piddir}/{os.path.basename(self.rundir.name)}.pid"
        cmd = [
            "chronyd", "-d", "-x", "-f", "/dev/null", "-u", "root",
            f"local stratum {self.stratum} distance {self.distance}",
            "allow",
            "cmdport 0",
            "bindcmdaddress /",
            f"pidfile {self.pidfile}",
        ]
        self.logfile = open(log, "w")
        self.process = self.netns.popen(cmd, stderr=self.logfile)

        # chronyd exits immediately on bad options or a missing binary
        # behind an exec wrapper; fail loudly instead of serving nothing
        time.sleep(1)
        if self.process.poll() is not None:
            with open(log) as f:
                output = f.read().strip()
            code = self.process.returncode
            self.stop()
            raise RuntimeError(
                f"chronyd failed to start (exit {code}): "
                f"{output or 'no output; is chrony installed in the test environment?'}")

    def stop(self):
        if self.process:
            self.process.terminate()
            self.process.wait()
            self.process = None
        if self.logfile:
            self.logfile.close()
            self.logfile = None
        if self.pidfile:
            with contextlib.suppress(OSError):
                os.unlink(self.pidfile)
            self.pidfile = None
        if self.rundir:
            self.rundir.cleanup()
            self.rundir = None
