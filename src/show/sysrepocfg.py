import shlex
import sys
import subprocess
import json

class Sysrepocfg:
    def __init__(self, data_file=None, raw=False):
        self.data_file = data_file
        self.raw_output = raw

    def _run(self, safe_xpath):
        try:
            result = subprocess.run([
                "sysrepocfg", "-f", "json", "-X", "-d", "operational", "-x", safe_xpath
            ], capture_output=True, text=True, check=True)
            data = json.loads(result.stdout)
            if not data:
                sys.exit(f"Error, no data from sysrepo")
            return data
        except subprocess.CalledProcessError as e:
            sys.exit(f"Error, running sysrepocfg: {e}")
        except json.JSONDecodeError as e:
            sys.exit(f"Error, parsing JSON output: {e}")


    def _run_emulator(self, xpath):
        """
        Emulates `sysrepocfg -X -x <xpath>` by extracting a subtree from the JSON file.

        :param json_file: Path to the local JSON file containing sysrepo-style data.
        :param xpath: YANG-style XPath (e.g., '/ietf-interfaces:interfaces').
        :return: Extracted subtree or None if not found.
        """
        with open(self.data_file, 'r') as f:
            data = json.load(f)

        path_parts = xpath.strip('/').split('/')

        node = data
        for part in path_parts:
            if isinstance(node, dict) and part in node:
                node = node[part]
            else:
                return None  # Path doesn't exist

        return node


    def run(self, xpath: str) -> dict:
        if not isinstance(xpath, str) or not xpath.startswith("/"):
            print("Invalid XPATH. It must be a valid string starting with '/'.")
            return {}

        safe_xpath = shlex.quote(xpath)

        if self.data_file is not None:
            return self._run_emulator(safe_xpath)

        return self._run(safe_xpath)
