import subprocess
import argparse
import json
import syslog
import sys
import os
import re

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


def read_file(path):
    if os.path.exists(path):
        with open(path, "r") as fd:
            output = str(fd.read())
            if output:
                return output.strip()
    return None


# Telit LE910
le910_carriers = [
    {"num":   0, "name": "AT&T"},
    {"num":   1, "name": "Verizon"},
    {"num":   2, "name": "T-Mobile"},
    {"num":   3, "name": "Bell"},
    {"num":   4, "name": "Telus"},
    {"num":  10, "name": "NTT-Docomo"},
    {"num":  11, "name": "Telstra"},
    {"num":  12, "name": "KDDI"},
    {"num":  13, "name": "Softbank"},
    {"num":  14, "name": "Vodafone New Zealand"},
    {"num":  15, "name": "Spark New Zealand"},
    {"num":  20, "name": "China Mobile"},
    {"num":  21, "name": "China Unicom"},
    {"num":  22, "name": "China Telecom"},
    {"num":  30, "name": "Sprint"},
    {"num":  31, "name": "SouthernLINC"},
    {"num":  40, "name": "Global"},
    {"num": 101, "name": "T-Mobile Germany"},
    {"num": 102, "name": "AT&T Mexico"},
    {"num": 103, "name": "Orange-WW"},
    {"num": 104, "name": "Southern Linc USA"},
    {"num": 105, "name": "Vodafone DE"}
]


def le910_carrier_bynum(num):
    for carrier in le910_carriers:
        if carrier["num"] == num:
            return carrier
    return None


def le910_carrier_byname(name):
    for carrier in le910_carriers:
        if carrier["name"] == name:
            return carrier
    return None


def le910_is_supported(variant):
    if variant == "NS":
        return True
    elif variant == "AP":
        return True
    elif variant == "NF" or variant == "NFD":
        return True
    elif variant == "CN":
        return True
    elif variant == "APX":
        return True
    elif variant == "WWX" or variant == "WWXD":
        return True
    else:
        return False


def le910_carrier_default(variant):
    if variant == "NF" or variant == "WWX" or variant == "WWXD":
        num = 0
    elif variant == "AP" or variant == "APX":
        num = 10
    elif variant == "CN":
        num = 20
    elif variant == "NS":
        num = 30
    elif variant == "EU" or variant == "EUX":
        num = 40
    else:
        return None

    return le910_carrier_bynum(num)


def le910_command(index, command):
    cmd = ['/sbin/modem-command',
           '--index', str(index),
           '--mode', '115200 8N1',
           '--secondary',
           '--timeout', '5',
           command]

    for attempt in range(1, 3):
        output = runcmd(cmd)
        if output:
            return output

    err("Command '%s' failed" % " ".join(cmd))
    return None


def le910_list_carriers(index):
    output = le910_command(index, 'AT#FWSWITCH=?')
    if output is None:
        return None

    r = re.search(r"#FWSWITCH: \(([^\)]*)\)", output)
    if not r:
        err("Unable to list carriers on modem%d" % index)
        return None

    rlist = r.group(1)
    nums = []
    for rl in rlist.split(","):
        r = re.search(r"(\d+)-(\d+)", rl)
        if r:
            start = int(r.group(1))
            end = int(r.group(2))
            for i in range(start, end + 1):
                nums.append(i)
        else:
            nums.append(int(rl))

    carriers = []
    for num in nums:
        carrier = le910_carrier_bynum(num)
        if carrier:
            carriers.append(carrier)
        else:
            err("Unknown carrier %d" % num)

    return carriers


def le910_set_carrier(index, carrier):
    output = le910_command(index, 'AT#FWSWITCH=%d' % carrier["num"])
    if output is None:
        return False
    else:
        return True


def le910_get_carrier(index):
    output = le910_command(index, 'AT#FWSWITCH?')
    if output is None:
        return -1

    r = re.search(r"#FWSWITCH: (\d+)", output)
    if not r:
        err("Unable to query carrier on modem%d" % index)
        return -1
    num = int(r.group(1))

    return le910_carrier_bynum(num)


def runcheck(manf, model):
    if manf == "Telit" and model[:5] == "LE910":
        return le910_is_supported(model.split("-")[1])
    else:
        return False


def runset(index, manf, model, name):
    if not runcheck(manf, model):
        fatal("Unsupported modem")

    if name == "default":
        carrier = le910_carrier_default(model.split("-")[1])
    else:
        carrier = le910_carrier_byname(name)
    if not carrier:
        fatal("Invalid carrier '%s'" % name)

    current = le910_get_carrier(index)
    if not current:
        fatal("Cannot get current carrier")

    if current["name"] == carrier["name"]:
        log("Carrier '%s' already set on modem%d" % (carrier["name"], index))
        print(json.dumps({"action": "none"}))
        sys.exit(0)

    log("Setting carrier '%s' on modem%d" % (carrier["name"], index))
    if not le910_set_carrier(index, carrier):
        fatal("Unable to set carrier on modem%d" % index)

    print(json.dumps({"action": "reset"}))
    sys.exit(0)


def runget(index, manf, model):
    if not runcheck(manf, model):
        fatal("Unsupported modem")

    carrier = le910_get_carrier(index)
    if not carrier:
        fatal("Unable to get carrier")

    print(json.dumps(carrier))
    sys.exit(0)


def runlist(index, manf, model):
    if not runcheck(manf, model):
        print(json.dumps([]))
        sys.exit(0)

    carriers = le910_list_carriers(index)
    if not carriers:
        fatal("No carriers")

    names = []
    for carrier in sorted(carriers, key=lambda c: c["name"]):
        names.append(carrier["name"])

    print(json.dumps(names))
    sys.exit(0)


def main():
    parser = argparse.ArgumentParser(prog='modem-carrier')
    parser.add_argument("-i", "--index", default=0, help="Modem index")
    parser.add_argument("-m", "--manf", default=0, help="Modem manufacturer")
    parser.add_argument("-M", "--model", default=0, help="Modem model")
    parser.add_argument("-s", "--set", help="Set carrier")
    parser.add_argument("-g", "--get", help="Get carrier",
                        action="store_true")
    parser.add_argument("-l", "--list", help="List supported carriers",
                        action="store_true")
    parser.add_argument("-c", "--check", help="Check support for carriers",
                        action="store_true")
    args = parser.parse_args()

    if not args.manf:
        fatal("No manufacturer given")
    else:
        manf = args.manf

    if not args.model:
        fatal("No model given")
    else:
        model = args.model

    if args.index:
        index = int(args.index)
    else:
        index = 0

    if args.check:
        if not runcheck(manf, model):
            sys.exit(1)
    elif args.list:
        runlist(index, manf, model)
    elif args.set:
        runset(index, manf, model, args.set)
    elif args.get:
        runget(index, manf, model)

    sys.exit(0)


if __name__ == "__main__":
    main()
