import subprocess
import argparse
import json
import time
import syslog
import sys

syslog.openlog(logoption=syslog.LOG_PID, facility=syslog.LOG_SYSLOG)


def log(msg):
    syslog.syslog(syslog.LOG_INFO, msg)


def err(msg):
    syslog.syslog(syslog.LOG_ERR, msg)


def fatal(msg):
    syslog.syslog(syslog.LOG_ALERT, msg)
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


def reset(index, info):
    log("Power-cycling modem %d" % index)

    with open("%s/authorized" % info["devpath"], "w") as fd:
        fd.write("0\n")
    time.sleep(0.5)

    path = "/sys/class/pcie/slot%d/pwrctl" % info["slot"]
    with open(path, "w") as fd:
        fd.write("cycle\n")

    return True


def main():
    parser = argparse.ArgumentParser(prog='modem-power')
    parser.add_argument("-i", "--index", default=0, help="Modem index")
    args = parser.parse_args()

    if args.index:
        index = int(args.index)
    else:
        index = 0

    info = runcmdj(['/usr/libexec/modemd/modem-info',
                    '-i', str(index)])
    if info is None:
        fatal("Unable to obtain modem info")

    if not reset(index, info):
        fatal("Unable to power-cycle modem")

    sys.exit(0)


if __name__ == "__main__":
    main()
