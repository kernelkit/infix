import time
import infamy.iface
def wait(func, *args):
    timeout = 10
    while(timeout>0):
        if len(args) == 0:
            f=func()
        else:
            f=func(*args)

        if(f):
            return True
        timeout-=1
        time.sleep(1)

    return False

def wait_links(target, ifaces):
    for i in ifaces:
        if not wait(infamy.iface.get_oper_up, target, i):
             raise Exception("Interface did not come up in time.")
        
    
def wait_mac_address(target, iface, mac):
    if not wait(infamy.iface.is_phys_address, target, iface, mac):
        raise Exception("Failed waiting for MAC address")
