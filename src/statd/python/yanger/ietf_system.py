import subprocess
import ipaddress

from .common import insert
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
            search.extend(line.split()[1:])

    insert(out, "infix-system:dns-resolver", "options", options)
    insert(out, "infix-system:dns-resolver", "server", servers)
    insert(out, "infix-system:dns-resolver", "search", search)

def add_software_slots(out, data):
    slots = []
    for slot in data["slots"]:
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

def add_software(out):
    software = {}
    try:
        data = HOST.run_json(["rauc", "status", "--detailed", "--output-format=json"])
        software["compatible"] = data["compatible"]
        software["variant"] = data["variant"]
        software["booted"] = data["booted"]
        boot_order = get_boot_order()
        if not boot_order is None:
            software["boot-order"] = boot_order
        add_software_slots(software, data)
    except subprocess.CalledProcessError:
        pass    # Maybe an upgrade i progress, then rauc does not respond

    installer_status = HOST.run_json(["rauc-installation-status"])
    installer = {}
    if installer_status.get("operation"):
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

def operational():
    out = {
        "ietf-system:system-state": {
        }
    }
    out_state = out["ietf-system:system-state"]

    add_software(out_state)
    add_ntp(out_state)
    add_dns(out_state)

    return out
