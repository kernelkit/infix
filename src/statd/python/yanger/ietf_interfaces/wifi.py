from ..host import HOST

def wifi(ifname):
    data=HOST.run(tuple(f"wpa_cli -i {ifname} status".split()), default="")
    wifi_data={}

    if data != "":
        for line in data.splitlines():
            k,v = line.split("=")
            if k == "ssid":
                wifi_data["ssid"] = v


        data=HOST.run(tuple(f"wpa_cli -i {ifname} signal_poll".split()), default="FAIL")

        # signal_poll return FAIL not connected
        if data.strip() != "FAIL":
            for line in data.splitlines():
                k,v = line.strip().split("=")
                if k == "RSSI":
                    wifi_data["rssi"]=int(v)

    return wifi_data
