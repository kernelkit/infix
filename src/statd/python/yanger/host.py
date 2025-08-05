import abc
import datetime
import functools
import json
import os
import subprocess

from . import common


HOST = None


class Host(abc.ABC):
    """Host system API"""

    @abc.abstractmethod
    def now(self):
        """Get the current time as a `datetime`"""
        pass

    @abc.abstractmethod
    def run(self, cmd, default=None, log=True):
        """Get stdout of cmd

        Run cmd, provided as an argv, and return the output it
        produces. On error, return the default value if provided,
        otherwise raise an exception.

        """
        pass

    def run_multiline(self, cmd, default=None):
        """Get lines of stdout of cmd"""
        try:
            txt = self.run(tuple(cmd), log=(default is None))
            return txt.splitlines()
        except:
            if default is not None:
                return default
            raise

    def run_json(self, cmd, default=None):
        """Get JSON object from stdout of cmd"""
        try:
            txt = self.run(tuple(cmd), log=(default is None))
            return json.loads(txt)
        except:
            if default is not None:
                return default
            raise

    @abc.abstractmethod
    def read(self, path):
        """Get the contents of path

        Returns `None` if the file is not readable for any reason.

        """
        pass

    def read_multiline(self, path, default=None):
        """Get lines of content from path

        Returns a list of lines from the file, or default if not readable.
        """
        try:
            txt = self.read(path)
            return txt.splitlines() if txt is not None else default
        except:
            if default is not None:
                return default
            raise

    def read_json(self, path, default=None):
        """Get JSON object from path """
        try:
            txt = self.read(path)
            return json.loads(txt)
        except:
            if default is not None:
                return default
            raise


class Localhost(Host):
    def now(self):
        return datetime.datetime.now(tz=datetime.timezone.utc)

    @functools.cache
    def run(self, cmd, default=None, log=True):
        try:
            result = subprocess.run(cmd, check=True, text=True,
                                    stdin=subprocess.DEVNULL,
                                    stdout=subprocess.PIPE,
                                    stderr=subprocess.DEVNULL)
            return result.stdout
        except subprocess.CalledProcessError as err:
            if default is not None:
                return default

            if log:
                common.LOG.error(f"Failed to run {err}")
            raise

    def read(self, path):
        try:
            with open(path, 'r', encoding='utf-8') as f:
                data = f.read().strip()
                return data
        except FileNotFoundError:
            # This is considered OK
            pass
        except IOError:
            common.LOG.error(f"Failed to read \"{path}\"")

        return None


class Remotehost(Localhost):
    def __init__(self, prefix, capdir):
        super().__init__()
        self.prefix = tuple(prefix.split()) if prefix else tuple()
        self.capdir = capdir
        if capdir:
            for subdir in ("rootfs", "run"):
                os.makedirs(os.path.join(capdir, subdir), exist_ok=True)

    def now(self):
        timestamp = self._run(["date", "-u", "+%s"], default=None, log=True)
        if self.capdir:
            path = os.path.join(self.capdir, "timestamp")
            with open(path, "w", encoding='utf-8') as f:
                f.write(f"{timestamp}\n")
            pass

        return datetime.datetime.fromtimestamp(int(timestamp), datetime.timezone.utc)

    def _run(self, cmd, default, log):
        # Assume that the wrapper acts like ssh(1) and simply concats
        # arguments to a single string. Therefore, we must quoute
        # arguments containing spaces so that commands like `vtysh -c
        # "show ip route json"` work as expected.
        cmd = " ".join([arg if " " not in arg else f"\"{arg}\"" for arg in cmd])
        return super().run(self.prefix + (cmd,), default, log)

    def run(self, cmd, default=None, log=True):
        if not self.capdir:
            return self._run(cmd, default, log)

        storedpath = os.path.join(self.capdir, "run", Replayhost.SlugOf(cmd))
        if os.path.exists(storedpath):
            with open(storedpath, 'r', encoding='utf-8') as f:
                return f.read()

        out = self._run(cmd, default, log)
        with open(storedpath, "w", encoding='utf-8') as f:
            f.write(out)

        return out

    def exists(self, path):
        if not self._run(("ls", path), default="", log=False):
            return False

        if self.capdir:
            dirname = os.path.join(self.capdir, "rootfs", os.path.dirname(path[1:]))
            filname = os.path.join(self.capdir, "rootfs", path[1:])
            os.makedirs(dirname, exist_ok=True)
            open(filname, "w", encoding='utf-8').close()  # Create empty file

        return True

    def read(self, path):
        out = self._run(("cat", path), default="", log=False)

        if self.capdir:
            dirname = os.path.join(self.capdir, "rootfs", os.path.dirname(path[1:]))
            filname = os.path.join(self.capdir, "rootfs", path[1:])
            os.makedirs(dirname, exist_ok=True)
            with open(filname, "w", encoding='utf-8') as f:
                f.write(out)

        return out


class Replayhost(Host):
    def SlugOf(cmd):
        return "_".join(cmd).replace("/", "+").replace(" ", "-")

    def __init__(self, replaydir):
        self.replaydir = replaydir

    def now(self):
        path = os.path.join(self.replaydir, "timestamp")
        with open(path, 'r', encoding='utf-8') as f:
            timestamp = f.read().strip()
            return datetime.datetime.fromtimestamp(int(timestamp), datetime.timezone.utc)

    def run(self, cmd, default=None, log=True):
        path = os.path.join(self.replaydir, "run", Replayhost.SlugOf(cmd))

        try:
            with open(path, 'r', encoding='utf-8') as f:
                return f.read()
        except:
            if default is not None:
                return default

            if log:
                common.LOG.error(f"No recording found for run \"{path}\"")
            raise

    def exists(self, path):
        path = os.path.join(self.replaydir, "rootfs", path[1:])
        return os.path.isfile(path)

    def read(self, path):
        path = os.path.join(self.replaydir, "rootfs", path[1:])
        try:
            with open(path, 'r', encoding='utf-8') as f:
                return f.read()
        except FileNotFoundError:
            # This is considered OK
            pass
        except:
            common.LOG.error(f"No recording found for file \"{path}\"")
            raise
