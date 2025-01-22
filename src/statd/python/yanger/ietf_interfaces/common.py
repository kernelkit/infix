from functools import cache

from ..host import HOST


@cache
def iplinks(ifname=None, netns=None):
    def _iplinks(ifname, netns):
        pre = [ "ip", "netns", "exec", netns ] if netns else []
        filt = ["dev", ifname] if ifname else []
        return HOST.run_json(pre + ["ip", "-s", "-d", "-j", "link", "show"] + filt)

    return { link["ifname"]: link for link in _iplinks(ifname, netns) }


def iplinks_lower_of(upper):
    return {
        link["ifname"]: link for link in
        filter(lambda link: link.get("master") == upper, iplinks().values())
    }


@cache
def ipaddrs(ifname=None, netns=None):
    def _ipaddrs(ifname, netns):
        pre = [ "ip", "netns", "exec", netns ] if netns else []
        filt = ["dev", ifname] if ifname else []
        return HOST.run_json(pre + ["ip", "-j", "addr", "show"] + filt)

    return { addr["ifname"]: addr for addr in _ipaddrs(ifname, netns) }
