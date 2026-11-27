"""Test and step markers in the DUTs' logs"""
from . import support, util

SDID = "test@32473"


class Markers:
    """Best effort, a DUT that fails a marker gets no more markers"""

    def __init__(self):
        self.test = support.test_name()
        self.devs = {}

    def _log(self, devs, msgid, text, **params):
        done = set()

        def one(dev):
            try:
                # A call to a DUT that is down hangs until the DUT is back
                if not dev.reachable():
                    raise ConnectionError("not reachable")
                dev.log(text, app_name="infamy", msgid=msgid,
                        sd={SDID: {"name": self.test, "role": dev.name, **params}})
                done.add(dev.name)
            except Exception as e:
                print(f"{dev.name}: no {msgid} marker: {e}")

        util.parallel(*[lambda d=d: one(d) for d in devs])

        for dev in devs:
            if dev.name not in done:
                self.devs.pop(dev.name, None)
        return done

    def attach(self, dev):
        if dev.name in self.devs or self._log([dev], "test-start", f"Test {self.test} starts"):
            self.devs[dev.name] = dev

    def step(self, num, msg):
        self._log(list(self.devs.values()), "step", f"Step {num}: {msg}", step=str(num))

    def stop(self):
        self._log(list(self.devs.values()), "test-stop", f"Test {self.test} stops")
