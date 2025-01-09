#!/usr/bin/env python3
import logging
import logging.handlers
import subprocess
import json
import sys  # (built-in module)
import os
import argparse
from datetime import datetime, timedelta, timezone

from . import common
from . import host

TESTPATH = ""
logger = None

def datetime_now():
    if TESTPATH:
        return datetime(2023, 1, 1, 12, 0, 0, tzinfo=timezone.utc)
    return datetime.now(timezone.utc)


def insert(obj, *path_and_value):
    """"This function inserts a value into a nested json object"""
    if len(path_and_value) < 2:
        raise ValueError("Error: insert() takes at least two args")

    *path, value = path_and_value

    curr = obj
    for key in path[:-1]:
        if key not in curr or not isinstance(curr[key], dict):
            curr[key] = {}
        curr = curr[key]

    curr[path[-1]] = value


def run_cmd(cmd, testfile, default=None, check=True):
    """Run a command (array of args) and return an array of lines"""

    if TESTPATH and testfile:
        cmd = ['cat', os.path.join(TESTPATH, testfile)]

    try:
        result = subprocess.run(cmd, check=check, stderr=subprocess.DEVNULL,
                                stdout=subprocess.PIPE, text=True)
        output = result.stdout
        return output.splitlines()
    except subprocess.CalledProcessError as err:
        common.LOG.error(f"{err}")
        if default is not None:
            return default
        raise


def run_json_cmd(cmd, testfile, default=None, check=True):
    """Run a command (array of args) with JSON output and return the JSON"""

    if TESTPATH and testfile:
        cmd = ['cat', os.path.join(TESTPATH, testfile)]

    try:
        result = subprocess.run(cmd, check=check, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, text=True)
        output = result.stdout
        data = json.loads(output)
    except subprocess.CalledProcessError as err:
        common.LOG.error(f"{err}")
        if default is not None:
            return default
        raise
    except json.JSONDecodeError as err:
        if check is True:
            common.LOG.error("failed parsing JSON output of command: "
                         f"{' '.join(cmd)}, error: {err}")
        if default is not None:
            return default
        raise
    return data


def main():
    global TESTPATH
    global logger

    parser = argparse.ArgumentParser(description="YANG data creator")
    parser.add_argument("model", help="YANG Model")
    parser.add_argument("-p", "--param", default=None, help="Model dependent parameter")
    parser.add_argument("-t", "--test", default=None, help="Test data base path")
    args = parser.parse_args()


    # Set up syslog output for critical errors to aid debugging
    common.LOG = logging.getLogger('yanger')
    if os.path.exists('/dev/log'):
        log = logging.handlers.SysLogHandler(address='/dev/log')
    else:
        # Use /dev/null as a fallback for unit tests
        log = logging.FileHandler('/dev/null')

    fmt = logging.Formatter('%(name)s[%(process)d]: %(message)s')
    log.setFormatter(fmt)
    common.LOG.setLevel(logging.INFO)
    common.LOG.addHandler(log)

    if args.test:
        TESTPATH = args.test
        host.HOST = host.Testhost(args.test)
    else:
        TESTPATH = ""
        host.HOST = host.Localhost()

    if args.model == 'ietf-interfaces':
        from . import ietf_interfaces
        yang_data = ietf_interfaces.operational(args.param)
    elif args.model == 'ietf-routing':
        from . import ietf_routing
        yang_data = ietf_routing.operational()
    elif args.model == 'ietf-ospf':
        from . import ietf_ospf
        yang_data = ietf_ospf.operational()
    elif args.model == 'ietf-hardware':
        from . import ietf_hardware
        yang_data = ietf_hardware.operational()
    elif args.model == 'infix-containers':
        from . import infix_containers
        yang_data = infix_containers.operational()
    elif args.model == 'ietf-system':
        from . import ietf_system
        yang_data = ietf_system.operational()
    else:
        logger.warning(f"Unsupported model {args.model}", file=sys.stderr)
        sys.exit(1)

    print(json.dumps(yang_data, indent=2))


if __name__ == "__main__":
    main()
