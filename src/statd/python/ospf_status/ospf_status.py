#!/usr/bin/python3
# This script is used to transform the output from the show ip ospf commands and order
# them to match the ietf-ospf YANG model. For example, interfaces is ordered under
# area but FRR has areas in interfaces.
#
# This makes the parsing for the operational parts of YANG model more easy
#

import sys
import json
import subprocess


def run_json_cmd(cmd, default=None, check=True):
    """Run a command (array of args) with JSON output and return the JSON"""
    try:
        cmd.append("2>/dev/null")
        result = subprocess.run(cmd, check=check, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, text=True)
        output = result.stdout
        data = json.loads(output)
    except (subprocess.CalledProcessError, json.JSONDecodeError):
        if default is not None:
            return default
        raise
    return data


def main():
    ospf_interfaces = ['sudo', 'vtysh', '-c', "show ip ospf interface json"]
    ip_ospf = ['sudo', 'vtysh', '-c', "show ip ospf json"]
    ospf_neigh = ['sudo', 'vtysh', '-c', "show ip ospf neighbor detail json"]
    try:
        interfaces = run_json_cmd(ospf_interfaces)
        ospf = run_json_cmd(ip_ospf)
        neighbors = run_json_cmd(ospf_neigh)
    except (subprocess.CalledProcessError, json.JSONDecodeError):
        return {}

    for ifname, iface in interfaces["interfaces"].items():
        iface["name"] = ifname
        iface["neighbors"] = []
        for area_id in ospf["areas"]:
            area_type=""

            stub=False
            if("NSSA" in iface["area"]):
                iface_area_id = iface["area"][:-7]
                area_type = "nssa-area"
            elif("Stub" in iface["area"]):
                iface_area_id = iface["area"][:-7]
                iface["areaId"] = iface["area"][:-7]
                area_type = "stub-area"
            else:
                iface_area_id = iface["area"]
                area_type = "normal-area"

            if(iface_area_id != area_id):
                continue

            ospf["areas"][area_id]["area-type"] = area_type
            iface["area"] = iface_area_id

            for nbrAddress,nbrDatas in neighbors["neighbors"].items():
                for nbrData in nbrDatas:
                    nbrIfname=nbrData["ifaceName"]
                    if(("NSSA" in nbrData.get("areaId", {})) or ("Stub" in nbrData.get("areaId", {}))):
                        nbrData["areaId"] = nbrData["areaId"][:-7]

                    if ((nbrIfname != ifname) or (area_id != nbrData.get("areaId"))):
                        #print(f'Continute {ifname} {nbrData.get("areaId")}')
                        continue
                    nbrData["neighborIp"] = nbrAddress
                    iface["neighbors"].append(nbrData)

            if(not ospf["areas"][area_id].get("interfaces", None)):
                ospf["areas"][area_id]["interfaces"] = []
            ospf["areas"][area_id]["interfaces"].append(iface)

    print(json.dumps(ospf))


if __name__ == "__main__":
    main()
    sys.exit(0)
