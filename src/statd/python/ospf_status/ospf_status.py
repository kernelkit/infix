#!/usr/bin/python3
# This script is used to transform the output from the show ip ospf commands and order
# them to match the ietf-ospf YANG model. For example, interfaces is ordered under
# area but FRR has areas in interfaces.
#
# This makes the parsing for the operational parts of YANG model more easy
#

import json
import subprocess

def main():
    iface_out=subprocess.check_output("vtysh -c 'show ip ospf interface json'", shell=True)
    ospf_out=subprocess.check_output("vtysh -c 'show ip ospf json'", shell=True)
    neighbor_out=subprocess.check_output("vtysh -c 'show ip ospf neighbor detail json'", shell=True)
    interfaces=json.loads(iface_out)
    ospf=json.loads(ospf_out)
    neighbors=json.loads(neighbor_out)

    for ifname,iface in interfaces["interfaces"].items():
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
                    nbrIfname=nbrData["ifaceName"].split(":")[0]
                    if(("NSSA" in nbrData.get("areaId", {})) or ("Stub" in nbrData.get("areaId", {}))):
                        nbrData["areaId"] = nbrData["areaId"][:-7]

                    if ((nbrIfname != ifname) and (area_id != nbrData.get("areaId"))):
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
