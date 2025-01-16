def veth(iplink):
    veth = {}

    if peer := iplink.get("link"):
        veth["peer"] = peer

    return veth
