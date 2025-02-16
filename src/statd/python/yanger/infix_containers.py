from .common import LOG
from .host import HOST


# Catch errors (check=True), at this point we've run 'podman ps' (below)
def podman_inspect(name):
    """Call podman inspect {name}, return object at {path} or None."""
    cmd = ['podman', 'inspect', name]
    try:
        return HOST.run_json(cmd, default=[])
    except Exception as e:
        LOG.error(f"failed podman inspect: {e}")
        return []


# Ignore any errors here, may be called on a build without containers
def podman_ps():
    """We list *all* containers, not just those in the configuraion."""
    cmd = ['podman', 'ps', '-a', '--format=json']
    return HOST.run_json(cmd, default=[])


def network(ps, inspect):
    net = {}

    # The 'podman ps' command lists ports even in host mode, but
    # that's not applicable, so skip networks and port forwardings
    networks = inspect.get("NetworkSettings", {}).get("Networks")
    if networks and "host" in networks:
        net = {"host": True}
    else:
        net = {
            "interface": [{"name": net} for net in ps["Networks"]],
            "publish": []
        }

        if (ps["State"] == "running") and ps["Ports"]:
            for port in ps["Ports"]:
                addr = ""
                if port["host_ip"]:
                    addr = f"{port['host_ip']}:"

                pub = f"{addr}{port['host_port']}->{port['container_port']}/{port['protocol']}"
                net["publish"].append(pub)

    return net


def container(ps):
    out = {
        "name":     ps["Names"][0],
        "id":       ps["Id"],
        "image":    ps["Image"],
        "image-id": ps["ImageID"],
        "running":  ps["State"] == "running",
        "status":   ps["Status"]
    }

    # Bonus information, may not be available
    if ps["Command"]:
        out["command"] = " ".join(ps["Command"])

    inspect = podman_inspect(out["name"])
    if inspect and isinstance(inspect, list) and len(inspect) > 0:
        inspect = inspect[0]
    else:
        inspect = {}

    net = network(ps, inspect)
    if net:
        out["network"] = net

    return out


def operational():
    return {
        "infix-containers:containers": {
            "container": [container(ps) for ps in podman_ps()]
        }
    }
