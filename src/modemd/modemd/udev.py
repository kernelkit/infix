import shutil
import errno
import time
import json
import fcntl
import sys
import re
import os

MODEMS = "/run/modems.json"
SYSTEM = "/run/system.json"
LOCK = "/var/lock/modems.lock"
LOGFILE = "/run/modemd/modem-udev.log"
debug = False


def log(msg):
    ts = '{:010.3f}'.format(time.monotonic())
    with open(LOGFILE, 'a') as fd:
        fd.write("[%s] modem-udev (pid %d) %s\n" % (ts, os.getpid(), msg))


def dbg(msg):
    global debug
    if debug:
        log(msg)


def info(msg):
    log(msg)


def err(msg):
    log("Error: %s" % msg)


def lock():
    start = time.time()
    while True:
        elapsed = time.time() - start
        if elapsed > 10:
            err("Timeout waiting for lock")
            break
        try:
            fd = open(LOCK, "a")
            fcntl.flock(fd.fileno(), fcntl.LOCK_EX)
            dbg("Acquired lock")
            return fd
        except IOError as e:
            dbg("Unable to lock (%d)" % e.errno)
            if e.errno != errno.EAGAIN:
                err("Unable to lock")
                return None
            else:
                time.sleep(1)

    return None


def unlock(fd):
    try:
        fcntl.flock(fd.fileno(), fcntl.LOCK_UN)
        fd.close()
        dbg("Released lock")
        return True
    except IOError as e:
        err("Unlock failed with %d" % e.errno)
        return False


def mkdir(path):
    if os.path.isdir(path):
        return True
    try:
        os.mkdir(path, mode=0o755)
        shutil.chown(path, user="root", group="wheel")
    except OSError:
        err("Unable to mkdir %s" % path)
        return False
    finally:
        return True


def read_modem_data():
    data = {}
    if os.path.exists(MODEMS):
        with open(MODEMS, "r") as fd:
            data = json.load(fd)
    return data


def write_modem_data(data):
    with open(MODEMS + ".bak", 'w') as fd:
        json.dump(data, fd)

    os.rename(MODEMS + ".bak", MODEMS)
    return True


def _sysfs_attr(devpath, name):
    try:
        with open("%s/%s" % (devpath, name), "r") as fd:
            return fd.readline().strip()
    except OSError:
        return None


def update_system_json(modems_list):
    data = {}
    try:
        with open(SYSTEM, "r") as fd:
            data = json.load(fd)
    except (IOError, json.JSONDecodeError):
        pass

    entries = []
    for i, m in enumerate(modems_list):
        entries.append({
            "index": i,
            "name": "modem%d" % i,
            "devpath": m.get("devpath", ""),
            "vid": m.get("vendor", ""),
            "pid": m.get("product", ""),
        })

    if not entries:
        data.pop("modem", None)
    else:
        data["modem"] = entries

    tmp = SYSTEM + ".tmp"
    with open(tmp, "w") as fd:
        json.dump(data, fd, indent=2)
    os.rename(tmp, SYSTEM)
    info("Updated system.json with %d modem(s)" % len(entries))


def update(d):
    modem = None
    devpath = d.get("devpath")

    data = read_modem_data()
    if "modems" not in data:
        data = {"modems": []}
    else:
        for m in data["modems"]:
            if devpath == m.get("devpath"):
                data["modems"].remove(m)
                modem = m
                break

    if not modem:
        modem = {"devpath": devpath}

    vendor = _sysfs_attr(devpath, "idVendor")
    if vendor:
        modem["vendor"] = vendor

    product = _sysfs_attr(devpath, "idProduct")
    if product:
        modem["product"] = product

    atport = d.get("atport")
    atports = modem.get("atports", [])
    if atport and atport not in atports:
        ptype = d.get("ptype", "default")
        if ptype == "primary":
            atports.insert(0, atport)
        else:
            atports.append(atport)
        modem["atports"] = atports

    qmiport = d.get("qmiport")
    qmiports = modem.get("qmiports", [])
    if qmiport and qmiport not in qmiports:
        qmiports.append(qmiport)
        modem["qmiports"] = qmiports

    iface = d.get("iface")
    ifaces = modem.get("interfaces", [])
    if iface and iface not in ifaces:
        ifaces.append(iface)
        modem["interfaces"] = ifaces

    data["modems"].append(modem)
    write_modem_data(data)
    update_system_json(data["modems"])

    info("Updated modem %s" % devpath)


def add_tty(devpath):
    devpath = os.path.realpath("/sys/%s/../../../../" % devpath)

    info("Adding tty device %s" % devpath)

    atport = os.getenv("DEVNAME")
    if not atport:
        err("No DEVNAME")
        return False

    if os.getenv("ID_MM_PORT_TYPE_AT_PRIMARY"):
        ptype = "primary"
    elif os.getenv("ID_MM_PORT_TYPE_AT_SECONDARY"):
        ptype = "secondary"
    else:
        ptype = "default"

    info("Adding %s atport %s for %s" % (ptype, atport, devpath))

    update({"devpath": devpath, "atport": atport, "ptype": ptype})


def add_net(devpath):
    devpath = os.path.realpath("/sys/%s/../../../" % devpath)

    info("Adding net device %s" % devpath)

    iface = os.getenv("INTERFACE", None)
    if not iface:
        err("No INTERFACE")
        return False

    r = re.search(r"wwan(\d+)\.(\d+)", iface)
    if r and r.group(2):
        info("Skipping interface %s" % iface)
        return False

    info("Adding interface %s for %s" % (iface, devpath))
    update({"devpath": devpath, "iface": iface})

    try:
        with open("/sys/class/net/%s/qmi/raw_ip" % iface, "w") as fd:
            fd.write("Y\n")
    except (IOError, OSError):
        err("Unable to set raw-ip")
        return False

    info("Interface %s has been set to rawip" % iface)


def add_usbmisc(devpath):
    devpath = os.path.realpath("/sys/%s/../../../" % devpath)

    info("Adding usbmisc device %s" % devpath)

    qmiport = os.getenv("DEVNAME", None)
    if not qmiport:
        err("No DEVNAME")
        return False

    info("Adding qmiport %s for %s" % (qmiport, devpath))
    update({"devpath": devpath, "qmiport": qmiport})


def apply():
    subsystem = os.getenv("SUBSYSTEM")
    if not subsystem:
        err("No SUBSYSTEM")
        return False

    devpath = os.getenv("DEVPATH")
    if not devpath:
        err("No DEVPATH")
        return False

    if "usb" not in devpath:
        info("Skipping %s" % devpath)
        return False

    if subsystem == "net":
        add_net(devpath)
    elif subsystem == "tty":
        add_tty(devpath)
    elif subsystem == "usbmisc":
        add_usbmisc(devpath)
    else:
        err("Subsystem %s unhandled" % subsystem)
        return False

    return True


def main():
    global debug
    with open("/proc/cmdline", 'r') as fd:
        cmdline = fd.read()
        if "debug" in cmdline:
            debug = True

    mkdir("/run/modemd")
    dbg("started")

    if debug:
        for i, j in os.environ.items():
            dbg("ENV: %s=%s" % (i, j))
    else:
        if os.path.exists(LOGFILE):
            os.remove(LOGFILE)

    fd = lock()
    if fd is None:
        sys.exit(2)

    if apply():
        rc = 0
    else:
        err("Unable to add modem device")
        rc = 1

    unlock(fd)

    dbg("ended")

    sys.exit(rc)


if __name__ == "__main__":
    main()
