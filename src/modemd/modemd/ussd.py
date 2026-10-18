import argparse
import subprocess
import sys

verbose = False


def runcmd(cmd):
    if verbose:
        print("Running: %s" % " ".join(cmd))

    ret = None
    try:
        res = subprocess.run(cmd, capture_output=True, text=True)
        if verbose:
            print(res.stdout)
        if res.returncode == 0:
            if res.stdout:
                ret = res.stdout.strip()
            else:
                ret = True
    except subprocess.CalledProcessError:
        return None
    finally:
        return ret


def ussd(index, code):
    cmd = ['/sbin/modem-command',
           '--index', str(index),
           '--expect', '+CUSD',
           '--timeout', '60',
           'AT+CUSD=1,"%s",15' % code]

    if verbose:
        cmd.append('--verbose')

    output = runcmd(cmd)
    if output is None:
        sys.stderr.write("ERROR: Command failed\n")
        return False

    resp = output.split('"')
    if len(resp) == 3:
        print(resp[1])

    return True


def main():
    global verbose
    parser = argparse.ArgumentParser(prog='modem-ussd')
    parser.add_argument("-i", "--index", default=0, help="Modem index")
    parser.add_argument("-c", "--code", default=None, help="USSD codec")
    parser.add_argument("-v", "--verbose", action="store_true")
    args = parser.parse_args()

    if args.verbose:
        verbose = True

    index = 0
    if args.index:
        index = int(args.index)

    if not args.code:
        sys.stderr.write("Error: need USSD code\n")
    else:
        if ussd(index, args.code):
            sys.exit(0)

    sys.exit(1)


if __name__ == "__main__":
    main()
