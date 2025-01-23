def _get_local_remote(info):
    return {
        "local": info.get("local") or info.get("local6"),
        "remote": info.get("remote") or info.get("remote6"),
    }
def gre(iplink):
    info=iplink.get("linkinfo", {}).get("info_data", {})

    return _get_local_remote(info)

def vxlan(iplink):
    info=iplink.get("linkinfo", {}).get("info_data", {})
    data=_get_local_remote(info)
    data.update({"vni": info["id"]})

    return data
