import subprocess
import argparse
import syslog
import json
import sys
import os
import re

debug = False

syslog.openlog(logoption=syslog.LOG_PID, facility=syslog.LOG_SYSLOG)


def dbg(msg):
    global debug
    if debug:
        syslog.syslog(syslog.LOG_INFO, msg)


def runcmd(cmd):
    ret = None
    try:
        res = subprocess.run(cmd, check=True, stdout=subprocess.PIPE,
                             stderr=subprocess.PIPE, text=True)
        if res.returncode == 0:
            if res.stdout:
                ret = res.stdout.strip()
            else:
                ret = True
    except subprocess.CalledProcessError:
        return None
    finally:
        return ret


def runcmdj(cmd):
    output = runcmd(cmd)
    if not output:
        return None
    try:
        data = json.loads(output)
    except json.JSONDecodeError:
        return None
    finally:
        return data


def vbool(val):
    if val == "yes" or val == "true":
        return True
    else:
        return False


def vnum(val):
    if val != "--":
        return int(val)
    else:
        return 0


def vnumstr(val):
    if val != "" and val != "--":
        return val
    else:
        return "0"


def vstr(val):
    if val != "--":
        return val
    else:
        return ""


def venum(val, enums, default):
    enval = vstr(val)
    for en in enums:
        if enval == en:
            return en
    return default


def vip4(val):
    val = vstr(val)
    if val == "":
        return "0.0.0.0"
    else:
        return val


def vip6(val):
    val = vstr(val)
    if val == "":
        return "::"
    else:
        return val


def fread(path):
    if os.path.exists(path):
        with open(path, "r") as fd:
            output = str(fd.read())
            if output:
                return output.strip()
    return None


def freadj(path):
    output = fread(path)
    if not output:
        return None
    try:
        data = json.loads(output)
    except json.JSONDecodeError:
        dbg("Unable to parse json output")
        return None
    finally:
        return data


def get_manufacturer(index, gen):
    path = "/run/modemd/modem%d/manufacturer" % index
    manf = fread(path)
    if manf:
        return manf

    manf = vstr(gen.get("manufacturer", ""))
    return manf


def get_model(index, gen):
    path = "/run/modemd/modem%d/model" % index
    model = fread(path)
    if model:
        return model

    model = vstr(gen.get("model", ""))
    return model


def get_supported_carriers(index, manf, model):
    path = "/run/modemd/modem%d/supported-carriers" % index
    output = freadj(path)
    if output:
        return output
    else:
        return []


def get_current_carrier(index, manf, model):
    path = "/run/modemd/modem%d/current-carrier" % index
    output = freadj(path)
    if output:
        return output
    else:
        return "default"


def _sim_lock_state(unlock_required):
    mapping = {
        "none":     "unlocked",
        "sim-pin":  "pin-required",
        "sim-pin2": "pin-required",
        "sim-puk":  "puk-required",
        "sim-puk2": "puk-required",
    }
    # Default to "unlocked": this function is only called when the SIM query
    # succeeded, so the card is present.  "--" means the modem doesn't report
    # lock state but the SIM is accessible.
    return mapping.get(unlock_required, "unlocked")


def device_devpath(devpath):
    path = devpath
    while path != "/sys":
        path = os.path.realpath("%s/.." % path)
        subdir = os.path.basename(path)
        r = re.search(r"usb\d", subdir)
        if r:
            return path
    return None


def modem_get_module(devpath):
    # Prefer system.json (new canonical source written by 00-probe)
    if os.path.exists("/run/system.json"):
        with open("/run/system.json", "r") as fd:
            data = json.load(fd)
        for modem in data.get("modem", []):
            if devpath in modem.get("devpath", ""):
                idx = modem.get("index", 0)
                return {"index": idx, "slot": idx,
                        "paths": [modem["devpath"]], "type": "modem"}

    # Fallback: Minex modules.json (hardware with slot/SIM multiplexer)
    if os.path.exists("/run/modules.json"):
        with open("/run/modules.json", "r") as fd:
            data = json.load(fd)
        for module in data.get("modules", []):
            if module["type"] != "modem":
                continue
            for path in module.get("paths", []):
                if path in devpath:
                    return module
        return None

    # Fallback: legacy modems.json written by modem-udev
    if os.path.exists("/run/modems.json"):
        with open("/run/modems.json", "r") as fd:
            data = json.load(fd)
        for index, modem in enumerate(data.get("modems", [])):
            if devpath in modem.get("devpath", ""):
                return {"index": index, "slot": index,
                        "paths": [modem["devpath"]], "type": "modem"}

    return None


def modem_get_sim(module):
    path = "/sys/class/sim"
    for index in range(0, 5):
        name = "sim%d" % index
        if not os.path.exists("%s/%s" % (path, name)):
            continue

        slot = int(fread("%s/%s/slot" % (path, name)))
        if slot != module["slot"]:
            continue

        present = int(fread("%s/%s/present" % (path, name)))

        return {
            "index": index,
            "name": name,
            "slot": slot,
            "present": bool(present)
        }

    return None


def sysfs_interfaces(devpath):
    ifaces = []
    try:
        for iface in os.listdir("/sys/class/net"):
            p = os.path.realpath("/sys/class/net/%s" % iface)
            if p.startswith(devpath):
                ifaces.append(iface)
    except OSError:
        pass
    return ifaces


def print_modem(index):
    # Try system.json first (canonical source written by 00-probe)
    if os.path.exists("/run/system.json"):
        with open("/run/system.json", "r") as fd:
            data = json.load(fd)
        for modem in data.get("modem", []):
            if modem.get("index") == index:
                module = {"index": index, "slot": index,
                          "paths": [modem["devpath"]], "type": "modem"}
                modem["sim"] = modem_get_sim(module) or \
                    {"index": 0, "name": "sim0", "slot": 0, "present": True}
                if not modem.get("interfaces"):
                    modem["interfaces"] = sysfs_interfaces(modem["devpath"])
                print(json.dumps(modem))
                sys.exit(0)

    # Fallback: legacy modems.json written by modem-udev
    if os.path.exists("/run/modems.json"):
        with open("/run/modems.json", "r") as fd:
            data = json.load(fd)
        for modem in data.get("modems", []):
            module = modem_get_module(modem["devpath"])
            if module and module["index"] == index:
                modem["slot"] = module["slot"]
                modem["sim"] = modem_get_sim(module) or \
                    {"index": 0, "name": "sim0", "slot": 0, "present": True}
                if not modem.get("interfaces"):
                    modem["interfaces"] = sysfs_interfaces(modem["devpath"])
                print(json.dumps(modem))
                sys.exit(0)

    print(json.dumps({}))
    sys.exit(0)


def print_all():
    modems = []

    output = runcmdj(['mmcli', '-J', '-L'])
    if not output:
        dbg("mmcli command failed")
        print(json.dumps(modems))
        sys.exit(0)

    for mpath in output['modem-list']:
        output = runcmdj(['mmcli', '-J', '-m', mpath])
        if not output:
            dbg("no output for %s" % mpath)
            continue
        modem = output.get("modem")
        if modem is None:
            dbg("no modem for %s" % mpath)
            continue
        gen = modem.get("generic")
        if gen is None:
            dbg("no generic for %s" % mpath)
            continue
        gpp = modem.get("3gpp")
        if gpp is None:
            dbg("no 3gpp for %s" % mpath)
            continue

        devpath = device_devpath(gen["device"])
        if devpath is None:
            dbg("no devpath for %s" % mpath)
            continue

        module = modem_get_module(devpath)
        if module is None:
            dbg("no modem module for %s" % devpath)
            continue

        index = module["index"]

        manf = get_manufacturer(index, gen)
        if manf is None:
            dbg("no manufacturer for modem%d" % index)
            continue

        model = get_model(index, gen)
        if model is None:
            dbg("no model for modem%d" % index)
            continue

        modem = {
            "index": index,
            "path": mpath
        }

        modem["info"] = {
            "manufacturer": manf,
            "model": model,
            "supported-carrier": get_supported_carriers(index, manf, model),
            "hardware-revision": vstr(gen["hardware-revision"]),
            "firmware-version": vstr(gen["revision"]),
            "serial-number": vstr(gen["equipment-identifier"]),
            "phone-number": gen.get("own-numbers", []),
        }

        sig_q = gen.get("signal-quality") or {}
        modem["status"] = {
            "state": vstr(gen["state"]),
            "selected-carrier": get_current_carrier(index, manf, model),
            "signal-quality": vnum(sig_q.get("value", "0")),
            "signal-quality-recent": vbool(sig_q.get("recent", "no")),
            "power-state": vstr(gen.get("power-state", "--")) or "unknown",
        }

        locks = gpp.get("enabled-locks") or []
        if isinstance(locks, list):
            modem["status"]["enabled-locks"] = [
                lk for lk in locks if lk and lk != "--" and lk != "none"
            ]

        if gen["state"] == "failed":
            reason = vstr(gen.get("state-failed-reason", "--"))
            if len(reason) > 0:
                modem["status"]["state-failed-reason"] = reason

        sim = modem_get_sim(module)
        if sim is not None:
            modem["status"]["sim-active"] = sim["index"]
            modem["status"]["sim-present"] = sim["present"]
            sim_name = sim["name"]
            sim_slot = sim["slot"]
        else:
            sim_name = "sim0"
            slot_raw = gen.get("primary-sim-slot", 1)
            sim_slot = int(slot_raw) if str(slot_raw).isdigit() else 1

        # rssi / rsrp are int16 dBm in YANG; rsrq / sinr are decimal64 dB.
        # mmcli reports them as floating-point strings; normalise here.
        int_signals = {"rssi", "rsrp"}
        dec_signals = {"rsrq", "sinr"}

        refresh = 0
        output = runcmdj(['mmcli', '-J', '-m', mpath, '--signal-get'])
        if output:
            refresh = int(output["modem"]["signal"]["refresh"]["rate"])
        if refresh == 0:
            # enable extended signal info
            runcmd(['mmcli', '-J', '-m', mpath, '--signal-setup=5'])
        else:
            signals = ["rssi", "rsrp", "rsrq", "rscp", "snr", "sinr"]
            found = False
            for tech in output["modem"]["signal"].keys():
                if tech == "refresh":
                    continue
                for sig in signals:
                    v = output["modem"]["signal"][tech].get(sig, "--")
                    if v == "--":
                        continue
                    try:
                        if sig in int_signals:
                            modem["status"]["signal-%s" % sig] = int(round(float(v)))
                        elif sig in dec_signals:
                            modem["status"]["signal-%s" % sig] = "%.1f" % float(v)
                        else:
                            modem["status"]["signal-%s" % sig] = v
                        found = True
                    except (TypeError, ValueError):
                        continue
                if found:
                    break

        access_techs = gen.get("access-technologies") or []
        if isinstance(access_techs, str):
            access_techs = [access_techs]
        access_techs = [t for t in access_techs if t and t != "--"]
        # mmcli reports the active access-technology set lowest-to-highest;
        # pick the most advanced for the YANG single-valued enum.
        network_type = access_techs[-1].lower() if access_techs else "unknown"

        modem["status"]["cellular"] = {
            "registration-state":
                venum(gpp["registration-state"],
                      ["idle", "home", "searching", "denied", "roaming"],
                      "unknown"),
            "operator-name": vstr(gpp["operator-name"]),
            "operator-id": vstr(gpp["operator-code"]),
            "network-type": network_type,
            "service-state":
                venum(gpp.get("packet-service-state", "--"),
                      ["attached", "detached"], "unknown"),
        }

        bearers = []
        for bpath in gen["bearers"]:
            output = runcmdj(['mmcli', '-J', '-m', mpath, '-b', bpath])
            if not output:
                continue

            b = output["bearer"]

            bearer = {
                "path": vstr(bpath),
                "connected": vbool(b["status"]["connected"]),
                "ipv4-address": vip4(b["ipv4-config"]["address"]),
                "ipv4-prefix": vnum(b["ipv4-config"]["prefix"]),
                "ipv6-address": vip6(b["ipv6-config"]["address"]),
                "ipv6-prefix": vnum(b["ipv6-config"]["prefix"]),
                "in-bytes": vnumstr(b["stats"]["bytes-rx"]),
                "out-bytes": vnumstr(b["stats"]["bytes-tx"]),
                "total-in-bytes": vnumstr(b["stats"]["total-bytes-rx"]),
                "total-out-bytes": vnumstr(b["stats"]["total-bytes-tx"]),
                "total-duration": vnumstr(b["stats"]["total-duration"])
            }
            if b["status"]["connected"] != "yes":
                reason = vstr(b["status"]["connection-error"]["message"])
                if len(reason) > 0:
                    bearer["connection-failed-reason"] = reason

            iface = vstr(b["status"]["interface"])
            if len(iface) > 0:
                bearer["interface"] = iface

            bearers.append(bearer)

        modem["status"]["bearer"] = bearers

        sim_path = vstr(gen.get("sim", "--"))
        output = None
        if sim_path:
            output = runcmdj(['mmcli', '-J', '-m', mpath, '-i', sim_path])

        if output:
            sim_props = output["sim"]["properties"]
            modem["info"]["imsi"] = vstr(sim_props.get("imsi", "--"))
            modem["info"]["iccid"] = vstr(sim_props.get("iccid", "--"))
            modem["sim-state"] = {
                "name": sim_name,
                "slot": sim_slot,
                "state": _sim_lock_state(vstr(gen.get("unlock-required", "--"))),
                "operator-name": vstr(sim_props.get("operator-name", "--")),
            }
        else:
            # mmcli SIM query failed (or there's no SIM path at all).  Emit
            # sim-state anyway so a 'simN' component appears in the YANG
            # tree — Andrew's report: a missing SIM should be visible from
            # 'show modem', not silent.  Derive the state from modem-level
            # signals: an empty 'sim' path or 'sim-missing' failure reason
            # both mean the slot is empty.  Otherwise we genuinely don't
            # know (modem busy / SIM read error / etc.) — report 'unknown'.
            if (not sim_path or
                vstr(gen.get("state-failed-reason", "--")) == "sim-missing"):
                sim_state_value = "not-inserted"
            else:
                sim_state_value = "unknown"
            modem["sim-state"] = {
                "name": sim_name,
                "slot": sim_slot,
                "state": sim_state_value,
                "operator-name": "",
            }

        output = runcmdj(['mmcli', '-J', '-m', mpath, '--location-get'])
        if output:
            loc = output["modem"]["location"]
            modem["status"]["location"] = {
                "latitude": vstr(loc["gps"]["latitude"]),
                "longitude": vstr(loc["gps"]["longitude"]),
                "altitude": vstr(loc["gps"]["altitude"]),
                "cid": vstr(loc["3gpp"]["cid"]),
                "lac": vstr(loc["3gpp"]["lac"]),
                "mcc": vstr(loc["3gpp"]["mcc"]),
                "mnc": vstr(loc["3gpp"]["mnc"]),
                "tac": vstr(loc["3gpp"]["tac"])
            }

        modems.append(modem)

    modems = sorted(modems, key=lambda m: m['index'])
    data = json.dumps(modems)

    print(data)


def main():
    parser = argparse.ArgumentParser(prog='modem-info')
    parser.add_argument("-i", "--index", default=0, help="Modem index")
    args = parser.parse_args()

    with open("/proc/cmdline", 'r') as fd:
        cmdline = fd.read()
        if "debug" in cmdline:
            debug = True

    if args.index:
        print_modem(int(args.index))
    else:
        print_all()


if __name__ == "__main__":
    main()
