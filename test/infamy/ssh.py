import subprocess

from dataclasses import dataclass
from . import env


@dataclass
class Location:
    host: str
    username: str
    password: str
    port: int = 22


import subprocess
import os

def fetch_file(remote_user, remote_address, remote_file, local_file, key_file, check=False, remove=False):
    """
    Fetches a file over SSH using scp and the provided private key.

    :param remote_user: The user on the remote machine.
    :param remote_address: The address of the remote machine.
    :param remote_file: The file to fetch from the remote machine.
    :param local_file: The local path where the file will be stored.
    :param key_file: The path to the private SSH key.
    :param check: check the return code of the command.
    :param remove: remove the fetched local file after copying.
    """
    try:
        result = subprocess.run(
            f"scp -q -o StrictHostKeyChecking=no -i {key_file} {remote_user}@[{remote_address}]:{remote_file} {local_file}",
            shell=True
        )

        if check:
            if result.returncode != 0:
                raise RuntimeError("Failed to copy file from remote host")

            if not os.path.exists(local_file):
                raise RuntimeError(f"File {local_file} does not exist after copy")

            if os.path.getsize(local_file) == 0:
                raise RuntimeError(f"File {local_file} is empty after copy")

    except Exception as e:
        print(f"Error during file transfer: {e}")
        raise

    finally:
        if os.path.exists(key_file):
            try:
                os.remove(key_file)
            except OSError as e:
                print(f"Error removing key file {key_file}: {e}")

    if remove:
        try:
            if os.path.exists(local_file):
                os.remove(local_file)
        except OSError as e:
            print(f"Error removing fetched file {local_file}: {e}")

class Device(object):
    def __init__(self, name: str, location: Location):
        self.name = name
        self.location = location
    def __str__(self):
        nm = f"{self.name}"
        if env.ENV.ltop:
            nm += f"({env.ENV.ltop.xlate(self.name)})"
        return nm + " [SSH]"

    def _mangle_subprocess_args(self, args, kwargs):
        if not args:
            return None

        args = list(args)
        if type(args[0]) is str:
            if kwargs.get("shell"):
                args[0] = ["/bin/sh", "-c", args[0]]
                kwargs["shell"] = False
            else:
                args[0] = [args[0]]

        args[0] = ["ssh",
                   "-oStrictHostKeyChecking no",
                   "-oUserKnownHostsFile /dev/null",
                   "-oLogLevel QUIET",
                   f"-l{self.location.username}",
                   self.location.host] + args[0]

        if self.location.password:
            args[0] = ["sshpass", f"-p{self.location.password}"] + args[0]

        return args, kwargs

    def run(self, *args, **kwargs):
        args, kwargs = self._mangle_subprocess_args(args, kwargs)
        return subprocess.run(*args, **kwargs)

    def runsh(self, script, *args, **kwargs):
        return self.run("/bin/sh", text=True, input=script,
                        stdout=subprocess.PIPE,
                        stderr=subprocess.STDOUT, *args, **kwargs)
