"""Helper functions for tests"""
import base64
import time
import threading
import infamy.neigh
from infamy import netconf
from infamy import restconf
import urllib.request


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


def until(fn, attempts=60, interval=1):
    last_exc = None
    for attempt in range(attempts):
        try:
            result = fn()
        except Exception as e:
            last_exc = e
            result = False
        if result:
            return result

        time.sleep(interval)

    if last_exc:
        raise last_exc
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


def wait_boot(target, env, probe=False):
    """Wait for target to go down and come back up after a reboot.

    The topology may set shutdown_time on the node, the time it takes
    for the node to go down, to wait before checking if it is gone.

    With probe, wait for the first node answering on the management
    port instead of the address the test attached to, for a device
    that boots another configuration and returns on another address.
    """
    print(f"{target} is shutting down ...")
    node = env.ltop.xlate(target.name) if env.ltop else target.name
    shutdown_time = env.ptop.get_shutdown_time(node)
    if shutdown_time:
        time.sleep(shutdown_time)

    until(lambda: not target.reachable(), attempts=100)

    print(f"{target} is booting up ...")
    until(lambda: target.reachable(), attempts=300)

    host = target.location.host
    if probe:
        host = infamy.neigh.ll6ping(target.location.interface)
        if not host:
            return False

    print(f"{target} is responding to IPv6 ping, waiting for {host} ...")

    pwd = target.location.password
    until(lambda: target.reachable() and is_reachable(host, env, pwd),
          attempts=300)

    return True


def reboot_to_startup(target, env, port="mgmt"):
    """Save running to startup-config, reboot onto it, and reattach.

    startup_override() makes the next boot load startup-config instead
    of the test-config.  Returns a new device attached with
    test_reset=False, so the saved config is kept.
    """
    target.startup_override()
    target.copy("running", "startup")
    target.reboot()
    if not wait_boot(target, env):
        raise Exception(f"{target} did not come back after reboot")
    return env.attach(target.name, port, test_reset=False)


def warn(msg):
    """Print a warning message in yellow to stderr."""
    YELLOW = "\033[93m"
    RST = "\033[0m"
    print(f"{YELLOW}warn - {msg}{RST}")

def curl(url, timeout=10, silent=False):
    """Fetch a URL and return its response body as a UTF-8 string.

    Args:
        url (str): The full URL to fetch.
        timeout (int): Request timeout in seconds.
        silent (bool): If True, suppress warning on failure (for retry scenarios).

    Returns:
        str | None: Response body as text, or None if the request failed.
    """
    url = urllib.parse.quote(url, safe='/:')

    try:
        with urllib.request.urlopen(url, timeout=timeout) as response:
            return response.read().decode('utf-8', errors='replace')
    except (urllib.error.URLError, ConnectionResetError, UnicodeEncodeError, TimeoutError) as e:
        if not silent:
            print(f"[WARN] curl: failed to fetch {url}: {e}")
        return ""
