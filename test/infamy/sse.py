"""RESTCONF notifications as server-sent events, RFC 8650"""
import json
import threading
from urllib.parse import urlparse

from infamy.restconf import requests_workaround
from infamy.transport import Events


def subscribe(target, params):
    """Establish a subscription and return the stream URI of the reply"""
    output = target.rpc_output("ietf-subscribed-notifications",
                               "establish-subscription", params)
    return output["ietf-restconf-subscribed-notifications:uri"]


class Stream(Events):
    """An open event stream"""

    def __init__(self, target, uri):
        super().__init__()
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
