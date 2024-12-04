#!/usr/bin/env python3
"""
SSH server configuration

Test SSH server functionality with pre-defined key pair:
1. Enable/Disable SSH service.
2. Configure listen address and port.
3. Validate connectivity using static key pair.
"""

import subprocess
import infamy

PRIVATE_KEY = "MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQCtzyoT8/23hSyo7trLaqc6Auj5jvwhhCxUh8WIyfd5G/R9R+/wFEtGo6c6h75/GFCotFCQYvLlqHkrI0QiLYCPo0Rcxzfpy8TZGYjlyD8aYTYeXR2Oow6cjHE3ajQEEPbr5eiV/NBezg00SrCazDN4VHEXcjhl4egaKxDyG1yi98kQISY0+ehNjR/CKOBvOxHqB0N7gUnasbBiiN/iCCkCuDFKnBM6cYrUAvwP/aj+f9dq4lImJ6YpHaSjFKIa2ZFmOi20X0cb0AS1cshjSU2qf9eS2nysbmlC50X8HL9gaIeVInsWTLxEHTrd2gyBCTPO3X6oLJWW7HoB+yA9wp6xAgMBAAECggEACDNXrsaSrFmfFm7jmZikAHmR7LFlfb7W2RupUyeFUrxiDBWscVFBznjK+3jbYPOAnb8ZNIDIqVOKOQHyVWL8d3p6b56yKYik1mHtKrtIl+npg+P8kKXqmvII5vaOsvjqb42izE3X9nsmFhcmjz0uegFQ7yxjUxJGMVLiGyw1khZHFLxAcCzwN2qnxni7MjU2d+ZAtNd4ilSjXZ46Q/6+CyrhoTDUhc+5iqCgXU2wtYWrnvEhCBFd3AYh1vWZuh1TxMgnsfYePk5fHM1AG10XUvI5jOjSkSN+AlJxuXeUSeLyUV4hekem/j/UT3KwVPAiEsBil4KWyneiildXxU5MaQKBgQDm1667T06I/ty6/KZSdfm4EpDKylHohdN8Vr0MfgZSzZgc3bNNMQxAXhGdTIi2keIpitoF+/vLiMhxa9q692XY85eKSOC0Lvv5IRUC9/fUtoKrESOoxwX8SJ3bHj/Xel7Ye0WOXVJcO1/PXO0KFgs2YDRdmQKMFNKS/CdK+2TuCQKBgQDAwEzQ7B3cRYp4R2s230wpSSsPkiJXDQLno82S8K2/vLuWnlwIL3A1833l7PDfp5APABU3EVpQ7EYE6usnO1/HDSZ208uiprx6LIbX0gZVoRnPOKFwRVD7zrYo1n11Lydg8OgKtey5GsruPRbLtAw3/ugayUDCUExXmYlFQLRVaQKBgG+NTrzpiDQfpR8fNGio5jITlrDIsGhDM33klJrS089z1swsPpdQ2nDIhI6VC4PeX4JfvRgjOvySbvqQejTblPYQUOzcZunrwowTdonmtnauc9qi/65x7uyJUu8uYP+J/Qd0Gpq/citr7dLRPyMen/B48RVB+b8j2NZ6z6ombhGxAoGAY2OE+IGX0Bnnkae55/xyKCO7WXcPz/U8lzbGbMs/vEtUKxETAYF8icU5GNL5TUn4pVN0nQWMnYeHf0em437hHyFvwPvq177EFvdYvHZmn8bHKSvZSqvjW0Q2d45J+J/M3Va7P7KZEsV2+Ct10qnPVxxQkGdPxiJjixP3TUdU9WkCgYEAtHa4cwsbgy0HWtNT2smc80jLGFfsX8+/MtgTVdx6zaTybl50hJeVG4kW+7Fvstr78iVl31qPWx14MjoXKTEeVMo6ulrEijnbCx6DgkOwq+EOUvZn0W7ly4RhDDA9W8qdBIAzAGumkCx4456Un3z8wbIVgSZB52IELCBKpbyhSWE="

PUBLIC_KEY = "MIIBCgKCAQEArc8qE/P9t4UsqO7ay2qnOgLo+Y78IYQsVIfFiMn3eRv0fUfv8BRLRqOnOoe+fxhQqLRQkGLy5ah5KyNEIi2Aj6NEXMc36cvE2RmI5cg/GmE2Hl0djqMOnIxxN2o0BBD26+XolfzQXs4NNEqwmswzeFRxF3I4ZeHoGisQ8htcovfJECEmNPnoTY0fwijgbzsR6gdDe4FJ2rGwYojf4ggpArgxSpwTOnGK1AL8D/2o/n/XauJSJiemKR2koxSiGtmRZjottF9HG9AEtXLIY0lNqn/Xktp8rG5pQudF/By/YGiHlSJ7Fky8RB063doMgQkzzt1+qCyVlux6AfsgPcKesQIDAQAB"

SSH_RSA_PUBLIC_KEY="AAAAB3NzaC1yc2EAAAADAQABAAABAQCtzyoT8/23hSyo7trLaqc6Auj5jvwhhCxUh8WIyfd5G/R9R+/wFEtGo6c6h75/GFCotFCQYvLlqHkrI0QiLYCPo0Rcxzfpy8TZGYjlyD8aYTYeXR2Oow6cjHE3ajQEEPbr5eiV/NBezg00SrCazDN4VHEXcjhl4egaKxDyG1yi98kQISY0+ehNjR/CKOBvOxHqB0N7gUnasbBiiN/iCCkCuDFKnBM6cYrUAvwP/aj+f9dq4lImJ6YpHaSjFKIa2ZFmOi20X0cb0AS1cshjSU2qf9eS2nysbmlC50X8HL9gaIeVInsWTLxEHTrd2gyBCTPO3X6oLJWW7HoB+yA9wp6x"

with infamy.Test() as test:
    with test.step("Setup topology and attach to the target"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

        _, data1 = env.ltop.xlate("target", "data1")
        _, data2 = env.ltop.xlate("target", "data2")

    with test.step("Configure SSH server"):
        SSH_PORT_1 = 777
        SSH_ADDRESS_1 = "77.77.77.77"
        SSH_ADDRESS_2 = "88.88.88.88"
        PREFIX_LENGTH = 24

        # Disable SSH before remove hostkey to pass YANG-validation.
        target.put_config_dicts({"infix-services": {
            "ssh": {
                "enabled": False
            }
        }})
        target.delete_xpath("/infix-services:ssh/hostkey[name()='genkey']")
        target.put_config_dicts({
            "ietf-keystore": {
                "keystore": {
                    "asymmetric-keys": {
                        "asymmetric-key": [
                            {
                                "name": "test-host-key",
                                "public-key-format": "ietf-crypto-types:ssh-public-key-format",
                                "public-key": PUBLIC_KEY,
                                "private-key-format": "ietf-crypto-types:rsa-private-key-format",
                                "cleartext-private-key": PRIVATE_KEY
                            }
                        ]
                    }
                }
            },
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                        {
                            "name": data1,
                            "ipv4": {
                                "address": [
                                    {
                                        "ip": SSH_ADDRESS_1,
                                        "prefix-length": PREFIX_LENGTH
                                    }
                                ]
                            }
                        },
                        {
                            "name": data2,
                            "ipv4": {
                                "address": [
                                    {
                                        "ip": SSH_ADDRESS_2,
                                        "prefix-length": PREFIX_LENGTH
                                    }
                                ]
                            }
                        }
                    ]
                }
            },
            "infix-services": {
                "ssh": {
                    "enabled": True,
                    "hostkey": [
                        "test-host-key"
                    ],
                    "listen": [
                    {
                        "name": "test-listener-1",
                        "address": SSH_ADDRESS_1,
                        "port": SSH_PORT_1
                    }
                ]
            }
        }
    })

    with test.step("Verify SSH public keys"):
        _, hport1 = env.ltop.xlate("host", "data1")
        _, hport2 = env.ltop.xlate("host", "data2")

        with infamy.IsolatedMacVlan(hport1) as ns77:
            ns77.addip("77.77.77.70", prefix_length=24)
            ns77.must_reach(SSH_ADDRESS_1)
            ssh_scan_result = ns77.runsh(f"ssh-keyscan -p {SSH_PORT_1} {SSH_ADDRESS_1}")
            lines = [
                line for line in ssh_scan_result.stdout.splitlines()
                if line.strip().startswith(f"[{SSH_ADDRESS_1}]:{SSH_PORT_1}")
            ]
            assert len(lines) == 1, f"Unexpected ssh-keyscan output: {ssh_scan_result.stdout}"
            target_public_key = lines[0].split()[2]

            assert target_public_key.strip() == SSH_RSA_PUBLIC_KEY.strip(), "Public key mismatch"

            print("Public key verified successfully.")

    with test.step("Verify it is not possible to access SSH on other IP address"):
        with infamy.IsolatedMacVlan(hport2) as ns88:
            ns88.addip("88.88.88.80", prefix_length=24)
            ns88.must_reach(SSH_ADDRESS_2)

            assert ns88.runsh(f"ssh-keyscan -p {SSH_PORT_1} {SSH_ADDRESS_2}").returncode == 1, "SSH is accessable on wrong interface"

    with test.step("Disable SSH server"):
        target.put_config_dict("infix-services", {
            "ssh": {
                "enabled": False
            }
        })
        assert(ns77.run(
            f"ssh-keyscan -p {SSH_PORT_1} {SSH_ADDRESS_1}",
            shell=True,
            text=True,
            stdout=subprocess.PIPE,
        ).returncode == 1)

    test.succeed()
