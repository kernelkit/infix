import re
import subprocess


def ll6ping(ifname, flags=["-w60", "-c1", "-L", "-n"]):
    argv = ["ping"] + flags + ["ff02::1%{}".format(ifname)]

    try:
        ping = subprocess.run(argv,
                              stdout=subprocess.PIPE,
                              stderr=subprocess.DEVNULL,
                              text=True, check=True)
    except subprocess.CalledProcessError:
        return None

    m = re.search(r"^\d+ bytes from ([:0-9a-f]+%\S+):",
                  ping.stdout, re.MULTILINE)
    return m.group(1) if m else None
