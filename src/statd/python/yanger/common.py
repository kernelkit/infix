import syslog
from datetime import timedelta

from . import host


class SysLog:
    """Lightweight syslog wrapper replacing the logging module.

    Provides the same .error()/.warning()/.info()/.debug() interface
    used throughout yanger, but uses the C syslog facility directly,
    avoiding the ~374ms import overhead of logging + logging.handlers.
    """

    DEBUG = syslog.LOG_DEBUG
    INFO = syslog.LOG_INFO
    WARNING = syslog.LOG_WARNING
    ERROR = syslog.LOG_ERR

    def __init__(self, name):
        syslog.openlog(name, syslog.LOG_PID)
        self._level = self.INFO

    def setLevel(self, level):
        self._level = level

    def _log(self, level, msg, *args):
        if level > self._level:
            return
        if args:
            msg = msg % args
        syslog.syslog(level, msg)

    def debug(self, msg, *args):
        self._log(self.DEBUG, msg, *args)

    def info(self, msg, *args):
        self._log(self.INFO, msg, *args)

    def warning(self, msg, *args):
        self._log(self.WARNING, msg, *args)

    def error(self, msg, *args):
        self._log(self.ERROR, msg, *args)


LOG = SysLog("yanger")

class YangDate:
    def __init__(self, dt=None):
        self.dt = dt if dt else host.HOST.now()

    def __str__(self):
        datestr = self.dt.strftime("%Y-%m-%dT%H:%M:%S%z")

        # Translate strftime's timezone format (HHMM) to the one
        # expected by YANG (HH:MM)
        return datestr[:-2] + ':' + datestr[-2:]

    @classmethod
    def from_delta(cls, delta):
        return cls(host.HOST.now() - delta)

    @classmethod
    def from_seconds(cls, seconds):
        return cls.from_delta(timedelta(seconds=seconds))


def insert(obj, *path_and_value):
    """"This function inserts a value into a nested json object"""
    if len(path_and_value) < 2:
        raise ValueError("Error: insert() takes at least two args")

    *path, value = path_and_value

    curr = obj
    for key in path[:-1]:
        if key not in curr or not isinstance(curr[key], dict):
            curr[key] = {}
        curr = curr[key]

    curr[path[-1]] = value
