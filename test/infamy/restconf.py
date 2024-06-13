import requests
import json
import warnings
import os
import sys
import libyang

from requests.auth import HTTPBasicAuth
from urllib.parse import quote
from urllib3.exceptions import InsecureRequestWarning
from dataclasses import dataclass
from lxml import etree
from infamy.transport import Transport

# We know we have a self-signed certificate, silence warning about it
warnings.simplefilter('ignore', InsecureRequestWarning)

@dataclass
class Location:
    interface: str
    host: str
    password: str
    username: str = "admin"
    port: int = 443

class Device(Transport):
    def __init__(self,
                 location: Location,
                 mapping: dict,
                 yangdir: None | str = None,
                 factory_default = True):
        self.location=location
        self.url_base=f"https://[{location.host}]:{location.port}"
        self.restconf_url=f"{self.url_base}/restconf"
        self.yang_url=f"{self.url_base}/yang/"
        self.rpc_url=f"{self.url_base}/restconf/operations"
        self.headers={
            'Content-Type': 'application/yang-data+json',
            'Accept': 'application/yang-data+json'
        }
        self.auth=HTTPBasicAuth(location.username, location.password)
        self.modules={}

        self.lyctx=libyang.Context(yangdir)
        self._ly_bootstrap(yangdir)
        self._ly_init(yangdir)

        if factory_default:
            self.factory_default()

    def get_schemas_list(self):
        data=self.get_operational("/ietf-yang-library:modules-state").print_dict()
        return data["modules-state"]["module"]

    def get_schema(self, name, revision, yangdir):
        schema_name=f"{name}@{revision}.yang"
        url=f"{self.yang_url}/{schema_name}"
        data=self._get_raw(url=url, parse=False)

        sys.stdout.write(f"Downloading YANG model {schema_name} ...\r\033[K")
        data=data.decode('utf-8')
        with open(f"{yangdir}/{schema_name}", 'w') as json_file:
            json_file.write(data)

    def schema_exist(self, name, revision, yangdir):
        schema_name=f"{name}@{revision}.yang"
        schema_path=f"{yangdir}/{schema_name}"
        return os.path.exists("{schema_path}")

    def _ly_bootstrap(self, yangdir):
        schemas=self.get_schemas_list()
        for schema in schemas:
            if not self.schema_exist(schema["name"], schema["revision"],yangdir):
                self.get_schema(schema["name"], schema["revision"], yangdir)
            if schema.get("submodule"):
                for submodule in schema["submodule"]:
                    if not self.schema_exist(submodule["name"], submodule["revision"],yangdir):
                        self.get_schema(submodule["name"], submodule["revision"], yangdir)

            if not any("submodule" in x and schema["name"] in x["submodule"] for x in schemas):
                self.modules.update({schema["name"]: schema})
            sys.stdout.write(f"Downloading YANG model {schema['name']} ...\r\033[K")

        print("YANG models downloaded.")

    def _ly_init(self, yangdir):
        lib=self.lyctx.load_module("ietf-yang-library")
        ns=libyang.util.c2str(lib.cdata.ns)

        for ms in self.modules.values():
            if ms["conformance-type"] != "implement":
                continue

            mod=self.lyctx.load_module(ms["name"])

            # TODO: ms["feature"] contains the list of enabled
            # features, so ideally we should only enable the supported
            # ones. However, features can depend on each other, so the
            # naïve looping approach doesn't work.
            mod.feature_enable_all()

    def _get_raw(self, url, parse=True):
        """Actually send a GET to RESTCONF server"""
        response=requests.get(url, headers=self.headers, auth=self.auth, verify=False)
        response.raise_for_status()  # Raise an exception for HTTP errors
        if parse:
            data=response.json()
            data=self.lyctx.parse_data_mem(json.dumps(data), "json", parse_only=True)
            return data
        else:
            return response.content

    def get_datastore(self, datastore="operational" , xpath="", parse=True):
        """Get a datastore"""
        path=f"/ds/ietf-datastores:{datastore}"
        if not xpath is None:
            path=f"{path}/{xpath}"
        path=quote(path, safe="/:")
        url=f"{self.restconf_url}{path}"
        return self._get_raw(url, parse)

    def get_running(self, xpath=None):
        """Wrapper function to get running datastore"""
        return self.get_datastore("running", xpath)

    def get_operational(self, xpath=None, parse=True):
        """Wrapper function to get operational datastore"""
        return self.get_datastore("operational", xpath, parse)

    def get_factory(self, xpath=None):
        """Wrapper function to get factory defaults"""
        return self.get_datastore("factory-default", xpath)

    def post_datastore(self, datastore, data):
        """Actually send a POST to RESTCONF server"""
        response=requests.post(
            f"{self.restconf_url}/ds/ietf-datastores:{datastore}/",
            json=data,  # Directly pass the dictionary without using json.dumps
            headers=self.headers,
            auth=self.auth,
            verify=False
        )
        response.raise_for_status()  # Raise an exception for HTTP errors

    def put_datastore(self, datastore, data):
        """Actually send a PUT to RESTCONF server"""
        response=requests.put(
            f"{self.restconf_url}/ds/ietf-datastores:{datastore}/",
            json=data,  # Directly pass the dictionary without using json.dumps
            headers=self.headers,
            auth=self.auth,
            verify=False
        )
        response.raise_for_status()  # Raise an exception for HTTP errors

    def get_config_dict(self, modname):
        """Get all configuration for module @modname as dictionary"""
        ds=self.get_running(modname)
        ds=json.loads(ds.print_mem("json", with_siblings=True, pretty=False))
        model, container=modname.split(":")
        for k, v in ds.items():
            return {container: v}

    def put_config_dict(self, modname, edit):
        """Add @edit to running config and put the whole configuration"""
        running=self.get_running()
        mod=self.lyctx.get_module(modname)
        change=mod.parse_data_dict(edit, no_state=True, validate=False)
        running.merge_module(change)

        return self.put_datastore("running", json.loads(running.print_mem("json", with_siblings=True, pretty=False)))

    def call_rpc(self, rpc):
        url=f"{self.rpc_url}/{rpc}"
        """Actually send a POST to RESTCONF server"""
        response=requests.post(
            url,
            headers=self.headers,
            auth=self.auth,
            verify=False
        )
        response.raise_for_status()  # Raise an exception for HTTP errors

    def get_dict(self, xpath=None, parse=True):
        """NETCONF compat function, just wraps get_data"""
        return self.get_data(xpath, parse)

    def get_data(self, xpath=None, parse=True):
        """Get operational data"""
        data=self.get_operational(xpath, parse)
        if parse==False:
            return data

        data=json.loads(data.print_mem("json", with_siblings=True, pretty=False))

        for k,v in data.items():
            model, container=k.split(":")
            break
        return {container: v}


    def copy(self, source, target):
        factory=self.get_datastore(source)
        data=factory.print_mem("json", with_siblings=True, pretty=False)
        self.put_datastore(target, json.loads(data))

    def reboot(self):
        self.call_rpc("ietf-system:system-restart")

    def factory_default(self):
        """Factory reset target"""
        return self.copy("factory-default", "running")

    def get_xpath(self,  xpath, key, value, path=None):
        """Compose complete XPath to a YANG node"""
        xpath=f"{xpath}={value}"

        if not path is None:
            xpath=f"{xpath}/{path}"

        return xpath

    def get_current_time_with_offset(self):
        data=self.get_data("/ietf-system:system-state/clock", parse=False)
        data=json.loads(data)
        print(data)
        return data["ietf-system:system-state"]["clock"]["current-datetime"]

    def get_iface(self, iface):
        """Fetch target dict for iface and extract param from JSON"""
        content=self.get_data(self.get_iface_xpath(iface))
        interface=content.get("interfaces", {}).get("interface", None)
        if interface is None:
            return None

        # This is a bug in rousette, should be able to use the same code as NETCONF
        return interface[0]

    def delete_xpath(self, xpath):
        """Delete XPath from running config"""
        path=f"/ds/ietf-datastores:running/{xpath}"
        path=quote(path, safe="/:")
        url=f"{self.restconf_url}{path}"
        response=requests.delete(url, headers=self.headers, auth=self.auth, verify=False)
        response.raise_for_status()  # Raise an exception for HTTP errors
        return True
