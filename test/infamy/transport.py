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
    def get_factory(self, xpath=None):
        """Get factory-default datastore contents matching xpath"""
        pass

    @abstractmethod
    def patch_config(self, modname, edit):
        pass

    @abstractmethod
    def get_dict(self, xpath=None):
        pass

    @abstractmethod
    def delete_xpath(self, xpath):
        pass

    @abstractmethod
    def delete_xpaths(self, xpaths):
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
    def call_action(self, xpath, input_data=None):
        """Invoke a YANG action at `xpath`.

        `input_data`, if supplied, is a dict of input leaves (e.g.
        ``{"state": "on"}``).  Backends wrap it in the protocol-specific
        envelope; actions that take no input may omit the argument.
        """
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

    def has_feature(self, model_name, feature_name):
        """Check if a specific feature is enabled on the device for a given YANG model."""
        if model_name not in self.modules:
            return False
        features = self.modules[model_name].get("feature", [])
        return feature_name in features

    def reachable(self):
        """Check if the device reachable on ll6"""
        neigh = ll6ping(self.location.interface, flags=["-w1", "-c1", "-L", "-n"])
        return bool(neigh)

    def test_reset(self):
        self.call_action("/infix-test:test/reset")

    def startup_override(self):
        self.call_action("/infix-test:test/override-startup")

    def log(self, message, severity=None, facility=None, app_name=None,
            msgid=None, sd=None):
        """Log a message on the target, using the infix-syslog:log RPC.

        Defaults to user.notice with app-name set to the calling user.
        `facility` is a plain name, e.g. "daemon", the module prefix is
        added here.  `sd` is RFC 5424 structured data, given as a dict
        of dicts: {"sd-id": {"name": "value", ...}}.
        """
        rpc = {"message": message}
        if severity:
            rpc["severity"] = severity
        if facility:
            module = "infix-syslog" if facility in ("rauc", "container", "web") else "ietf-syslog"
            rpc["facility"] = f"{module}:{facility}"
        if app_name:
            rpc["app-name"] = app_name
        if msgid:
            rpc["msgid"] = msgid
        if sd:
            rpc["structured-data"] = [{
                "id": sdid,
                "param": [{"name": name, "value": value} for name, value in params.items()]
            } for sdid, params in sd.items()]

        return self.call_dict("infix-syslog", {"log": rpc})
