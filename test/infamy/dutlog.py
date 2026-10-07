"""Collect each DUT's syslog per test, see doc/testing.md

Best-effort, a failed capture never fails the test.
"""
import datetime
import json
import os
import signal
import subprocess
import sys
import threading
import time
import uuid

from . import util

SYSLOGD = "/usr/local/sbin/syslogd"
CONF = "/tmp/infamy-syslog.conf"
PIDFILE = "/tmp/infamy-syslogd.pid"
SOCKET = "/tmp/infamy-syslog.sock"
SDID = "test@61046"

# Same files as on the DUT, messages with the selector from its syslog.conf
RULES = (
    ("*.*", "syslog"),
    ("kern.*", "kern.log"),
    ("*.=info;*.=notice;*.=warn;auth,authpriv.none;cron,daemon.none;mail,news.none",
     "messages"),
)


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
    util.until(lambda: os.path.exists(PIDFILE), attempts=50, interval=0.1)
    with open(PIDFILE) as f:
        return int(f.read())


def _ll_addr(ifname):
    """Return our IPv6 link-local address on ifname"""
    out = subprocess.run(["ip", "-6", "-j", "addr", "show", "dev", ifname, "scope", "link"],
                         stdout=subprocess.PIPE, check=True).stdout
    return json.loads(out)[0]["addr_info"][0]["local"]


def _logged(path, run, msgid):
    # Not every logged line is valid UTF-8
    try:
        with open(path, errors="replace") as f:
            return any(run in line and msgid in line for line in f)
    except OSError:
        return False


class Capture:
    def __init__(self):
        name = os.path.basename(os.path.dirname(os.path.realpath(sys.argv[0])))
        self.test = os.environ.get("NINEPM_TEST_NAME", name)
        self.run = uuid.uuid4().hex[:12]
        self.dir = os.path.join(os.environ.get("NINEPM_LOG_PATH", ""), "syslog", self.test)
        self.duts = {}  # node -> (dev, sender address), dev None when muted

    def _mark(self, node, dev, msgid, text=None, **params):
        now = datetime.datetime.now().astimezone().isoformat(timespec="milliseconds")
        dev.log(text or f"{msgid} {self.test}", app_name="infamy", msgid=msgid,
                sd={SDID: {"name": self.test, "node": node,
                           "run": self.run, "host-time": now, **params}})

    def _marks(self, msgid, text=None, **params):
        """Mark all DUTs in parallel, return the nodes that answered

        RPC timeouts are 90-120 s, so a DUT that does not answer within a
        second, e.g. while rebooting, is muted until it is attached again.
        """
        done = set()

        def mark(node, dev):
            try:
                self._mark(node, dev, msgid, text, **params)
                done.add(node)
            except Exception as e:
                print(f"dutlog: {node}: failed logging {msgid} marker: {e}")

        threads = [threading.Thread(target=mark, args=(node, dev), daemon=True)
                   for node, (dev, _) in self.duts.items() if dev]
        for t in threads:
            t.start()
        deadline = time.monotonic() + 1
        for t in threads:
            t.join(max(0, deadline - time.monotonic()))

        for node, (dev, source) in self.duts.items():
            if dev and node not in done:
                print(f"dutlog: {node}: no more markers until next attach")
                self.duts[node] = (None, source)
        return done

    def _logs(self, node):
        return os.path.join(self.dir, node)

    def _reload(self, pid):
        """Write rules for this test's DUTs, sorted on sender address"""
        with open(CONF, "w") as f:
            for node, (_, source) in self.duts.items():
                os.makedirs(self._logs(node), exist_ok=True)
                # A filter only covers the rule following it
                for sel, name in RULES:
                    f.write(f':source, isequal, "{source}"\n'
                            f"{sel}\t-{self._logs(node)}/{name}\t;RFC5424\n")
        os.kill(pid, signal.SIGHUP)

    def begin(self, node, dev, mgmtip, cport, dport):
        """Make node log to us, and mark the start of the test in its log

        Called on every attach, test_reset drops the remote action.
        """
        if "NINEPM_LOG_PATH" not in os.environ:
            print("dutlog: NINEPM_LOG_PATH not set, not capturing syslog")
            return

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
            new = node not in self.duts
            # The source property is the sender address, with scope
            self.duts[node] = (dev, mgmtip)
            if not new:
                return
            self._reload(pid)

            # The DUT applies the remote action asynchronously, resend
            # the marker until it shows up
            path = os.path.join(self._logs(node), "syslog")
            util.until(lambda: _logged(path, self.run, "test-start") or
                       self._mark(node, dev, "test-start"), attempts=15, interval=0.3)
        except Exception as e:
            print(f"dutlog: {node}: failed setting up syslog capture: {e}")

    def step(self, num, msg):
        """Mark the start of a test step"""
        self._marks("step", f"step {num}: {msg}", step=str(num))

    def end(self):
        """Mark the stop of the test, and stop sorting logs to this test"""
        if not self.duts:
            return

        try:
            paths = [os.path.join(self._logs(node), "syslog")
                     for node in self._marks("test-stop")]
            util.until(lambda: all(_logged(p, self.run, "test-stop") for p in paths),
                       attempts=10, interval=0.2)
        except Exception:
            pass  # Tests that set up syslog drop our remote action

        try:
            self.duts = {}
            self._reload(_syslogd())
        except Exception as e:
            print(f"dutlog: failed resetting syslog capture: {e}")

        print(f"dutlog: syslog saved in {self.dir}")
