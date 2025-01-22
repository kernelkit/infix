#!/usr/bin/env python3
"""
Generate ssh key pair

Verify that 'guest' user can fetch data using only the 'public' key
"""

import infamy
import os
import tempfile
from infamy import ssh

USER = "guest"
KEY = {
    "private": """-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAABFwAAAAdzc2gtcn
NhAAAAAwEAAQAAAQEAn3zL3StVQZLsP9dnrXxK/PT4m2tjW0M+2GN6JdiIUZyKI9KKHWaf
2uHNdXNUPo0JGwgyj1geOcN2TEdkPd113mO9qrS87wgSzs3gpF93MzACSy8c55dxF5g50w
FnrRNZ1JgciyzryncNQjxYkvbbrIcHX4+ojn3sdx70MJbVeYv60M9IeyXksBENF5+rAYA8
VpQ+k7cGJeHIdMx6MURwo6nUIoOzTKJ4WhedwTCkWGlEixE98PsynyvB2Ih8a8uiX/nXLb
PT0MsaQohFt0aFOuZ8J6taRu1wHZk/w3hikkKl5HZ5l74GZK0GGfuB1tr3kJR88MBCAITE
7HEvkIjl/wAAA8hbDYq6Ww2KugAAAAdzc2gtcnNhAAABAQCffMvdK1VBkuw/12etfEr89P
iba2NbQz7YY3ol2IhRnIoj0oodZp/a4c11c1Q+jQkbCDKPWB45w3ZMR2Q93XXeY72qtLzv
CBLOzeCkX3czMAJLLxznl3EXmDnTAWetE1nUmByLLOvKdw1CPFiS9tushwdfj6iOfex3Hv
QwltV5i/rQz0h7JeSwEQ0Xn6sBgDxWlD6TtwYl4ch0zHoxRHCjqdQig7NMonhaF53BMKRY
aUSLET3w+zKfK8HYiHxry6Jf+dcts9PQyxpCiEW3RoU65nwnq1pG7XAdmT/DeGKSQqXkdn
mXvgZkrQYZ+4HW2veQlHzwwEIAhMTscS+QiOX/AAAAAwEAAQAAAQAOYsviyc9ZaF7ODWiJ
Mg5zjcdFAaVHLKQlEagJdOQq9GNTguC5cTHXJQoK35nIQKGDIjSpUGn9jN+FVuU4XVsN8d
JAbSgjqYdExzZNrVzLrbdvP7MsQrFNTwpcOaK37mhqcEQW27jzHNUB1f6pVwIOqGlmWcd6
/unO/uhI37omyfCkZT7lU4E+iCGrbIiwmutT2KmUJ6FZOedDl7XbwyUZMqmtjOGbN+0Hc3
ahtA79+C4MaIptGVr10c85WLz/Rh9oN9tDYZRqhU5cARUd9LyEeKF9R7L+kCq4b2zuEqNI
BSYLGIu859f+IEXeA2Hq965tCTMMgwauMAS1If9R+rlRAAAAgHeoqdL7d3FX1iijZWSBua
Bran5mNdcGzf/8SD25WRMUPhsoqHOvtDWuOkop6nAu/9GGigJaBICuDxJz3KN1Hhpuer+g
/x7+Dgwd5mW41hXOTezp2hGO98OwHLJpmohCz1NIZIHz0h6NZbRZ4vozMtYBs1aq1Qv6h2
DLsxLMLY7FAAAAgQDKkDQXxiXr7Bv7UJaYA4BZAg6qPMvW943mOreLl0uO8urE3mt2MnVy
Et1r2dEhxwqsWrDj5rsBVSXrg/8kXYHrqAgldZ3TxEOCU8xVV4RAzxfE0uFwdawZHdsIJL
JWwNnQ8TdWXPZze0lE9R5b2E4EvJilHvD13pdcIN+hyvGE0QAAAIEAyY+JAtJjDS6wW07E
eTVndLHpDiTZO+tCIP+OO45uYA/8O+IMc+wvIsHCpZC7e3bskg6z2gt5B0DwSnf2Q4zXxP
OFXdOBTFO8XcgWKRo9BIPV6BNH9qrx0Z0m5G45rY6SE9c5Ypv0ExjXRZ/iPWLBSarsv1fF
lrH5aNT6hzZKsc8AAAAMcm9vdEBpbmZhbXkwAQIDBAUGBw==
-----END OPENSSH PRIVATE KEY-----
""",
    "public": "AAAAB3NzaC1yc2EAAAADAQABAAABAQCffMvdK1VBkuw/12etfEr89Piba2NbQz7YY3ol2IhRnIoj0oodZp/a4c11c1Q+jQkbCDKPWB45w3ZMR2Q93XXeY72qtLzvCBLOzeCkX3czMAJLLxznl3EXmDnTAWetE1nUmByLLOvKdw1CPFiS9tushwdfj6iOfex3HvQwltV5i/rQz0h7JeSwEQ0Xn6sBgDxWlD6TtwYl4ch0zHoxRHCjqdQig7NMonhaF53BMKRYaUSLET3w+zKfK8HYiHxry6Jf+dcts9PQyxpCiEW3RoU65nwnq1pG7XAdmT/DeGKSQqXkdnmXvgZkrQYZ+4HW2veQlHzwwEIAhMTscS+QiOX/",
}

with infamy.Test() as test:
    with test.step("Connect to the target device"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Create a guest user on the target device"):
        # Upon attachment, target reverts to `test-config`, in which
        # the SSH service is enabled, hence we do not have to
        # explicitly enable it here.
        target.put_config_dict("ietf-system", {
            "system": {
                "authentication": {
                    "user": [
                        {
                            "name": USER,
                            "authorized-key": [
                                {
                                    "name":"guest-ssh-key",
                                    "algorithm": "ssh-rsa",
                                    "key-data": KEY["public"]
                                }
                            ],
                            "infix-system:shell":"bash"
                        }
                    ]
                }
            }
        })

    with test.step("Write private key to a temporary file"):
        with tempfile.NamedTemporaryFile(delete=False, mode='w') as key_file:
            key_file.write(KEY["private"])
            key_file_name = key_file.name

        os.chmod(key_file_name, 0o600)

    with test.step("Verify it is possible to fetch syslog data using public key"):
        address = target.get_mgmt_ip()
        syslog_file = "./syslog_copy"

        ssh.fetch_file(
            remote_user=USER,
            remote_address=address,
            remote_file="/var/log/syslog",
            local_file=syslog_file,
            key_file=key_file_name,
            check=True,
            remove=True
        )

    test.succeed()
