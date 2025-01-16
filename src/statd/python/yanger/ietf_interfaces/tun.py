def gre(iplink):
    info=iplink.get("linkinfo", {}).get("info_data", {})

    return {
        "local": info["local"],
        "remote": info["remote"],
    }
