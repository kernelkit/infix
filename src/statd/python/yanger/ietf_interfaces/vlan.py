def proto2yang(proto):
    return {
        "802.1Q":  "ieee802-dot1q-types:c-vlan",
        "802.1ad": "ieee802-dot1q-types:s-vlan",
    }.get(proto, "other")


def vlan(iplink):
    info = iplink["linkinfo"]["info_data"]

    vlan = {
        "tag-type": proto2yang(info["protocol"]),
        "id": info["id"],
    }

    # Lower could be in a different namespace, and thus might not be
    # available to us
    if lower := iplink.get("link"):
        vlan["lower-layer-if"] = lower

    return vlan
