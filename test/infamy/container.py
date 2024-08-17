"""Manage Infix containers"""
import time
from infamy.util import warn


class Container:
    """Helper methods"""
    IMAGE = "curios-httpd-v24.05.0.tar.gz"

    def __init__(self, target):
        self.system = target

    def _find(self, name):
        oper = self.system.get_data("/infix-containers:containers")
        if not oper:
            return None

        for container in oper["containers"]["container"]:
            if container["name"] == name:
                return container
        return None

    def exists(self, name):
        """Check if container {name} runs on target."""
        container = self._find(name)
        if not container:
            return False
        return True

    def running(self, name):
        """Check if container {name} exists and is running."""
        container = self._find(name)
        if container and container["running"]:
            return True
        return False

    def action(self, name, act):
        """Call container action (context RPC), retry three times."""
        xpath = f"/infix-containers:containers/container[name='{name}']/{act}"

        for attempt in range(3):
            try:
                return self.system.call_action(xpath)
            except Exception as e:
                warn(f"failed {act} {name} ({attempt + 1}/3): {e}")
                time.sleep(1)
                if attempt == 2:
                    raise
