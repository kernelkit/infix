from abc import ABC, abstractmethod
from infamy.neigh import ll6ping
import infamy.iface

def infer_put_dict(name, models):
    if not models.get("ietf-system"):
        models["ietf-system"] = { "system": { "hostname": name} }
    else:
        if not models["ietf-system"].get("system"):
            models["ieft-system"]["system"] = {}
        if not models["ietf-system"]["system"].get("hostname"):
            models["ietf-system"]["system"]["hostname"] = name

class Transport(ABC):
    """Common functions for NETCONF/RESTCONF"""

    @abstractmethod
    def get_data(self, xpath=None, parse=True):
        pass

    @abstractmethod
    def get_config_dict(self, modname):
        pass

    @abstractmethod
    def put_config_dict(self, modname, edit):
        pass

    @abstractmethod
    def get_dict(self, xpath=None):
        pass

    @abstractmethod
    def delete_xpath(self, xpath):
        pass

    @abstractmethod
    def copy(self, source, target):
        pass

    @abstractmethod
    def reboot(self):
        pass

    @abstractmethod
    def call_dict(self, module, call):
        pass

    @abstractmethod
    def call_action(self, xpath):
        pass

    def __getitem__(self, key):
        if key in self.mapping:
            return self.mapping[key]
        return None

    def get_iface(self, name):
        """Fetch target dict for iface and extract param from JSON"""
        content = self.get_data(infamy.iface.get_xpath(name))
        interfaces = content.get("interfaces", {}).get("interface", {})

        # KeyedList does not support `.get()`
        return interfaces[name] if name in interfaces else None

    def get_mgmt_ip(self):
        """Return managment IP address used for RESTCONF/NETCONF"""
        return self.location.host

    def get_mgmt_iface(self):
        """Return managment interface used for RESTCONF/NETCONF"""
        return self.location.interface

    def has_model(self, model_name):
        """Check if the device has the given YANG model loaded."""
        return model_name in self.modules

    def reachable(self):
        """Check if the device reachable on ll6"""
        neigh = ll6ping(self.location.interface, flags=["-w1", "-c1", "-L", "-n"])
        return bool(neigh)

    def test_reset(self):
        self.call_action("/infix-test:test/reset")

    def startup_override(self):
        self.call_action("/infix-test:test/override-startup")
