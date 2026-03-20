import json
import os
import sys

from . import common
from . import host

USAGE = """\
usage: yanger [-p PARAM] [-x PREFIX] [-r DIR | -c DIR] model

YANG data creator

positional arguments:
  model                 YANG Model

options:
  -p, --param PARAM     Model dependent parameter, e.g. interface name
  -x, --cmd-prefix PREFIX
                        Use this prefix for all system commands, e.g.
                        'ssh user@remotehost sudo'
  -r, --replay DIR      Generate output based on recorded system commands
                        from DIR, rather than querying the local system
  -c, --capture DIR     Capture system command output in DIR, such that the
                        current system state can be recreated offline (with
                        --replay) for testing purposes
"""

def _parse_args(argv):
    model = None
    param = None
    cmd_prefix = None
    replay = None
    capture = None

    i = 1
    while i < len(argv):
        arg = argv[i]
        if arg in ('-h', '--help'):
            sys.stdout.write(USAGE)
            sys.exit(0)
        elif arg in ('-p', '--param'):
            i += 1
            if i >= len(argv):
                sys.exit(f"error: {arg} requires an argument")
            param = argv[i]
        elif arg in ('-x', '--cmd-prefix'):
            i += 1
            if i >= len(argv):
                sys.exit(f"error: {arg} requires an argument")
            cmd_prefix = argv[i]
        elif arg in ('-r', '--replay'):
            i += 1
            if i >= len(argv):
                sys.exit(f"error: {arg} requires an argument")
            replay = argv[i]
            if not os.path.isdir(replay):
                sys.exit(f"error: '{replay}' is not a valid directory")
        elif arg in ('-c', '--capture'):
            i += 1
            if i >= len(argv):
                sys.exit(f"error: {arg} requires an argument")
            capture = argv[i]
        elif arg.startswith('-'):
            sys.exit(f"error: unknown option: {arg}")
        elif model is None:
            model = arg
        else:
            sys.exit(f"error: unexpected argument: {arg}")
        i += 1

    if model is None:
        sys.exit("error: missing required argument: model")
    if replay and cmd_prefix:
        sys.exit("error: --cmd-prefix cannot be used with --replay")
    if replay and capture:
        sys.exit("error: --replay cannot be used with --capture")

    return model, param, cmd_prefix, replay, capture

def main():
    model, param, cmd_prefix, replay, capture = _parse_args(sys.argv)

    if cmd_prefix or capture:
        host.HOST = host.Remotehost(cmd_prefix, capture)
    elif replay:
        host.HOST = host.Replayhost(replay)
    else:
        host.HOST = host.Localhost()

    if model == 'ietf-interfaces':
        from . import ietf_interfaces
        yang_data = ietf_interfaces.operational(param)
    elif model == 'ietf-routing':
        from . import ietf_routing
        yang_data = ietf_routing.operational()
    elif model == 'ietf-ospf':
        from . import ietf_ospf
        yang_data = ietf_ospf.operational()
    elif model == 'ietf-rip':
        from . import ietf_rip
        yang_data = ietf_rip.operational()
    elif model == 'ietf-hardware':
        from . import ietf_hardware
        yang_data = ietf_hardware.operational()
    elif model == 'infix-containers':
        from . import infix_containers
        yang_data = infix_containers.operational()
    elif model == 'infix-dhcp-server':
        from . import infix_dhcp_server
        yang_data = infix_dhcp_server.operational()
    elif model == 'ietf-system':
        from . import ietf_system
        yang_data = ietf_system.operational()
    elif model == 'ietf-ntp':
        from . import ietf_ntp
        yang_data = ietf_ntp.operational()
    elif model == 'ieee802-dot1ab-lldp':
        from . import infix_lldp
        yang_data = infix_lldp.operational()
    elif model == 'infix-firewall':
        from . import infix_firewall
        yang_data = infix_firewall.operational()
    elif model == 'ietf-bfd-ip-sh':
        from . import ietf_bfd_ip_sh
        yang_data = ietf_bfd_ip_sh.operational()
    else:
        common.LOG.warning("Unsupported model %s", model)
        sys.exit(1)

    print(json.dumps(yang_data, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    main()
