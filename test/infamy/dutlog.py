"""Save each DUT's syslog for the duration of a test

Off by default, enable with --capture-syslog or TEST_SYSLOG_CAPTURE=y.
A marker with a per-run id is logged on each DUT when the test begins
and ends, and the lines between them are saved to
$NINEPM_LOG_PATH/syslog/<test>-<node>.log.  Best-effort, a failed
capture never fails the test.
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
