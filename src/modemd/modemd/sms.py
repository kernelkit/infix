from datetime import datetime
import argparse
import locale
import subprocess
import json
import glob
import re
import sys
import os

LOCALEPATH = "/usr/lib/locale"


def run_cmd(cmd):
    try:
        res = subprocess.run(cmd, capture_output=True, text=True)
        return True if res.returncode == 0 else False
    except subprocess.CalledProcessError:
        return False


def send(index, number, text):
    if index < 0:
        # use first modem by default
        index = 0

    rpc = {
        "ietf-hardware:hardware": {
            "component": [{
                "name": "modem%d" % index,
                "infix-hardware:modem": {
                    "send-sms": {
                        "phone-number": number,
                        "message-text": text
                    }
                }
            }]
        }
    }
    print("Sending SMS to modem%d" % index)

    path = "/run/modemd/modem%d/rpc" % index
    if os.path.exists(path):
        sys.stderr.write("Error: another rpc is running\n")
        return

    with open(path, "w") as fd:
        json.dump(rpc, fd)

    if not run_cmd(['sysrepocfg', '-f', 'json', '-R', path]):
        sys.stderr.write("Error: Unable to send SMS\n")

    os.unlink(path)


def listsms():
    smslist = []
    files = glob.glob('/var/sms/*')
    for f in files:
        with open(f, "r") as fd:
            sms = json.load(fd)
            sms["path"] = f

            t = sms["payload"]["properties"]["timestamp"]
            t = re.sub("[+-][\\d:]+$", "", t)
            sms["time"] = datetime.strptime(t, "%Y-%m-%dT%H:%M:%S")

            smslist.append(sms)

    return sorted(smslist, key=lambda sms: sms["time"])


def delete(index=-1):
    count = 0
    for sms in listsms():
        if index > -1 and sms["modem"] == index:
            os.remove(sms["path"])
            count += 1
    print("Deleted %d SMS files" % count)


def show(index=-1):
    for sms in listsms():
        if index > -1 and sms["modem"] != index:
            continue

        content = sms["payload"]["content"]
        prop = sms["payload"]["properties"]

        print("--- %s ---\n" % os.path.basename(sms["path"]))
        print("From: %s" % content["number"])
        print("SMSC: %s" % prop["smsc"])
        print("Modem: %d" % sms["modem"])
        print("Date: %s" % prop["timestamp"])
        print("State: %s" % prop["state"])
        print("\n%s\n" % content["text"])


def listlocale():
    locales = []
    for f in os.listdir(LOCALEPATH):
        if os.path.isdir(os.path.join(LOCALEPATH, f)):
            locales.append(f)
    return ", ".join(locales)


def main():
    parser = argparse.ArgumentParser(prog='modem-sms')
    parser.add_argument("-d", action="store_true", help="Delete SMS")
    parser.add_argument("-s", action="store_true", help="Send SMS")

    parser.add_argument("-i", "--index", default=0, help="Modem index")
    parser.add_argument("-n", "--number", default=None, help="Phone number")
    parser.add_argument("-t", "--text", default=None, help="Message text")

    parser.add_argument("-l", "--locale", default=None, help="Set locale")
    args = parser.parse_args()

    index = -1
    if args.index:
        index = int(args.index)

    if args.locale:
        try:
            locale.setlocale(locale.LC_ALL, args.locale)
        except locale.Error:
            print("Error: Invalid locale (available: %s)" % listlocale())
            sys.exit(1)

    if args.s:
        if not args.number:
            sys.stderr.write("Error: need number\n")
        elif not args.text:
            sys.stderr.write("Error: need text\n")
        else:
            send(index, args.number, args.text)
    elif args.d:
        delete(index)
    else:
        show(index)


if __name__ == "__main__":
    main()
