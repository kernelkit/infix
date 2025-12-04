import subprocess
import ipaddress
import re

from .common import insert,YangDate
from .host import HOST

def uboot_get_boot_order():
    data = HOST.run_multiline("fw_printenv BOOT_ORDER".split(), [])
    for line in data:
        if "BOOT_ORDER" in line:
            return line.strip().split("=")[1].split()

    raise Exception

def grub_get_boot_order():
    data = HOST.run_multiline("grub-editenv /mnt/aux/grub/grubenv list".split(), [])
    for line in data:
        if "ORDER" in line:
            return line.split("=")[1].strip().split()

    raise Exception

def get_boot_order():
    order = None
    try:
        order = uboot_get_boot_order()
    except:
        pass
    try:
        if order is None:
            order = grub_get_boot_order()
    except:
        pass

    return order

def add_ntp(out):
    data = HOST.run_multiline(["chronyc", "-c", "sources"], [])
    source = []
    state_mode_map = {
        "^": "server",
        "=": "peer",
        "#": "local-clock"
    }
    source_state_map = {
        "*": "selected",
        "+": "candidate",
        "-": "outlier",
        "?": "unusable",
        "x": "falseticker",
        "~": "unstable"
    }
    for line in data:
        src = {}
        line = line.split(',')
        src["address"] = line[2]
        src["mode"] = state_mode_map[line[0]]
        src["state"] = source_state_map[line[1]]
        src["stratum"] = int(line[3])
        src["poll"] = int(line[4])
        source.append(src)

    insert(out, "infix-system:ntp", "sources", "source", source)

def add_dns(out):
    options = {}
    servers = []
    search = []

    content = HOST.read_multiline("/etc/resolv.conf.head", [])
    for line in content:
        line = line.strip()

        if line.startswith('nameserver'):
            ip = line.split()[1]
            try:
                ipaddress.ip_address(ip)
                servers.append({
                    "address": ip,
                    "origin": "static"
                })
            except ValueError:
                continue

        elif line.startswith('search'):
            search.extend(line.split()[1:])

        elif line.startswith('options'):
            opts = line.split()[1:]
            for opt in opts:
                if opt.startswith('timeout:'):
                    options["timeout"] = int(opt.split(':')[1])
                elif opt.startswith('attempts:'):
                    options["attempts"] = int(opt.split(':')[1])

    output = HOST.run_multiline(['/sbin/resolvconf', '-l'], [])
    for line in output:
        line = line.strip()
        if line.startswith('nameserver'):
            parts = line.split('#', 1)
            ip = parts[0].split()[1]

            iface = None
            if len(parts) > 1:
                iface = parts[1].strip()

            try:
                ipaddress.ip_address(ip)
                servers.append({
                    "address": ip,
                    "origin": "dhcp",
                    "interface": iface
                })
            except ValueError:
                continue

        elif line.startswith('search'):
            parts = line.split('#', 1)
            search.extend(parts[0].split()[1:])

    insert(out, "infix-system:dns-resolver", "options", options)
    insert(out, "infix-system:dns-resolver", "server", servers)
    insert(out, "infix-system:dns-resolver", "search", search)

def add_software_slots(out, data):
    slots = []
    for slot in data.get("slots", []):
        for key, value in slot.items():
            new = {}
            new["name"] = key
            new["bootname"] = slot[key].get("bootname")
            new["class"] = slot[key].get("class")
            new["state"] = slot[key].get("state")
            new["bundle"] = {}
            slot_status=value.get("slot_status", {})
            if slot_status.get("bundle", {}).get("compatible"):
                new["bundle"]["compatible"] = slot_status.get("bundle", {}).get("compatible")
            if slot_status.get("bundle", {}).get("version"):
                new["bundle"]["version"] = slot_status.get("bundle", {}).get("version")
            if slot_status.get("checksum", {}).get("size"):
                new["size"] = str(slot_status.get("checksum", {}).get("size"))
            if slot_status.get("checksum", {}).get("sha256"):
                new["sha256"] = slot_status.get("checksum", {}).get("sha256")

            new["installed"] = {}
            if slot_status.get("installed", {}).get("timestamp"):
                new["installed"]["datetime"] = slot_status.get("installed", {}).get("timestamp")

            if slot_status.get("installed", {}).get("count"):
                new["installed"]["count"] = slot_status.get("installed", {}).get("count")

            new["activated"] = {}
            if slot_status.get("activated", {}).get("timestamp"):
                new["activated"]["datetime"] = slot_status.get("activated", {}).get("timestamp")

            if slot_status.get("activated", {}).get("count"):
                new["activated"]["count"] = slot_status.get("activated", {}).get("count")
            slots.append(new)
    out["slot"] = slots

def add_platform(out):
    platform = {}
    pmap = {
        "NAME": "os-name",
        "VERSION_ID": "os-version",
        "BUILD_ID": "os-release",
        "ARCHITECTURE": "machine"
    }

    os_release = HOST.read("/etc/os-release")
    for line in os_release.splitlines():
        key, value = line.split('=')
        name = pmap.get(key)
        if name:
            platform[name] = value.strip("\"")

    insert(out, "platform", platform)

def add_services(out):
    data = HOST.run_json(["initctl", "-j"], [])
    services = []

    for d in data:
        if "pid" not in d or "status" not in d or "identity" not in d or "description" not in d:
            continue

        entry = {
            "pid": d["pid"],
            "name": d["identity"],
            "status": d["status"],
            "description": d["description"],
            "statistics": {
                "memory-usage": str(d.get("memory", 0)),
                "uptime": str(d.get("uptime", 0)),
                "restart-count": int(d.get("restarts", 0))
            }
        }
        services.append(entry)

    insert(out, "infix-system:services", "service", services)

def add_software(out):
    software = {}
    try:
        data = HOST.run_json(["rauc", "status", "--detailed", "--output-format=json"], {})
        software["compatible"] = data.get("compatible", "")
        software["variant"] = data.get("variant", "")
        software["booted"] = data.get("booted", "")
        boot_order = get_boot_order()
        if not boot_order is None:
            software["boot-order"] = boot_order
        add_software_slots(software, data)
    except subprocess.CalledProcessError:
        pass    # Maybe an upgrade i progress, then rauc does not respond

    installer = {}
    installer_status = HOST.run_json(["rauc-installation-status"], {})
    if installer_status.get("operation", {}):
        installer["operation"] = installer_status["operation"]
    if "progress" in installer_status:
        progress = {}

        if installer_status["progress"].get("percentage"):
            progress["percentage"] = int(installer_status["progress"]["percentage"])
        if installer_status["progress"].get("message"):
            progress["message"] = installer_status["progress"]["message"]
        installer["progress"] = progress
    software["installer"] = installer

    insert(out, "infix-system:software", software)

def add_hostname(out):
    hostname =  HOST.run(tuple(["hostname"]))
    out["hostname"] = hostname.strip()

def add_timezone(out):
    path = HOST.run(tuple("realpath /etc/localtime".split()), "")
    timezone = None
    prefixes = [
        '/usr/share/zoneinfo/posix/',
        '/usr/share/zoneinfo/right/',
        '/usr/share/zoneinfo/'
    ]

    for prefix in prefixes:
        if path is not None and path.startswith(prefix):
            timezone = path[len(prefix):]
            break
    if timezone is not None:
        timezone=timezone.strip()
        pattern = r'Etc/GMT([\+\-]\d{1,2})$'
        match = re.search(pattern, timezone)
        if match:
            offset = -int(match.group(1))
            insert(out, "clock", "timezone-utc-offset", offset)
        else:
            if timezone == "Etc/UTC":
                insert(out, "clock", "timezone-utc-offset", 0)
            else:
                insert(out, "clock", "timezone-name", timezone)


def add_users(out):
    shadow_output = HOST.run_multiline(["getent", "shadow"], [])
    users = []

    for line in shadow_output:
        parts = line.split(':')
        if len(parts) < 2:
            continue
        username = parts[0]
        password_hash = parts[1]

        # Skip any records that do not pass YANG validation
        if (not password_hash or
            password_hash.startswith('0') or
            password_hash.startswith('*') or
            password_hash.startswith('!')):
            continue
        user = {}
        user["name"] = username
        user["password"] = password_hash
        users.append(user)


    insert(out, "authentication", "user", users)

def add_clock(out):
    clock = {}
    clock_now=YangDate()

    uptime=HOST.read("/proc/uptime")
    uptime = float(uptime.split()[0])

    clock["boot-datetime"] = str(clock_now.from_seconds(uptime))
    clock["current-datetime"] = str(clock_now)
    insert(out, "clock", clock)

def add_resource_usage(out):
    """Add system resource usage (memory, load average, filesystem) to system-state"""
    resource = {}

    # Memory usage
    try:
        meminfo = HOST.read("/proc/meminfo")
        if not meminfo:
            return
        mem_info = {}
        for line in meminfo.splitlines():
            parts = line.split(":")
            if len(parts) == 2:
                key = parts[0].strip()
                value = parts[1].strip()
                if key in ["MemTotal", "MemFree", "MemAvailable"]:
                    # Store in KiB (as provided by /proc/meminfo, mislabeled as kB)
                    mem_info[key] = int(value.split()[0])

        if mem_info:
            memory = {}
            if "MemTotal" in mem_info:
                memory["total"] = str(mem_info["MemTotal"])
            if "MemFree" in mem_info:
                memory["free"] = str(mem_info["MemFree"])
            if "MemAvailable" in mem_info:
                memory["available"] = str(mem_info["MemAvailable"])
            resource["memory"] = memory
    except (FileNotFoundError, ValueError):
        pass

    # Load average
    try:
        loadavg = HOST.read("/proc/loadavg")
        load_parts = loadavg.strip().split()
        if len(load_parts) >= 3:
            load = {
                "load-1min": load_parts[0],
                "load-5min": load_parts[1],
                "load-15min": load_parts[2]
            }
            resource["load-average"] = load
    except (FileNotFoundError, ValueError):
        pass

    # Filesystem usage
    filesystems = []
    for mount in ["/", "/var", "/cfg"]:
        try:
            result = HOST.run_multiline(["df", "-k", mount], [])
            if len(result) > 1:
                parts = result[1].split()
                if len(parts) >= 4:
                    filesystems.append({
                        "mount-point": mount,
                        "size": str(parts[1]),
                        "used": str(parts[2]),
                        "available": str(parts[3])
                    })
        except (subprocess.CalledProcessError, ValueError, IndexError):
            pass

    if filesystems:
        resource["filesystem"] = filesystems

    if resource:
        insert(out, "infix-system:resource-usage", resource)

def operational():
    out = {
        "ietf-system:system": {
        },
        "ietf-system:system-state": {
        }
    }
    out_state = out["ietf-system:system-state"]
    out_system = out["ietf-system:system"]
    add_hostname(out_system)
    add_users(out_system)
    add_timezone(out_system)
    add_software(out_state)
    add_ntp(out_state)
    add_dns(out_state)
    add_clock(out_state)
    add_platform(out_state)
    add_services(out_state)
    add_resource_usage(out_state)

    return out
