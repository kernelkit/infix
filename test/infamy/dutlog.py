"""Collect each DUT's syslog in the test container during a test

Off by default, enable with --capture-syslog or TEST_SYSLOG_CAPTURE=y.
Each DUT logs to a syslogd in the test container, which sorts messages
by sender into $NINEPM_LOG_PATH/syslog/<test>/<node>/.  The start and
stop of the test are marked in each DUT's log with the log RPC, using
msgid test-start and test-stop.  Best-effort, a failed capture never
fails the test.
"""
import datetime
import json
import os
import signal
import subprocess
import sys
import time
import uuid

SYSLOGD = "/usr/local/sbin/syslogd"
CONF = "/tmp/infamy-syslog.conf"
PIDFILE = "/tmp/infamy-syslogd.pid"
SOCKET = "/tmp/infamy-syslog.sock"
SDID = "test@61046"


def _syslogd():
    """Return pid of the syslogd collecting DUT logs, start it if needed"""
    try:
        with open(PIDFILE) as f:
            pid = int(f.read())
        os.kill(pid, 0)
        return pid
    except (OSError, ValueError):
        pass

    open(CONF, "w").close()
    # -n: no DNS, so the source property is the sender's address
    # -k: keep facility kern from the DUTs, -K: no local kernel log
    subprocess.run([SYSLOGD, "-f", CONF, "-P", PIDFILE, "-p", SOCKET,
                    "-n", "-k", "-K", "-m", "0"], check=True)
    _wait(lambda: os.path.exists(PIDFILE))
    with open(PIDFILE) as f:
        return int(f.read())


def _ll_addr(ifname):
    """Return our IPv6 link-local address on ifname"""
    out = subprocess.run(["ip", "-6", "-j", "addr", "show", "dev", ifname, "scope", "link"],
                         stdout=subprocess.PIPE, check=True).stdout
    return json.loads(out)[0]["addr_info"][0]["local"]


def _wait(fn, timeout=5):
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        if fn():
            return True
        time.sleep(0.2)
    return False


def _logged(path, run, msgid):
    try:
        with open(path) as f:
            return any(run in line and msgid in line for line in f)
    except OSError:
        return False


class Capture:
    def __init__(self):
        name = os.path.basename(os.path.dirname(os.path.realpath(sys.argv[0])))
        self.test = os.environ.get("NINEPM_TEST_NAME", name)
        self.run = uuid.uuid4().hex[:12]
        self.dir = None
        self.duts = {}

    def _mark(self, node, dev, msgid):
        now = datetime.datetime.now().astimezone().isoformat(timespec="milliseconds")
        dev.log(f"{msgid} {self.test}", app_name="infamy", msgid=msgid,
                sd={SDID: {"name": self.test, "node": node,
                           "run": self.run, "host-time": now}})

    def _path(self, node):
        return os.path.join(self.dir, node, "syslog")

    def _reload(self, pid):
        """Write rules for this test's DUTs, sorted on sender address"""
        with open(CONF, "w") as f:
            for node, (_, source) in self.duts.items():
                path = os.path.dirname(self._path(node))
                os.makedirs(path, exist_ok=True)
                # A filter only covers the rule following it
                for sel, name in (("*.*", "syslog"), ("kern.*", "kern.log")):
                    f.write(f':source, isequal, "{source}"\n'
                            f"{sel}\t-{path}/{name}\t;RFC5424\n")
        os.kill(pid, signal.SIGHUP)

    def begin(self, node, dev, mgmtip, cport, dport):
        """Make node log to us, and mark the start of the test in its log

        Called on every attach, test_reset drops the remote action.
        """
        if not hasattr(dev, "log"):
            return

        logdir = os.environ.get("NINEPM_LOG_PATH")
        if not logdir:
            print("dutlog: NINEPM_LOG_PATH not set, not capturing syslog")
            return
        self.dir = os.path.join(logdir, "syslog", self.test)

        try:
            pid = _syslogd()
            dev.patch_config("ietf-syslog", {
                "syslog": {
                    "actions": {
                        "remote": {
                            "destination": [{
                                "name": "infamy",
                                "udp": {
                                    "address": f"{_ll_addr(cport)}%{dport}"
                                },
                                "facility-filter": {
                                    "facility-list": [{
                                        "facility": "all",
                                        "severity": "all"
                                    }]
                                },
                                "infix-syslog:log-format": "rfc5424"
                            }]
                        }
                    }
                }
            })
            known = node in self.duts
            # The source property is the sender address, with scope
            self.duts[node] = (dev, mgmtip)
            if known:
                return
            self._reload(pid)

            # The DUT applies the remote action asynchronously, retry
            # the marker until it shows up
            for _ in range(5):
                self._mark(node, dev, "test-start")
                if _wait(lambda: _logged(self._path(node), self.run, "test-start"), 1):
                    break
            else:
                print(f"dutlog: {node}: no syslog received in {self._path(node)}")
        except Exception as e:
            print(f"dutlog: {node}: failed setting up syslog capture: {e}")

    def end(self):
        """Mark the stop of the test, and stop sorting logs to this test"""
        if not self.duts:
            return

        for node, (dev, _) in self.duts.items():
            try:
                self._mark(node, dev, "test-stop")
                _wait(lambda: _logged(self._path(node), self.run, "test-stop"), 2)
            except Exception as e:
                print(f"dutlog: {node}: failed logging stop marker: {e}")

        try:
            self.duts = {}
            self._reload(_syslogd())
        except Exception as e:
            print(f"dutlog: failed resetting syslog capture: {e}")

        print(f"dutlog: syslog saved in {self.dir}")
