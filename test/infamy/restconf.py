import requests
import json
import warnings
import os
import sys

from requests.auth import HTTPBasicAuth
from urllib.parse import quote
from urllib3.exceptions import InsecureRequestWarning
from dataclasses import dataclass

# We know we have a self-signed certificate, silence warning about it
warnings.simplefilter('ignore', InsecureRequestWarning)

@dataclass
class Location:
    interface: str
    host: str
    password: str
    username: str = "admin"
    port: int = 443

class Device(object):
    def __init__(self,
                 location: Location,
                 mapping: dict,
                 yangdir: None | str = None,
                 factory_default = True):
        self.location = location
        self.url = f"https://[{location.host}]:{location.port}/restconf"
        self.yang_url = f"https://[{location.host}]:{location.port}/yang/"
        self.headers = {
            'Content-Type': 'application/yang-data+json',
            'Accept': 'application/yang-data+json'
        }
        self.auth = HTTPBasicAuth(location.username, location.password)
        self.modules = {}

        if factory_default:
            self.factory_default()

    def get_schemas_list(self):
        data=self.get_datastore("operational", "/ietf-yang-library:modules-state")
        self.modules = { m["name"] : m for m in data["ietf-yang-library:modules-state"]["module"] }
        return data["ietf-yang-library:modules-state"]["module"]

    def get_schema(self, schema_name, yangdir):
        schema_url=f"{self.yang_url}/{schema_name}"
        data=self.do_restconf(self.yang_url,schema_name)
        # print(type(data))
        sys.stdout.write(f"Downloading YANG model {schema_name} ...\r\033[K")
        data=data.decode('utf-8')
        with open(f"{yangdir}/{schema_name}", 'w') as json_file:
            json_file.write(data)

    def schema_exist(self, schema_name, yangdir):
        schema_path=f"{yangdir}/{schema_name}"
        return os.path.exists("{schema_path}")

    def get_mgmt_ip(self):
        return self.location.host

    def get_mgmt_iface(self):
        return self.location.interface


    def address(self):
        """Return managment IP address used for NETCONF"""
        return self.location.host

    def reachable(self):
        neigh = ll6ping(self.location.interface, flags=["-w1", "-c1", "-L", "-n"])
        return bool(neigh)

    def do_restconf(self, url, path):
        path = quote(path, safe="/:")
        url = f"{url}/{path}"
        try:
            response = requests.get(url, headers=self.headers, auth=self.auth, verify=False)
            response.raise_for_status()  # Raise an exception for HTTP errors
            return response.content
        except requests.exceptions.HTTPError as errh:
            print("HTTP Error:", errh)
        except requests.exceptions.ConnectionError as errc:
            print("Error Connecting:", errc)
        except requests.exceptions.Timeout as errt:
            print("Timeout Error:", errt)
        except Exception as err:
            print("Unknown error", err)

    def get_datastore(self, datastore, xpath="", as_xml=False):
        path = f"/ds/ietf-datastores:{datastore}/{xpath}"
        path = quote(path, safe="/:")
        url = f"{self.url}{path}"
        try:
            response = requests.get(url, headers=self.headers, auth=self.auth, verify=False)
            response.raise_for_status()  # Raise an exception for HTTP errors
            data = response.json()
            return data
        except requests.exceptions.HTTPError as errh:
            print("HTTP Error:", errh)
        except requests.exceptions.ConnectionError as errc:
            print("Error Connecting:", errc)
        except requests.exceptions.Timeout as errt:
            print("Timeout Error:", errt)
        except Exception as err:
            print("Unknown error", err)

    def put_datastore(self, datastore, data):
        try:
            response = requests.put(
                f"{self.url}/ds/ietf-datastores:{datastore}/",
                json=data,  # Directly pass the dictionary without using json.dumps
                headers=self.headers,
                auth=self.auth,
                verify=False
            )
            response.raise_for_status()  # Raise an exception for HTTP errors
        except requests.exceptions.HTTPError as errh:
            print("HTTP Error:", errh)
        except requests.exceptions.ConnectionError as errc:
            print("Error Connecting:", errc)
        except requests.exceptions.Timeout as errt:
            print("Timeout Error:", errt)
        except requests.exceptions.RequestException as err:
            print("Unknown error", err)

    def get_config_dict(self, modname):
        ds = self.get_datastore("running", modname)
        model, container = modname.split(":")
        for k, v in ds.items():
            return {container: v}

    def merge_dicts(self, dict1, dict2):
        merged = dict1.copy()  # Start with the first dictionary

        for key, value in dict2.items():
            if key in merged:
                if isinstance(merged[key], dict) and isinstance(value, dict):
                    # If both values are dictionaries, merge them recursively
                    merged[key] = self.merge_dicts(merged[key], value)
                else:
                    # If they are not both dictionaries, overwrite the value from d2
                    merged[key] = value

        return merged

    def put_config_dict(self, modname, edit):
        new = {}
        for k, v in edit.items():
            new[f"{modname}:{k}"] = v

        running = self.get_datastore("running")
        running=self.merge_dicts(running, new)

        return self.put_datastore("running", running)

    def get_data(self, xpath=None, as_xml=False):
        data=self.get_datastore("operational", xpath, as_xml)
        for k,v in data.items():
            model, container = k.split(":")

        return {container: v}

    def factory_default(self):
        factory=self.get_datastore("factory-default")
        self.put_datastore("running", factory)
