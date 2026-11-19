"""RESTCONF notifications as server-sent events, RFC 8650"""
import json
import queue
import threading
import time
from urllib.parse import urlparse

from infamy.restconf import requests_workaround


def subscribe(target, params):
    """Establish a subscription and return the stream URI of the reply"""
    output = target.rpc_output("ietf-subscribed-notifications",
                               "establish-subscription", params)
    return output["ietf-restconf-subscribed-notifications:uri"]


class Stream:
    """An open event stream, its notifications are queued as dicts"""

    def __init__(self, target, uri):
        self.events = queue.Queue()
        self.closed = threading.Event()
        self.reply = requests_workaround("GET", f"{target.url_base}{urlparse(uri).path}",
                                         None, {"Accept": "text/event-stream"},
                                         target.auth, stream=True)
        threading.Thread(target=self._read, daemon=True).start()

    def _read(self):
        data = []
        try:
            for line in self.reply.iter_lines(decode_unicode=True):
                if line.startswith("data:"):
                    data.append(line[5:])
                elif not line and data:
                    event = json.loads("\n".join(data))
                    print(json.dumps(event))
                    self.events.put(event["ietf-restconf:notification"])
                    data = []
        except Exception as err:
            print(f"Stream read failed: {err}")
        self.closed.set()

    def wait_for(self, match, timeout):
        """Return the first queued notification match() accepts, or None"""
        end = time.time() + timeout
        while True:
            try:
                event = self.events.get(timeout=max(end - time.time(), 0.01))
            except queue.Empty:
                return None
            if match(event):
                return event
            if time.time() >= end:
                return None
