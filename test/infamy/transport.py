from abc import ABC, abstractmethod
from infamy.neigh import ll6ping

class Transport(ABC):
    """Common functions for NETCONF/RESTCONF"""

    @abstractmethod
    def get_data(self, xpath=None, as_xml=False):
        pass
    @abstractmethod
    def get_config_dict(self, modname):
        pass
    @abstractmethod
    def put_config_dict(self, modname, edit):
        pass
    @abstractmethod
    def get_dict(self, xpath=None, as_xml=False):
        pass
    @abstractmethod
    def get_xpath(self,  xpath, key, value, path=None):
        pass
    @abstractmethod
    def get_iface(self, iface):
        pass
    @abstractmethod
    def get_iface_xpath(self, iface, path=None):
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

    def get_mgmt_ip(self):
        """Return managment IP address used for RESTCONF"""
        return self.location.host

    def get_mgmt_iface(self):
        """Return managment interface used for RESTCONF"""
        return self.location.interface

    def address(self):
        """Return managment IP address used for RESTCONF"""
        return self.location.host

    def reachable(self):
        """Check if the device reachable on ll6"""
        neigh = ll6ping(self.location.interface, flags=["-w1", "-c1", "-L", "-n"])
        return bool(neigh)
