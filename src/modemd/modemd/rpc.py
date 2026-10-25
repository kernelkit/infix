import argparse
import subprocess
import sys


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

    xpath = "/ietf-hardware:hardware/component[name='modem%d']/infix-hardware:%s" % (index, args.rpc)
    if subprocess.run(["rpc", xpath]).returncode:
        sys.stderr.write("Error: unable to send '%s' rpc\n" % args.rpc)
        sys.exit(1)


if __name__ == "__main__":
    main()
