import argparse
import json
import sys
import os


def sendrpc(index, rpc):
    path = "/run/modemd/modem%d/rpc" % index
    if os.path.exists(path):
        sys.stderr.write("Error: another rpc is running\n")
        return

    with open(path, "w") as fd:
        json.dump(rpc, fd)

    if not os.system(['sysrepocfg', '-f', 'json', '-R', path]):
        sys.stderr.write("Error: unable to restart\n")

    os.unlink(path)


def main():
    parser = argparse.ArgumentParser(prog='modem-rpc')
    parser.add_argument("-i", "--index", default=0, help="Modem index")
    parser.add_argument("-r", "--rpc", default=0, help="RPC command")
    args = parser.parse_args()

    if args.index:
        index = int(args.index)
    else:
        index = 0

    if not args.rpc:
        sys.stderr.write("Error: no rpc command\n")
        sys.exit(1)

    print("Sending '%s' rpc to modem%d" % (args.rpc, index))

    rpc = {
        "ietf-hardware:hardware": {
            "component": [{
                "name": "modem%d" % index,
                "infix-hardware:modem": {args.rpc: {}}
            }]
        }
    }

    sendrpc(index, rpc)


if __name__ == "__main__":
    main()
