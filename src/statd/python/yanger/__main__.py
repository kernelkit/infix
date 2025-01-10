import logging
import logging.handlers
import subprocess
import json
import sys  # (built-in module)
import os
import argparse

from . import common
from . import host

def main():
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
        host.HOST = host.Testhost(args.test)
    else:
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
        common.LOG.warning(f"Unsupported model {args.model}", file=sys.stderr)
        sys.exit(1)

    print(json.dumps(yang_data, indent=2))

if __name__ == "__main__":
    main()
