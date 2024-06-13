
from collections import namedtuple
from dataclasses import dataclass

import logging
import socket
import sys
import time
import uuid   # For _ncc_get_data() extension
import os
import copy
import re

import libyang
import lxml
import netconf_client.connect
import netconf_client.ncclient
from infamy.transport import Transport
from netconf_client.error import RpcError


def netconf_syn(addr):
    try:
        ai = socket.getaddrinfo(addr, 830, 0, 0, socket.SOL_TCP)
        sock = socket.socket(ai[0][0], ai[0][1], 0)
        sock.connect(ai[0][4])
        sock.close()
        print(f"{addr} answers to TCP connections on port 830 (NETCONF)")
        return True
    except Exception:
        return False


modinfo_fields = ("identifier", "version", "format", "namespace")
ModInfoTuple = namedtuple("ModInfoTuple", modinfo_fields)


class ModInfo(ModInfoTuple):
    def xmlns(self):
        return f"xmlns:{self.identifier}=\"{self.namespace}\""


NS = {
    "ietf-netconf-monitoring": "urn:ietf:params:xml:ns:yang:ietf-netconf-monitoring",
    "nc": "urn:ietf:params:xml:ns:netconf:base:1.0",
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
            self._peer_ip = self.session.sock.sock.getpeername()[0]
            print(f"Connection status\n\tLocal IP: {self._local_ip}\n"
                  "\tPeer IP: {self._peer_ip}")
        except (AttributeError, socket.error) as err:
            print(f"Failed connecting, status: {err}")
            pass

    def _debug(self):
        self.set_logger_level(logging.DEBUG)
        self.logger().addHandler(logging.StreamHandler(sys.stderr))

class NccGetDataReply:
    """Fold in to DataReply class when upstreaming"""
    def __init__(self, raw, ele):
        self.data_ele = ele.find("{urn:ietf:params:xml:ns:yang:ietf-netconf-nmda}data")
        self.data_xml = lxml.etree.tostring(self.data_ele)
        self.raw_reply = raw

class NccGetSchemaReply:
    def __init__(self, raw):
        self.ele = lxml.etree.fromstring(raw.xml.decode())
        self.ele = self.ele.find("{urn:ietf:params:xml:ns:yang:ietf-netconf-monitoring}data")
        self.schema = self.ele.text

@dataclass
class Location:
    interface: str
    host: str
    password: str
    username: str = "admin"
    port: int = 830

class Device(Transport):
    def __init__(self,
                 location: Location,
                 mapping: dict,
                 yangdir: None | str = None,
                 factory_default=True):

        self.location = location
        self.mapping = mapping
        self.location = location
        self.ly = libyang.Context(yangdir)
        self._ncc_init(location)
        # self.ncc._fetch_connection_ip()
        # self.ncc._debug()

        self.modules = {}
        self._ly_bootstrap(yangdir)

        del self.ly
        self.ly = libyang.Context(yangdir)
        self._ly_init(yangdir)
        if factory_default:
            try:
                self.ncc.dispatch('<factory-default xmlns="urn:infix:factory-default:ns:yang:1.0"/>')
            except RpcError as err:
                print(f"Failed sending factory-default RPC: {err}")
                raise err

    def _ncc_init(self, location):
        ai = socket.getaddrinfo(location.host, location.port,
                                0, 0, socket.SOL_TCP)
        sock = socket.socket(ai[0][0], ai[0][1], 0)
        sock.settimeout(60)
        print(f"Connecting to mgmt IP {location.host}:{location.port} ...")
        try:
            sock.connect(ai[0][4])
        except InterruptedError as err:
            print(f"Connection interrupted: {err}")
            raise err
        except TimeoutError as err:
            print(f"Connection timeout: {err}")
            raise err
        sock.settimeout(None)

        session = netconf_client.connect.connect_ssh(sock=sock,
                                                     username=location.username,
                                                     password=location.password)
        self.ncc = Manager(session)

    def _ly_bootstrap(self, yangdir):
        self.modules["ietf-netconf-monitoring"] = {
            "name": "ietf-netconf-monitoring"
        }

        for val in self.modules.values():
            mod = self.ly.load_module(val["name"])
            mod.feature_enable_all()
        schemas = self.get_schemas_list()
        for schema in schemas:
            if os.path.exists(yangdir + "/" + schema['filename']) is False:
                self.get_schema(schema, yangdir)
                sys.stdout.write("Downloading YANG model "
                                 f"{schema['identifier']} ...\r\033[K")
        print("YANG models downloaded.")

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

        self.modules = {m["name"]: m for m in data["modules-state"]["module"]}

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
                modname, _ = seg.split(":")
                modnames.append(modname)

        return list(filter(lambda m: m["name"] in modnames,
                           self.modules.values()))

    def _ncc_make_rpc(self, guts, msg_id=None):
        if not msg_id:
            msg_id = uuid.uuid4()

        return '<rpc message-id="{id}" xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">{guts}</rpc>' \
            .format(guts=guts, id=msg_id).encode("utf-8")

    def _ncc_get_data_rpc(self, datastore="operational", filter=None, msg_id=None):
        pieces = []
        pieces.append('<get-data xmlns="urn:ietf:params:xml:ns:yang:ietf-netconf-nmda">')
        pieces.append(f'<datastore xmlns:ds="urn:ietf:params:xml:ns:yang:ietf-datastores">'
                      f'ds:{datastore}'
                      f'</datastore>')
        if filter:
            xmlns = " ".join([f"xmlns:{m['name']}=\"{m['namespace']}\""
                              for m in self._modules_in_xpath(filter)])
            pieces.append(f'<xpath-filter {xmlns}>{filter}</xpath-filter>')
        pieces.append("</get-data>")
        return self._ncc_make_rpc("".join(pieces), msg_id=msg_id)

    def copy(self, source, target):
        cmd = f'''<copy-config>
        <target>
            <{target}/>
        </target>
        <source>
           <{source}/>
        </source>
        </copy-config>'''
        self.ncc._send_rpc(self._ncc_make_rpc(cmd))

    def reboot(self):
        """<system-restart xmlns="urn:ietf:params:xml:ns:yang:ietf-system"/>"""
        return self.call_dict("ietf-system", {
            "system-restart": {}
        })

    def _get(self, xpath, getter, parse=True):
        # Figure out which modules we are referencing
        mods = self._modules_in_xpath(xpath)

        # Fetch the data
        xmlns = " ".join([f"xmlns:{m['name']}=\"{m['namespace']}\""
                          for m in mods])
        filt = f"<filter type=\"xpath\" select=\"{xpath}\" {xmlns} />"
        # pylint: disable=c-extension-no-member
        data = getter(filter=filt).data_ele[0]
        if parse==False:
            return data
        cfg = lxml.etree.tostring(data)
        return self.ly.parse_data_mem(cfg, "xml", parse_only=True)

    def _get_data(self, xpath, parse=True):
        """Local member wrapper for netconf-client <get-data> RPC"""
        # pylint: disable=protected-access
        (raw, ele) = self.ncc._send_rpc(self._ncc_get_data_rpc(filter=xpath))
        data = NccGetDataReply(raw, ele)
        if(parse==False):
            return data

        if len(data.data_ele) == 0:
            return None

        # pylint: disable=c-extension-no-member
        cfg = lxml.etree.tostring(data.data_ele[0])
        return self.ly.parse_data_mem(cfg, "xml", parse_only=True)

    def get(self, xpath, parse=True):
        """RPC <get> (legacy NETCONF) fetches config:false data"""
        return self._get(xpath, self.ncc.get, parse)

    def get_dict(self, xpath):
        """Return Python dictionary of <get> RPC data"""

        data = self.get(xpath)
        if not data:
            return None

        return data.print_dict()

    def get_data(self, xpath=None, parse=True):
        """RPC <get-data> to fetch operational data"""

        if parse==False:
            return self._get_data(xpath,parse)

        data = self._get_data(xpath)
        if not data:
            return None

        return data.print_dict()

    def get_config(self, xpath):
        """Get NETCONF XML configuration for a given XPath"""
        return self._get(xpath, self.ncc.get_config)

    def get_config_dict(self, xpath):
        """Get Python dictionary version of XML configuration"""
        return self.get_config(xpath).print_dict()

    def put_config(self, edit):
        """Send XML configuration over NETCONF"""
        yang2nc = {
            "none": None,
            "delete": "delete",
        }

        xml = f"<config xmlns=\"{NS['nc']}\" xmlns:nc=\"{NS['nc']}\">" \
            + edit + "</config>"

        # Translate any edit operations from the yang format generated
        # by diffing trees with libyang, to their NETCONF equivalents.
        for src, dst in yang2nc.items():
            xml = xml.replace(f"yang:operation=\"{src}\"",
                              f"nc:operation=\"{dst}\"" if dst else "")

        for _ in range(0, 3):
            try:
                self.ncc.edit_config(xml, default_operation='merge')
            except RpcError as _e:
                print(f"Failed sending edit-config RPC: {_e}  Retrying ...")
                time.sleep(1)
                continue
            break

    def put_config_dict(self, modname, edit):
        """Convert Python dictionary to XMl and send as configuration"""
        mod = self.ly.get_module(modname)
        lyd = mod.parse_data_dict(edit, no_state=True, validate=False)
        return self.put_config(lyd.print_mem("xml", with_siblings=True, pretty=False))

    def call(self, call):
        """Call RPC, XML version"""
        return self.ncc.dispatch(call)

    def call_dict(self, modname, call):
        """Call RPC, Python dictionary version"""
        mod = self.ly.get_module(modname)
        lyd = mod.parse_data_dict(call, rpc=True)
        return self.call(lyd.print_mem("xml", with_siblings=True, pretty=False))

    def call_action(self, action):
        """Call NETCONF action (contextualized RPC), XML version"""
        xml = "<action xmlns=\"urn:ietf:params:xml:ns:yang:1\">" \
            + action + "</action>"
        return self.ncc.dispatch(xml)

    def call_action_dict(self, modname, action):
        """Call NETCONF action (contextualized RPC), Python dictionary version"""
        mod = self.ly.get_module(modname)
        lyd = mod.parse_data_dict(action, rpc=True)
        return self.call_action(lyd.print_mem("xml", with_siblings=True, pretty=False))

    def get_schemas_list(self):
        schemas = []

        data = self.get_dict("/netconf-state")
        for d in data["netconf-state"]["schemas"]["schema"]:
            schema = {}
            schema["identifier"] = d['identifier']
            schema["format"] = d["format"]
            if d['version']:
                schema["version"] = d['version']
                schema["filename"] = f"{d['identifier']}@{d['version']}.yang"
            else:
                schema["filename"] = f"{d['identifier']}.yang"
            schemas.append(schema)
        return schemas

    def get_schema(self, schema, outdir):
        query = {
            "get-schema": {
                "identifier": schema['identifier'],
                "version": schema['version'],
                "format": "yang"
            }
        }
        rpc_reply = self.call_dict("ietf-netconf-monitoring",  query)
        data = NccGetSchemaReply(rpc_reply)

        with open(outdir+"/"+schema["filename"], "w") as f:
            f.write(data.schema)

    def get_xpath(self, xpath, key, value, path=None):
        """Compose complete XPath to a YANG node in /ietf-interfaces"""
        xpath = f"{xpath}[{key}='{value}']"
        if not path is None:
            xpath=f"{xpath}/{path}"
        return xpath

    def get_iface_xpath(self, iface, path=None):
        """Compose complete XPath to a YANG node in /ietf-interfaces"""
        xpath = f"/ietf-interfaces:interfaces/interface"
        return self.get_xpath(xpath, "name", iface, path)

    def get_iface(self, iface):
        """Fetch target dict for iface and extract param from JSON"""
        content = self.get_data(self.get_iface_xpath(iface))
        interface=content.get("interfaces", {}).get("interface", None)

        if interface is None:
            return None

        # This really not required, it is due a bug in rousette.
        return interface[iface]

    def delete_xpath(self, xpath):
        # Split out the model and the container from xpath
        pattern = r"^/(?P<module>[^:]+):(?P<path>[^/]+)"
        match = re.search(pattern, xpath)
        module = match.group('module')
        modpath = f"/{match.group('module')}:{match.group('path')}"

        old = self.get_config_dict(modpath)
        new = copy.deepcopy(old)
        libyang.xpath_del(new, xpath)
        mod = self.ly.get_module(module)
        oldd = mod.parse_data_dict(old, no_state=True)
        newd = mod.parse_data_dict(new, no_state=True)
        lyd = oldd.diff(newd)

        return self.put_config(lyd.print_mem("xml", with_siblings=True, pretty=False))

    def get_current_time_with_offset(self):
        root = self.get_data("/ietf-system:system-state/clock", parse=False).data_ele
        current_datetime = root.find('.//{urn:ietf:params:xml:ns:yang:ietf-system}current-datetime').text
        return current_datetime
