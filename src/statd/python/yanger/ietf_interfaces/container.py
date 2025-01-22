from ..host import HOST
from ..infix_containers import podman_ps

from . import common
from . import link


def ip_netns_list():
    return HOST.run_json(["ip", "-j", "netns", "list"], [])


def find_interface(cifname):
    for ns in ip_netns_list():
        for iplink in common.iplinks(netns=ns["name"]).values():
            if iplink.get("ifalias") == cifname:
                ipaddrs = common.ipaddrs(ifname=iplink["ifname"], netns=ns["name"])
                return (iplink, next(iter(ipaddrs.values())))

    return (None, None)


def podman_interfaces():
    interfaces = {}

    for container in podman_ps():
        containername = container.get("Names", ["Unknown"])[0]

        for ifname in container.get("Networks", []):
            if ifname not in interfaces:
                interfaces[ifname] = []
            if containername not in interfaces[ifname]:
                interfaces[ifname].append(containername)

    return interfaces


def interfaces(ifname):
    interfaces = []

    for cifname, cnames in podman_interfaces().items():
        if ifname and cifname != ifname:
            continue

        iplink, ipaddr = find_interface(cifname)
        if not (iplink and ipaddr):
            continue

        interface = link.interface_common(iplink, ipaddr)

        # The original interface name is stored in ifalias by podman -
        # which we then translate to the description. We need to
        # reverse these since the "name" from the user's perspective
        # is the one set in running-config, not whatever the container
        # has renamed it to.
        interface["description"] = interface["name"]
        interface["name"] = cifname

        interface["infix-interfaces:container-network"] = {
            "containers": cnames,
        }
        interfaces.append(interface)

    return interfaces
