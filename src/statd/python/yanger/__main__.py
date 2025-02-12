import logging
import logging.handlers
import json
import sys  # (built-in module)
import os
import argparse

from . import common
from . import host

def main():
    def dirpath(path):
        if not os.path.isdir(path):
            raise argparse.ArgumentTypeError(f"'{path}' is not a valid directory")
        return path

    parser = argparse.ArgumentParser(description="YANG data creator")
    parser.add_argument("model", help="YANG Model")
    parser.add_argument("-p", "--param",
                        help="Model dependent parameter, e.g. interface name")
    parser.add_argument("-x", "--cmd-prefix", metavar="PREFIX",
                        help="Use this prefix for all system commands, e.g. " +
                        "'ssh user@remotehost sudo'")

    rrparser = parser.add_mutually_exclusive_group()
    rrparser.add_argument("-r", "--replay", type=dirpath, metavar="DIR",
                          help="Generate output based on recorded system commands from DIR, " +
                          "rather than querying the local system")
    rrparser.add_argument("-c", "--capture", metavar="DIR",
                          help="Capture system command output in DIR, such that the current system " +
                          "state can be recreated offline (with --replay) for testing purposes")

    args = parser.parse_args()
    if args.replay and args.cmd_prefix:
        parser.error("--cmd-prefix cannot be used with --replay")

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

    if args.cmd_prefix or args.capture:
        host.HOST = host.Remotehost(args.cmd_prefix, args.capture)
    elif args.replay:
        host.HOST = host.Replayhost(args.replay)
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
    elif args.model == 'infix-dhcp-server':
        from . import infix_dhcp_server
        yang_data = infix_dhcp_server.operational()
    elif args.model == 'ietf-system':
        from . import ietf_system
        yang_data = ietf_system.operational()
    elif args.model == 'ieee802-dot1ab-lldp':
        from . import infix_lldp           
        yang_data = infix_lldp.operational()
    else:
        common.LOG.warning("Unsupported model %s", args.model)
        sys.exit(1)

    print(json.dumps(yang_data, indent=2))


if __name__ == "__main__":
    main()
