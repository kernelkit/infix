#!/usr/bin/env python3
r"""Basic Firewall Container

Verify that an nftables container can be used for IP masquerading and
port forwarding to another container running a basic web server.

....
                                                    <--- Docker containers --->
.-------------.            .----------------------. .--------..---------------.
|      | mgmt |------------| mgmt |        |      | |  fire  ||      |  web   |
| host | data |------------| ext0 | target | int0 | |  wall  || eth0 | server |
'-------------'.42       .1'----------------------' '--------''---------------'
                                              \ .1             .2 /
              192.168.0.0/24                   \   10.0.0.0/24   /
                                                `-- VETH pair --'
....

The web server container is connected to the target on an internal
network, using a VETH pair, serving HTTP on port 91.

The firewall container sets up a port forward with IP masquerding
to/from `ext0:8080` to 10.0.0.2:91.

Correct operation is verified using HTTP GET requests for internal port
91 and external port 8080, to ensure the web page, with a known key
phrase, is only reachable from the public interface `ext0`, on
192.168.0.1:8080.

"""
import infamy
from infamy.util import until, to_binary


with infamy.Test() as test:
    NFTABLES = f"oci-archive:{infamy.Container.NFTABLES_IMAGE}"
    HTTPD = f"oci-archive:{infamy.Container.HTTPD_IMAGE}"
    WEBIP = "10.0.0.2"
    INTIP = "10.0.0.1"
    EXTIP = "192.168.0.1"
    OURIP = "192.168.0.42"
    WEBNM = "web"
    NFTNM = "firewall"
    GOOD_URL = f"http://{EXTIP}:8080/index.html"
    BAD_URL = f"http://{EXTIP}:91/index.html"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, ext0 = env.ltop.xlate("target", "ext0")
        _, hport = env.ltop.xlate("host", "data")
        addr = target.get_mgmt_ip()

        if not target.has_model("infix-containers"):
            test.skip()

    with test.step("Set hostname to 'container-host'"):
        target.put_config_dict("ietf-system", {
            "system": {
                "hostname": "container-host"
                }
            })

    with test.step("Create VETH pair for web server container"):
        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": f"{ext0}",
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": f"{EXTIP}",
                                "prefix-length": 24
                            }]
                        }
                    },
                    {
                        "name": "int0",
                        "type": "infix-if-type:veth",
                        "enabled": True,
                        "infix-interfaces:veth": {
                            "peer": f"{WEBNM}"
                        },
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": f"{INTIP}",
                                "prefix-length": 24,
                            }]
                        }
                    },
                    {
                        "name": f"{WEBNM}",
                        "type": "infix-if-type:veth",
                        "enabled": True,
                        "infix-interfaces:veth": {
                            "peer": "int0"
                        },
                        "ipv4": {
                            "address": [{
                                "ip": f"{WEBIP}",
                                "prefix-length": 24,
                            }]
                        },
                        "container-network": {}
                    },
                ]
            }
        })

    with test.step("Create firewall container from bundled OCI image"):
        # Store the nftables .conf file contents as a multi-line string
        config = to_binary(f"""#!/usr/sbin/nft -f

flush ruleset

define WAN = "{ext0}"
define INT = "int0"
define WIP = "{WEBIP}"
                          """
                          """

table ip nat {
    chain prerouting {
        type nat hook prerouting priority 0; policy accept;
        iifname $WAN tcp dport 8080 dnat to $WIP:91
    }

    chain postrouting {
        type nat hook postrouting priority 100; policy accept;
        oifname $WAN masquerade
        oifname $INT masquerade
    }
}
""")

        target.put_config_dict("infix-containers", {
            "containers": {
                "container": [
                    {
                        "name": f"{NFTNM}",
                        "image": f"{NFTABLES}",
                        "network": {
                            "host": True
                        },
                        "mount": [
                          {
                            "name": "nftables.conf",
                            "content": config,
                            "target": "/etc/nftables.conf"
                          }
                        ],
                        "privileged": True
                    }
                ]
            }
        })

    with test.step("Create web server container from bundled OCI image"):
        target.put_config_dict("infix-containers", {
            "containers": {
                "container": [
                    {
                        "name": f"{WEBNM}",
                        "image": f"{HTTPD}",
                        "command": "/usr/sbin/httpd -f -v -p 91",
                        "network": {
                            "interface": [
                                {"name": f"{WEBNM}"}
                            ]
                        }
                    }
                ]
            }
        })

    with test.step("Verify firewall container has started"):
        c = infamy.Container(target)
        until(lambda: c.running(NFTNM), attempts=10)

    with test.step("Verify web container has started"):
        c = infamy.Container(target)
        until(lambda: c.running(WEBNM), attempts=10)

    with infamy.IsolatedMacVlan(hport) as ns:
        NEEDLE = "tiny web server from the curiOS docker"
        ns.addip(OURIP)
        with test.step("Verify connectivity, host can reach target:ext0"):
            ns.must_reach(EXTIP)
        with test.step("Verify 'web' is NOT reachable on http://container-host.local:91"):
            url = infamy.Furl(BAD_URL)
            until(lambda: not url.nscheck(ns, NEEDLE))
        with test.step("Verify 'web' is reachable on http://container-host.local:8080"):
            url = infamy.Furl(GOOD_URL)
            until(lambda: url.nscheck(ns, NEEDLE))

    test.succeed()
