"""Helper functions for tests"""
import base64
import time
import threading
import infamy.neigh
from infamy import netconf
from infamy import restconf


class ParallelFn(threading.Thread):
    def __init__(self, group=None, target=None, name=None,
                 args=(), kwargs={}, Verbose=None):
        threading.Thread.__init__(self, group, target, name, args, kwargs)
        self._exc, self._return = None, None

    def run(self):
        if self._target is not None:
            try:
                self._return = self._target(*self._args,
                                            **self._kwargs)
            except Exception as e:
                self._exc = e

    def join(self, *args):
        threading.Thread.join(self, *args)

        if self._exc:
            raise self._exc

        return self._return


def parallel(*fns):
    ths = [ParallelFn(target=fn) for fn in fns]
    [th.start() for th in ths]
    return [th.join() for th in ths]


def until(fn, attempts=10, interval=1):
    for attempt in range(attempts):
        if fn():
            return

        time.sleep(interval)

    raise Exception("Expected condition did not materialize")


def is_reachable(neigh, env, pwd):
    if env.args.transport is not None:
        if env.args.transport == "netconf":
            return netconf.netconf_syn(neigh)
        elif env.args.transport == "restconf":
            return restconf.restconf_reachable(neigh, pwd)
        else:
            raise Exception(f"Unsupported transport {env.args.transport}!")
    else:
        # No transport specified, so check both netconf and restconf
        netconf_reachable = netconf.netconf_syn(neigh)
        restconf_reachable = restconf.restconf_reachable(neigh, pwd)
        return netconf_reachable and restconf_reachable


def to_binary(text):
    """Base64 encode the text, removing newlines"""
    enc = base64.b64encode(text.encode('utf-8'))

    # Convert the encoded bytes to a string and remove any newlines
    return enc.decode('utf-8').replace('\n', '')


def wait_boot(target, env):
    print(f"{target} is shutting down ...")
    until(lambda: not target.reachable(), attempts=100)

    print(f"{target} is booting up ...")
    until(lambda: target.reachable(), attempts=300)

    iface = target.get_mgmt_iface()
    if not iface:
        return False

    neigh = infamy.neigh.ll6ping(iface)
    if not neigh:
        return False

    print(f"{target} is responding to IPv6 ping ...")

    pwd = target.location.password
    until(lambda: is_reachable(neigh, env, pwd), attempts=300)

    return True


def warn(msg):
    """Print a warning message in yellow to stderr."""
    YELLOW = "\033[93m"
    RST = "\033[0m"
    print(f"{YELLOW}warn - {msg}{RST}")
