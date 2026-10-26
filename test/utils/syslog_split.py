#!/usr/bin/env python3
"""Filter or split the DUT log collected by test/syslog-sink

Lines are sorted to the test that runs when they are logged, from the
Infamy markers, and to the device that sent them, from its physical
name, or its hostname if it has none.  Lines logged between two tests
belong to the next test, it is resetting the DUTs.

  syslog_split.py -t 0006-set-hostname -d dut1 syslog.log
  syslog_split.py -o syslog/ syslog.log     # syslog/<test>/<device>.log
"""
import argparse
import os
import re
import sys

TEST_SDID = "test@32473"
DEVICE_SDID = "device@32473"
HEADER = re.compile(r"^<\d+>1 ")
PARAM = re.compile(r'(\S+?)="((?:[^"\\]|\\.)*)"')
ELEMENT = re.compile(r'\[(\S+?)((?: \S+?="(?:[^"\\]|\\.)*")*)\]')


def parse_sd(text):
    """Return {SD-ID: {name: value}} for the RFC 5424 STRUCTURED-DATA at the start of text"""
    sd, pos = {}, 0
    while m := ELEMENT.match(text, pos):
        sd[m[1]] = dict(PARAM.findall(m[2]))
        pos = m.end()
    return sd


def parse(line):
    """Return (device, msgid, sd) of an RFC 5424 log line, None if it is not one"""
    fields = HEADER.sub("", line, count=1).split(" ", 5)
    if len(fields) < 6:
        return None

    _, host, _, _, msgid, rest = fields
    sd = parse_sd(rest)
    device = sd.get(DEVICE_SDID, {}).get("name", host)
    return device, msgid, sd


def split(lines):
    """Yield (test, device, line), test is None for lines after the last test"""
    test = None
    pending = []

    for line in lines:
        entry = parse(line)
        if not entry:
            continue

        device, msgid, sd = entry
        marked = sd.get(TEST_SDID, {}).get("name")
        if msgid == "test-start" and marked:
            test = marked
            for dev, ln in pending:
                yield test, dev, ln
            pending = []

        owner = marked or test
        if owner:
            yield owner, device, line
        else:
            pending.append((device, line))

        if msgid == "test-stop" and marked == test:
            test = None

    for dev, ln in pending:
        yield None, dev, ln


def main():
    parser = argparse.ArgumentParser(description="Filter or split the DUT log from test/syslog-sink")
    parser.add_argument("-t", "--test", help="only lines from this test")
    parser.add_argument("-d", "--device", help="only lines from this device")
    parser.add_argument("-o", "--outdir", help="write each test's log per device to OUTDIR/<test>/<device>.log")
    parser.add_argument("log", help="log file, - for stdin")
    args = parser.parse_args()

    f = sys.stdin if args.log == "-" else open(args.log, errors="replace")
    files = {}
    try:
        for test, device, line in split(f):
            if args.test and test != args.test:
                continue
            if args.device and device != args.device:
                continue

            if not args.outdir:
                sys.stdout.write(line)
                continue

            path = os.path.join(args.outdir, test or "after-last-test", f"{device}.log")
            if path not in files:
                os.makedirs(os.path.dirname(path), exist_ok=True)
                files[path] = open(path, "w")
            files[path].write(line)
    finally:
        for out in files.values():
            out.close()


if __name__ == "__main__":
    main()
