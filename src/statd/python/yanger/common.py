from datetime import timedelta

from . import host

LOG = None

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
