"""Operational data provider for infix-containers YANG model.

Collects container status, network info, resource limits from cgroups,
and runtime statistics via podman commands.
"""
import os
import re

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


def parse_size_kib(size_str):
    """Parse size string like '1.5MB' or '512kB' to KiB (kibibytes)."""
    if not size_str:
        return 0

    size_str = size_str.strip().upper()

    # Extract numeric part and unit
    match = re.match(r'([0-9.]+)\s*([KMGT]?I?B)?', size_str)
    if not match:
        return 0

    value = float(match.group(1))
    unit = match.group(2) if match.group(2) else 'B'

    # Convert to KiB (kibibytes)
    multipliers = {
        'B': 1/1024,
        'KB': 1000/1024, 'KIB': 1,
        'MB': (1000**2)/1024, 'MIB': 1024,
        'GB': (1000**3)/1024, 'GIB': 1024**2,
        'TB': (1000**4)/1024, 'TIB': 1024**3,
    }

    return int(value * multipliers.get(unit, 1))


def parse_cgroup_memory(mem_str):
    """Parse cgroup memory.max value (bytes) to KiB."""
    if not mem_str or mem_str == "max":
        return 0
    try:
        mem_bytes = int(mem_str)
        return mem_bytes // 1024
    except ValueError:
        return 0


def parse_cgroup_cpu(cpu_str):
    """Parse cgroup cpu.max value to millicores."""
    if not cpu_str:
        return 0
    parts = cpu_str.split()
    if len(parts) != 2 or parts[0] == "max":
        return 0
    try:
        quota = int(parts[0])
        period = int(parts[1])
        # Convert to millicores: (quota/period) * 1000
        return (quota * 1000) // period
    except ValueError:
        return 0


def read_cgroup_limits(inspect):
    """Read resource limits from cgroup files for a container."""
    if not inspect or not isinstance(inspect, dict):
        return None

    cgroup_path = inspect.get("State", {}).get("CgroupPath")
    if not cgroup_path:
        return None

    cgroup_base = f"/sys/fs/cgroup{cgroup_path}"
    mem_val = 0
    cpu_val = 0

    try:
        # Read memory limit (in bytes, convert to KiB)
        mem_max_path = os.path.join(cgroup_base, "memory.max")
        if os.path.exists(mem_max_path):
            with open(mem_max_path, 'r') as f:
                mem_str = f.read().strip()
                mem_val = parse_cgroup_memory(mem_str)

        # Read CPU limit (quota and period in microseconds, convert to millicores)
        cpu_max_path = os.path.join(cgroup_base, "cpu.max")
        if os.path.exists(cpu_max_path):
            with open(cpu_max_path, 'r') as f:
                cpu_str = f.read().strip()
                cpu_val = parse_cgroup_cpu(cpu_str)
    except Exception as e:
        LOG.error(f"failed reading cgroup limits: {e}")
        return None

    if mem_val > 0 or cpu_val > 0:
        result = {}
        if mem_val > 0:
            result["memory"] = f"{mem_val}"
        if cpu_val > 0:
            result["cpu"] = cpu_val
        return result

    return None


def resource_stats(name):
    """Get resource usage stats for a running container using podman stats."""
    cmd = ['podman', 'stats', '--no-stream', '--format', 'json', '--no-reset', name]
    try:
        stats = HOST.run_json(cmd, default=[])
        if not stats or len(stats) == 0:
            return None

        stat = stats[0]
        rusage = {}

        # Memory usage - parse used memory, convert to KiB
        # Encode as string for uint64 compatibility
        mem_usage_str = stat.get("mem_usage", "")
        if "/" in mem_usage_str:
            mem_used_str = mem_usage_str.split("/")[0].strip()
            mem_used_kib = parse_size_kib(mem_used_str)
            rusage["memory"] = f"{mem_used_kib}"

        # CPU percentage - format as decimal64 with 2 fractional digits
        cpu_perc = stat.get("cpu_percent", "0%").rstrip("%")
        try:
            rusage["cpu"] = "{:.2f}".format(float(cpu_perc))
        except (ValueError, TypeError):
            pass

        block_io = stat.get("block_io", "0B / 0B")
        if "/" in block_io:
            block_read_str, block_write_str = block_io.split("/")
            block_read_kib = parse_size_kib(block_read_str.strip())
            block_write_kib = parse_size_kib(block_write_str.strip())

            rusage["block-io"] = {}
            if block_read_kib > 0:
                rusage["block-io"]["read"] = f"{block_read_kib}"
            if block_write_kib > 0:
                rusage["block-io"]["write"] = f"{block_write_kib}"

        net_io = stat.get("net_io", "0B / 0B")
        if "/" in net_io:
            net_rx_str, net_tx_str = net_io.split("/")
            net_rx_kib = parse_size_kib(net_rx_str.strip())
            net_tx_kib = parse_size_kib(net_tx_str.strip())

            rusage["net-io"] = {}
            if net_rx_kib > 0:
                rusage["net-io"]["received"] = f"{net_rx_kib}"
            if net_tx_kib > 0:
                rusage["net-io"]["sent"] = f"{net_tx_kib}"

        pids = stat.get("pids", "0")
        try:
            rusage["pids"] = int(pids)
        except (ValueError, TypeError):
            pass

        return rusage if rusage else None

    except Exception as e:
        LOG.error(f"failed getting stats for {name}: {e}")
        return None


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

    limits = read_cgroup_limits(inspect)
    if limits:
        out["resource-limit"] = limits

    if out["running"]:
        rusage = resource_stats(out["name"])
        if rusage:
            out["resource-usage"] = rusage

    return out


def operational():
    return {
        "infix-containers:containers": {
            "container": [container(ps) for ps in podman_ps()]
        }
    }
