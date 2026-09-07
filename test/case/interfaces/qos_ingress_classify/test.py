#!/usr/bin/env python3
"""
QoS Ingress Classification and Egress Remarking

Configure the trust order, default priority, the standard PCP and DSCP
presets, and remarking on transmit, then verify the rendering:

 - trust dscp-pcp, DSCP first then PCP
 - default priority 2
 - PCP preset ieee: PCP n to priority n
 - DSCP preset ietf: RFC 4594 groups, e.g. EF (46) to priority 5
 - remark pcp and dscp from priority: priority 5 to PCP 5 and DSCP 40

On a port whose driver has DCB operations the kernel's DCB tables must
match, and the operational datastore reports classification and
remarking as offloaded.  On any other port the same classification must
be rendered as tc flower rules on the port's ingress, in trust order,
with a catch-all for the default priority, and DSCP remarking as pedit
rules on its egress; PCP remarking needs driver support.
Removing the configuration returns the port to the defaults: trust pcp
with default priority 0, and no remarking.
"""
import json
import re
import infamy
from infamy.util import until


def capabilities(target, port):
    data = target.get_data(f"/ietf-interfaces:interfaces/interface[name='{port}']"
                           "/infix-interfaces:qos/capabilities")
    for iface in data["interfaces"]["interface"]:
        qos = iface.get("qos") or iface.get("infix-interfaces:qos") or {}
        return qos.get("capabilities", {})
    return {}


def dscp_num(name):
    """dcb prints DSCP by name when it knows one: CS1, AF21, EF ..."""
    if name.startswith("CS"):
        return int(name[2:]) * 8
    if name.startswith("AF"):
        return int(name[2]) * 8 + int(name[3]) * 2
    if name == "EF":
        return 46
    return int(name)


def dcb_tokens(ssh, cmd, port):
    """Return {table: [tokens]} from dcb ... show dev PORT text output, DSCP as numbers"""
    out = ssh.runsh(f"dcb {cmd} show dev {port}").stdout
    tables = {}
    for line in out.splitlines():
        name, _, rest = line.partition(" ")
        name = name.rstrip(":")
        if not name:
            continue
        tokens = rest.split()
        if name == "dscp-prio":
            tokens = [f"{dscp_num(k)}:{v}" for k, v in (t.split(":") for t in tokens)]
        elif name == "prio-dscp":
            tokens = [f"{k}:{dscp_num(v)}" for k, v in (t.split(":") for t in tokens)]
        tables[name] = tokens
    return tables


def skbedit_priority(act):
    """tc prints the priority as a classid: 'none' for 0, else e.g. ':5' in hex"""
    prio = str(act["priority"])
    if prio == "none":
        return 0
    return int(prio.rsplit(":", 1)[-1] or "0", 16)


def flower_rules(ssh, port):
    """Return [(pref, keys, priority)] for skbedit filters on the port's ingress"""
    out = ssh.runsh(f"tc -j filter show dev {port} ingress").stdout
    rules = []
    for flt in json.loads(out or "[]"):
        opts = flt.get("options")
        if not opts:
            continue
        for act in opts.get("actions", []):
            if act.get("kind") == "skbedit" and "priority" in act:
                rules.append((flt["pref"], opts.get("keys", {}), skbedit_priority(act)))
    return rules


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")
        _, port = env.ltop.xlate("target", "data")
        dcb = bool(capabilities(target, port).get("supported-trust-order"))
        print(f"{port}: DCB {'supported' if dcb else 'not supported'}")

    with test.step("Configure trust dscp-pcp, default priority 2, presets, and remarking"):
        target.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": port,
                    "enabled": True,
                    "infix-interfaces:qos": {
                        "ingress": {
                            "trust": "dscp-pcp",
                            "default-priority": 2,
                            "pcp-map": {"preset": "ieee"},
                            "dscp-map": {"preset": "ietf"},
                        },
                        "egress": {
                            "remark": {"pcp": "from-priority", "dscp": "from-priority"}
                        }
                    }
                }]
            }
        }})

    if dcb:
        with test.step("Verify dcb apptrust order is dscp pcp"):
            until(lambda: dcb_tokens(tgtssh, "apptrust", port).get("order") == ["dscp", "pcp"])

        with test.step("Verify dcb app default priority and PCP and DSCP maps"):
            app = dcb_tokens(tgtssh, "app", port)
            print(app)
            assert app.get("default-prio") == ["2"], f"default-prio {app.get('default-prio')}"
            pcp = set(app.get("pcp-prio", []))
            for want in ("0nd:0", "1nd:1", "0de:0", "1de:1", "7nd:7", "7de:7"):
                assert want in pcp, f"missing {want} in pcp-prio {sorted(pcp)}"
            dscp = set(app.get("dscp-prio", []))
            for want in ("0:0", "8:1", "26:3", "46:5", "48:6", "56:7"):
                assert want in dscp, f"missing {want} in dscp-prio {sorted(dscp)}"

        with test.step("Verify dcb rewr priority to PCP and DSCP maps"):
            rewr = dcb_tokens(tgtssh, "rewr", port)
            print(rewr)
            pcp = set(rewr.get("prio-pcp", []))
            for want in ("0:0nd", "5:5nd", "7:7nd"):
                assert want in pcp, f"missing {want} in prio-pcp {sorted(pcp)}"
            dscp = set(rewr.get("prio-dscp", []))
            for want in ("0:0", "1:8", "5:40", "7:56"):
                assert want in dscp, f"missing {want} in prio-dscp {sorted(dscp)}"

        with test.step("Verify classification and remarking are reported as offloaded"):
            until(lambda: {"classification", "remarking"} <=
                  set(capabilities(target, port).get("offload", [])))
    else:
        with test.step("Verify tc flower rules: DSCP block before PCP block, then default"):
            until(lambda: len(flower_rules(tgtssh, port)) > 0)
            rules = flower_rules(tgtssh, port)
            print(f"{len(rules)} skbedit rules")

            # ip_tos is printed as value/mask, e.g. 0xb8/0xfc; key on the DSCP
            dscp = {}
            for r in rules:
                if "ip_tos" in r[1]:
                    tos = int(str(r[1]["ip_tos"]).split("/")[0], 0)
                    dscp.setdefault(tos >> 2, set()).add(r[2])
            pcp = {r[1]["vlan_prio"]: r for r in rules if "vlan_prio" in r[1]}
            dflt = [r for r in rules if not r[1]]
            print(f"dscp {dscp}\npcp {pcp}\ndefault {dflt}")

            assert dscp.get(46) == {5}, f"EF: {dscp.get(46)}"
            assert dscp.get(8) == {1}, f"CS1: {dscp.get(8)}"
            assert all(pcp[p][2] == p for p in range(8)), pcp
            assert dflt and dflt[0][2] == 2, dflt
            # four variants per codepoint: IPv4, IPv6, tagged IPv4, tagged IPv6
            assert sum(1 for r in rules if "ip_tos" in r[1]) == 4 * len(dscp), len(rules)

            dscp_pref = {r[0] for r in rules if "ip_tos" in r[1]}
            pcp_pref = {r[0] for r in pcp.values()}
            assert max(dscp_pref) < min(pcp_pref) < dflt[0][0], (dscp_pref, pcp_pref, dflt)

        with test.step("Verify DSCP remarking as pedit rules on egress"):
            # tc -j is not valid JSON for basic filters with ematches, so count in text
            out = tgtssh.runsh(f"tc filter show dev {port} egress").stdout
            pedits = [l for l in out.splitlines() if re.match(r"\s*action order \d+:\s+pedit", l)]
            assert len(pedits) == 32, f"{len(pedits)} pedit rules"

        with test.step("Verify classification is not reported as offloaded"):
            assert "classification" not in capabilities(target, port).get("offload", [])

    with test.step("Remove qos configuration and verify the defaults are back"):
        target.delete_xpath(f"/ietf-interfaces:interfaces/interface[name='{port}']"
                            "/infix-interfaces:qos")
        if dcb:
            until(lambda: dcb_tokens(tgtssh, "app", port).get("default-prio") == ["0"])
            assert dcb_tokens(tgtssh, "apptrust", port).get("order") == ["pcp"]
            assert not dcb_tokens(tgtssh, "rewr", port).get("prio-dscp")
        else:
            until(lambda: [r for r in flower_rules(tgtssh, port) if not r[1]] == [(900, {}, 0)])
            assert not tgtssh.runsh(f"tc filter show dev {port} egress").stdout.strip()

    test.succeed()
