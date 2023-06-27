
from collections import namedtuple
from dataclasses import dataclass

import logging
import socket
import sys
import time

import libyang
import lxml
import netconf_client.connect
import netconf_client.ncclient
from netconf_client.error import RpcError

modinfo_fields = ("identifier", "version", "format", "namespace")
ModInfoTuple = namedtuple("ModInfoTuple", modinfo_fields)
class ModInfo(ModInfoTuple):
    def xmlns(self):
        return f"xmlns:{self.identifier}=\"{self.namespace}\""

NS = {
    "ietf-netconf-monitoring": "urn:ietf:params:xml:ns:yang:ietf-netconf-monitoring",
}

class Manager(netconf_client.ncclient.Manager):
    """Wrapper for the real manager

    Just ensures that we can enable debugging without issues when
    operating on an IPv6 socket.

    """
    def _fetch_connection_ip(self):
        """Retrieves and stores the connection's local and remote IP"""
        self._local_ip = None
        self._peer_ip = None
        try:
            self._local_ip = self.session.sock.sock.getsockname()[0]
            self._peer_ip  = self.session.sock.sock.getpeername()[0]
        except (AttributeError, socket_error):
            pass

    def _debug(self):
        self.set_logger_level(logging.DEBUG)
        self.logger().addHandler(logging.StreamHandler(sys.stderr))

@dataclass
class Location:
    host: str
    port: int = 830
    username: str = "admin"
    password: str = "admin"

class Device(object):
    def __init__(self,
                 location: Location,
                 mapping: dict,
                 yangdir: None | str = None):

        self.mapping = mapping
        self.ly = libyang.Context(yangdir)

        self._ncc_init(location)
        self._ly_init(yangdir)
        # self.update_schema()
        self.ncc.dispatch('<factory-default xmlns="urn:infix:factory-default:ns:yang:1.0"/>')

    def _ncc_init(self, location):
        ai = socket.getaddrinfo(location.host, location.port,
                                0, 0, socket.SOL_TCP)
        sock = socket.socket(ai[0][0], ai[0][1], 0)
        sock.settimeout(60)
        print(f"Connecting to mgmt IP {location.host}:{location.port} ...")
        sock.connect(ai[0][4])
        sock.settimeout(None)

        session = netconf_client.connect.connect_ssh(sock=sock,
                                                     username=location.username,
                                                     password=location.password)
        self.ncc = Manager(session)

    def _ly_init(self, yangdir):
        self.ly = libyang.Context(yangdir)

        lib = self.ly.load_module("ietf-yang-library")
        ns = libyang.util.c2str(lib.cdata.ns)

        xml = lxml.etree.tostring(self.ncc.get(filter=f"""
        	<filter type="subtree">
        		<modules-state xmlns="{ns}" />
        	</filter>""").data_ele[0])

        data = self.ly.parse_data("xml", libyang.IOType.MEMORY,
                                  xml, parse_only=True).print_dict()

        self.modules = { m["name"] : m for m in data["modules-state"]["module"] }

        for ms in self.modules.values():
            if ms["conformance-type"] != "implement":
                continue

            mod = self.ly.load_module(ms["name"])

            # TODO: ms["feature"] contains the list of enabled
            # features, so ideally we should only enable the supported
            # ones. However, features can depend on each other, so the
            # naïve looping approach doesn't work.
            mod.feature_enable_all()

    def _modules_in_xpath(self, xpath):
        modnames = []

        # Find all referenced models
        for seg in xpath.split("/"):
            if ":" in seg:
                modname, node = seg.split(":")
                modnames.append(modname)

        return list(filter(lambda m: m["name"] in modnames,
                           self.modules.values()))

    def _get(self, xpath, getter):
        # Figure out which modules we are referencing
        mods = self._modules_in_xpath(xpath)

        # Fetch the data
        xmlns = " ".join([f"xmlns:{m['name']}=\"{m['namespace']}\"" for m in mods])
        filt = f"<filter type=\"xpath\" select=\"{xpath}\" {xmlns} />"
        cfg = lxml.etree.tostring(getter(filter=filt).data_ele[0])

        return self.ly.parse_data_mem(cfg, "xml", parse_only=True)

    def get(self, xpath):
        return self._get(xpath, self.ncc.get)

    def get_dict(self, xpath):
        return self.get(xpath).print_dict()

    def get_config(self, xpath):
        return self._get(xpath, self.ncc.get_config)

    def get_config_dict(self, xpath):
        return self.get_config(xpath).print_dict()

    def put_config(self, edit):
        xml = "<config xmlns=\"urn:ietf:params:xml:ns:netconf:base:1.0\">" + edit + "</config>"
        for _ in range(0,3):
            try:
                self.ncc.edit_config(xml, default_operation='merge')
            except RpcError as _e:
                print(f"Failed sending edit-config RPC: {_e}  Retrying ...")
                time.sleep(1)
                continue
            break

    def put_config_dict(self, modname, edit):
        mod = self.ly.get_module(modname)
        lyd = mod.parse_data_dict(edit, no_state=True)
        return self.put_config(lyd.print_mem("xml", with_siblings=True, pretty=False))

    def call(self, call):
        return self.ncc.dispatch(call)

    def call_dict(self, modname, call):
        mod = self.ly.get_module(modname)
        lyd = mod.parse_data_dict(call, rpc=True)
        return self.call(lyd.print_mem("xml", with_siblings=True, pretty=False))

    def call_action(self, action):
        xml = "<action xmlns=\"urn:ietf:params:xml:ns:yang:1\">" + action + "</action>"
        return self.ncc.dispatch(xml)

    def call_action_dict(self, modname, action):
        mod = self.ly.get_module(modname)
        lyd = mod.parse_data_dict(action, rpc=True)
        return self.call_action(lyd.print_mem("xml", with_siblings=True, pretty=False))

