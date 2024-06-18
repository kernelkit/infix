import time
import threading
import infamy.neigh
import infamy.netconf as netconf
import infamy.restconf as restconf

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

def wait_boot(target):
    until(lambda: target.reachable() == False, attempts = 100)
    print("Device is booting..")
    until(lambda: target.reachable() == True, attempts = 300)
    iface=target.get_mgmt_iface()
    if not iface:
        return False
    neigh=infamy.neigh.ll6ping(iface)
    if not neigh:
        return False
    until(lambda: netconf.netconf_syn(neigh) == True, attempts = 300)
    until(lambda: restconf.restconf_reachable(neigh, target.location.password) == True, attempts = 300)

    return True
