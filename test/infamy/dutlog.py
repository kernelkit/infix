"""Capture each attached DUT's syslog for the duration of a test

A begin marker is logged on every DUT when the test attaches to it,
and an end marker when the test exits.  The lines between the two are
then fetched over SSH and saved next to the 9pm test output, in
$NINEPM_LOG_PATH/syslog/<test>-<node>.log

Both markers carry the host's wall-clock time, so the offset between
the DUT and host clocks can be read from the captured file and DUT
log lines lined up with the 9pm output log.  Marker matching is done
on a per-run id, never on timestamps, so a DUT with a skewed clock or
another timezone does not matter.

Everything here is best-effort, a DUT that has rebooted or lost its
management connectivity must never fail the test.
"""
import datetime
import os
import subprocess
import sys
import uuid

from . import ssh

LOGS = "/var/log/syslog.0 /var/log/syslog"
TAG = "infamy"


def _now():
    return datetime.datetime.now().astimezone().isoformat(timespec="milliseconds")


class Capture:
    def __init__(self):
        name = os.path.basename(os.path.dirname(os.path.realpath(sys.argv[0])))
        self.test = os.environ.get("NINEPM_TEST_NAME", name)
        self.run = uuid.uuid4().hex[:12]
        self.duts = {}

    def _marker(self, what):
        return f"test-{what} {self.test} run={self.run} host-time={_now()}"

    def begin(self, node, dev, location):
        """Log the begin marker on node, once, and remember how to reach it"""
        if node in self.duts:
            return

        self.duts[node] = ssh.Location(location.host, location.username,
                                       location.password)
        try:
            if hasattr(dev, "log"):
                dev.log(self._marker("begin"), app_name=TAG, msgid="test-begin")
            else:
                dev.runsh(f"logger -t {TAG} -p user.notice '{self._marker('begin')}'",
                          timeout=10)
        except Exception as e:
            print(f"dutlog: failed logging begin marker on {node}: {e}")

    def end(self):
        """Log the end marker on all DUTs and save what was logged in-between"""
        for node, location in self.duts.items():
            try:
                self._fetch(node, location)
            except Exception as e:
                print(f"dutlog: failed capturing syslog from {node}: {e}")

    def _fetch(self, node, location):
        # One SSH round-trip: log the end marker over the same transport
        # for every DUT, wait for syslogd to write it, then extract this
        # run.  The rotated file is included in case the log rotated
        # during the test.
        run = f"run={self.run}"
        script = f"""
logger -t {TAG} -p user.notice '{self._marker("end")}'
for i in $(seq 20); do
    sudo grep -q 'test-end.*{run}' /var/log/syslog && break
    sleep 0.1
done
sudo cat {LOGS} 2>/dev/null | awk '/test-begin.*{run}/ {{p=1}} p; /test-end.*{run}/ {{exit}}'
"""
        dev = ssh.Device(node, location, wait=False)
        rc = dev.run("/bin/sh", text=True, input=script, stdout=subprocess.PIPE,
                     stderr=subprocess.DEVNULL, loglevel="QUIET", timeout=30)
        if rc.returncode != 0 or not rc.stdout:
            print(f"dutlog: no syslog captured from {node} (rc {rc.returncode})")
            return

        logdir = os.environ.get("NINEPM_LOG_PATH")
        if not logdir:
            print(f"dutlog: {node}: {len(rc.stdout.splitlines())} lines, "
                  "set NINEPM_LOG_PATH to save them")
            return

        path = os.path.join(logdir, "syslog", f"{self.test}-{node}.log")
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w") as f:
            f.write(rc.stdout)
        print(f"dutlog: {node}: saved {len(rc.stdout.splitlines())} lines to {path}")
