import abc
import datetime
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
            txt = self.run(cmd, log=(default is None))
            return txt.splitlines()
        except:
            if default is not None:
                return default
            raise

    def run_json(self, cmd, default=None):
        """Get JSON object from stdout of cmd"""
        try:
            txt = self.run(cmd, log=(default is None))
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
            with open(path, 'r') as f:
                data = f.read().strip()
                return data
        except FileNotFoundError:
            # This is considered OK
            pass
        except IOError:
            common.LOG.error(f"Failed to read \"{path}\"")

        return None

class Testhost(Host):
    def __init__(self, basedir):
        self.basedir = basedir

    def now(self):
        return datetime.datetime(2023, 1, 1, 12, 0, 0, tzinfo=datetime.timezone.utc)

    def run(self, cmd, default=None, log=True):
        slug = "_".join(cmd).replace("/", "+").replace(" ", "-")
        path = os.path.join(self.basedir, "run", slug)

        try:
            with open(path, 'r') as f:
                return f.read()
        except:
            if default is not None:
                return default

            if log:
                common.LOG.error(f"No recording found for run \"{path}\"")
            raise

    def read(self, path):
        path = os.path.join(self.basedir, "rootfs", path[1:])
        try:
            with open(path, 'r') as f:
                return f.read()
        except FileNotFoundError:
            # This is considered OK
            pass
        except:
            common.LOG.error(f"No recording found for file \"{path}\"")
            raise
