"""Mark test and step boundaries in the DUTs' logs, see doc/test-arch.md"""
import os
import sys
import threading
import time

SDID = "test@32473"


def test_name():
    """Name of the running test, as 9PM knows it or its directory"""
    name = os.path.basename(os.path.dirname(os.path.realpath(sys.argv[0])))
    return os.environ.get("NINEPM_TEST_NAME", name)


class Markers:
    """Best-effort, a DUT that does not answer is skipped until attached again"""

    def __init__(self):
        self.test = test_name()
        self.devs = {}

    def _log_one(self, dev, msgid, text, **params):
        try:
            dev.log(text, app_name="infamy", msgid=msgid,
                    sd={SDID: {"name": self.test, "role": dev.name, **params}})
            return True
        except Exception as e:
            print(f"{dev.name}: failed logging {msgid} marker: {e}")
            return False

    def _log(self, devs, msgid, text, **params):
        def log(dev):
            if self._log_one(dev, msgid, text, **params):
                done.add(dev.name)

        done = set()
        threads = [threading.Thread(target=log, args=(dev,), daemon=True) for dev in devs]
        for t in threads:
            t.start()
        deadline = time.monotonic() + 1
        for t in threads:
            t.join(max(0, deadline - time.monotonic()))

        for dev in devs:
            if dev.name not in done:
                self.devs.pop(dev.name, None)

    def attach(self, dev):
        if dev.name in self.devs or self._log_one(dev, "test-start", f"Test {self.test} starts"):
            self.devs[dev.name] = dev

    def step(self, num, msg):
        self._log(list(self.devs.values()), "step", f"Step {num}: {msg}", step=str(num))

    def stop(self):
        self._log(list(self.devs.values()), "test-stop", f"Test {self.test} stops")
