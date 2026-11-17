"""Support bundles from the DUTs"""
import base64
import os
import sys

from . import util

TIMEOUT = 300


def test_name():
    """The test's 9PM name, or its directory"""
    name = os.path.basename(os.path.dirname(os.path.realpath(sys.argv[0])))
    return os.environ.get("NINEPM_TEST_NAME", name)


def collect(dev, path):
    """Save the support bundle of dev in path"""
    out = dev.rpc_output("infix-system", "support-collect")
    if "data" not in out:
        print(f"{dev.name}: support bundle too large, "
              f"left at {out.get('filename')} on the device")
        return

    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as f:
        f.write(base64.b64decode(out["data"]))
    print(f"{dev.name}: support bundle saved in {path}")


def collect_all(devs, topdir):
    """Save the support bundles of devs in topdir, in parallel"""
    def one(dev):
        try:
            collect(dev, os.path.join(topdir, test_name(), f"{dev.name}.tar.gz"))
        except Exception as e:
            print(f"{dev.name}: no support bundle: {e}")

    try:
        util.parallel(*[lambda d=d: one(d) for d in devs], timeout=TIMEOUT)
    except TimeoutError as e:
        print(f"Support bundles: {e}")
