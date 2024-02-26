import time
import infamy.neigh
import infamy.netconf as netconf

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
    return True
