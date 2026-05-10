import subprocess
import argparse
import urllib.parse
import hashlib
import syslog
import json
import time
import os
import sys

tempdir = "/tmp/modem-fwupdate"
debug = False
index = 0

syslog.openlog(logoption=syslog.LOG_PID, facility=syslog.LOG_SYSLOG)


def dbg(msg):
    global debug
    if debug:
        syslog.syslog(syslog.LOG_INFO, msg)


def info(msg):
    syslog.syslog(syslog.LOG_INFO, msg)


def err(msg):
    syslog.syslog(syslog.LOG_ERR, msg)


def fatal(msg):
    syslog.syslog(syslog.LOG_ALERT, msg)
    rmdir(tempdir)
    sys.exit(1)


def runcmd(cmd):
    ret = None
    try:
        res = subprocess.run(cmd, check=True, stdout=subprocess.PIPE,
                             stderr=subprocess.PIPE, text=True)
        if res.returncode == 0:
            if res.stdout:
                ret = res.stdout.strip()
            else:
                ret = True
    except subprocess.CalledProcessError:
        err("Command failed")
        dbg(res.stderr)
        return None
    finally:
        return ret


def runcmdj(cmd):
    output = runcmd(cmd)
    if not output:
        return None
    try:
        data = json.loads(output)
    except json.JSONDecodeError:
        return None
    finally:
        return data


def exists(path):
    try:
        os.stat(path)
    except OSError:
        return False
    return True


def rm(path):
    if os.path.exists(path):
        os.remove(path)


def rmdir(path):
    if os.path.isdir(path):
        for f in os.listdir(path):
            p = os.path.join(path, f)
            if os.path.isdir(p):
                rmdir(p)
            else:
                os.remove(p)


def mkdir(path):
    if os.path.isdir(path):
        return True
    try:
        os.mkdir(path, mode=0o755)
    except OSError:
        fatal("Unable to mkdir %s" % path)


def initctl(cmd, svc):
    output = runcmdj(['initctl', '-j', 'status'])
    for ctl in output:
        if ctl["identity"] == svc:
            return runcmd(['initctl', cmd, svc])
    return False


def get_path(index):
    for modem in runcmdj(['/usr/libexec/modemd/modem-info']):
        if modem["index"] == index:
            return modem["path"]


def get_device_info(index):
    path = get_path(index)
    if path is None:
        return None

    output = runcmdj(['mmcli', '-J', '-m', path])
    if not output:
        return None

    gen = output["modem"]["generic"]
    info = {
        "index": index,
        "vendor": gen["manufacturer"],
        "model": gen["model"],
        "state": gen["state"],
        "qmidev": [],
        "atdev": []
    }
    for port in gen["ports"]:
        if "(qmi)" in port:
            info["qmidev"].append(port.split()[0])
        elif "(at)" in port:
            info["atdev"].append(port.split()[0])

    return info


def wait_for_device(dev, appear):
    if appear:
        mode = "appear"
    else:
        mode = "disappear"

    info("Waiting for modem%d to %s" % (dev["index"], mode))

    path = "/dev/%s" % dev["qmidev"][0]
    timeout = 30
    while timeout > 0:
        if mode == "appear":
            if exists(path):
                return True
        elif mode == "disappear":
            if not exists(path):
                return True
        time.sleep(1)
        timeout -= 1
    return False


def qmiupdate(dev):
    if len(dev["qmidev"]) == 0:
        err("No qmi device found")
        return False

    qmidev = "/dev/%s" % dev["qmidev"][0]
    cwefile = None
    nvufile = None

    for f in os.scandir(tempdir):
        if f.name.endswith('cwe'):
            cwefile = "%s/%s" % (tempdir, f.name)
        if f.name.endswith('nvu'):
            nvufile = "%s/%s" % (tempdir, f.name)

    if not cwefile:
        err("No CWE file found")
        return False
    if not nvufile:
        err("No NVU file found")
        return False

    info("Resetting modem")
    if not runcmd(['qmicli', '-d', qmidev,
                   '--dms-set-operating-mode=offline']):
        err("Unable to set modem offline")
        return False
    if not runcmd(['qmicli', '-d', qmidev,
                   '--dms-set-operating-mode=reset']):
        err("Unable to reset modem")
        return False

    if not wait_for_device(dev, 0):
        err("Device has not disappeared")
        return False
    if not wait_for_device(dev, 1):
        err("Device has not appeared")
        return False

    info("Running qmi-firmware-update, please stand by...")

    cmd = ['qmi-firmware-update', '-v', '--override-download',
           '-w', qmidev, '-u', cwefile, nvufile]
    return runcmd(cmd)


def firmware_update_sierra(dev, url, checksum):
    if not download(url, checksum):
        err("Unable to prepare firmware")
        return False

    info("Stopping modem services")
    initctl("stop", "modemd")
    initctl("stop", "modem-manager")

    time.sleep(3)
    ret = qmiupdate(dev)

    info("Starting modem services")
    initctl("restart", "modem-manager")
    initctl("restart", "modemd")

    return ret


def atcmd(dev, command, expect=None, timeout=None):
    device = "/dev/%s" % dev["atdev"][0]

    cmd = ['modem-command', '-d', device, command]
    if expect:
        cmd += ['-e', expect]
    if timeout:
        cmd += ['-t', str(timeout)]

    return runcmd(cmd)


def firmware_update_telit(dev, url, checksum):
    if 'LN920' not in dev["model"]:
        err("Not supported")
        return False

    if len(dev["atdev"]) == 0:
        err("No AT device found")
        return False

    if dev["state"] != "connected":
        err("Modem is not connected")
        return False

    u = urllib.parse.urlparse(url)
    if not u:
        err("Unable to parse URL")
        return False
    if u.scheme != "ftp":
        err("Not an FTP URL")
        return False

    hostname = ""
    username = ""
    password = ""
    port = 21

    if u.hostname:
        hostname = u.hostname
    if u.username:
        username = u.username
    if u.password:
        password = u.password
    if u.port:
        port = int(u.port)
    if u.path:
        path = u.path

    if hostname == "":
        err("No hostname in URL")
        return False
    if path == "":
        err("No path in URL")
        return False

    parms = (hostname, port, path, username, password)
    cmd = 'AT#FTPGETOTAENH=%s,%d,%s,%s,%s' % parms
    exp = '#DREL'
    if not atcmd(dev, cmd, exp, 60):
        err("Unable to download firmware")
        return False

    if not atcmd(dev, 'AT#OTAUP=2'):
        err("Unable to update firmware")
        return False

    if not atcmd(dev, 'AT#ENHRST=1,3'):
        err("Unable to reboot modem")
        return False

    time.sleep(5)
    info("Restarting modem services")
    initctl("restart", "modem-manager")
    initctl("restart", "modemd")
    return True


def verify(path, checksum):
    if not checksum or checksum == "any":
        return True

    sha256 = hashlib.sha256()
    with open(path, 'rb') as f:
        while True:
            data = f.read(1024)
            if not data:
                break
            sha256.update(data)

    calculated = sha256.hexdigest()

    if checksum != calculated:
        err("Checksum mismatch (%s vs. %s)" % (calculated, checksum))
        return False
    else:
        info("Verified checksum")
        return True


def download(url, checksum):
    path = "%s/firmware.zip" % tempdir

    info("Downloading %s" % url)

    if not runcmd(['curl', '-s', '-L', url, '-o', path]):
        err("Unable to download firmware")
        return False

    if not verify(path, checksum):
        err("Checksum verification failed")
        return False

    if not runcmd(['unzip', '-d', tempdir, path]):
        err("Unable to unpack firmware")
        return False

    return True


def main():
    global debug, index
    syslog.openlog(logoption=syslog.LOG_PID, facility=syslog.LOG_DAEMON)

    parser = argparse.ArgumentParser(prog='modem-firmware-update')
    parser.add_argument("-i", "--index", default=None, help="Modem index")
    parser.add_argument("-u", "--url", default=None, help="URL to firmware")
    parser.add_argument("-c", "--checksum", default=None,
                        help="SHA256 checksum of firmware")
    parser.add_argument('-d', action='store_true')

    args = parser.parse_args()
    if args.d:
        debug = True
    if args.index:
        index = int(args.index)
    if not args.url:
        fatal("No firmware URL specified")

    rmdir(tempdir)
    mkdir(tempdir)

    dev = get_device_info(index)
    if dev is None:
        fatal("No device found for modem%d" % index)

    info("Starting firmware update for %s" % dev["model"])

    if "Sierra" in dev["vendor"]:
        ret = firmware_update_sierra(dev, args.url, args.checksum)
    elif "Telit" in dev["vendor"]:
        ret = firmware_update_telit(dev, args.url, args.checksum)
    else:
        fatal("Unsupported vendor")

    if ret:
        info("Firmware update succeeded")
    else:
        fatal("Firmware update failed")

    rmdir(tempdir)


if __name__ == "__main__":
    main()
