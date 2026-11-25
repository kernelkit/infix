#!/usr/bin/env python3
"""Container Restart Policy

Verify that a container that exits is started again as its restart
policy says.  Three containers without an image run a command of the
system that exits after a second: /bin/sleep 1.

 - 'never' is started once
 - 'retry' is started again 10 times, and then no more
 - 'always' is still being started again after that

A container that is no longer started again stays that way when other
configuration changes, until the start action is called.
"""
import time
import infamy
from infamy.util import until

with infamy.Test() as test:
    def starts(name):
        out = tgtssh.runsh(f"sudo grep -ac 'Starting container:{name}\\[' /var/log/syslog").stdout
        return int(out.strip() or 0)

    def status(name):
        return infamy.Container(target)._find(name).get("status", "")

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target, tgtssh = env.attach("target", "mgmt"), env.attach("target", "mgmt", "ssh")

        if not target.has_model("infix-containers"):
            test.skip()

    with test.step("Create containers 'never', 'retry' and 'always', running /bin/sleep 1"):
        target.put_config_dicts({
            "infix-containers": {
                "containers": {
                    "container": [{
                        "name": policy,
                        "command": "/bin/sleep 1",
                        "restart-policy": policy,
                        "network": {
                            "host": True
                        }
                    } for policy in ("never", "retry", "always")]
                }
            }
        })

    with test.step("Verify container 'retry' is started 11 times, then has status 'exited (0)'"):
        until(lambda: starts("retry") == 11, attempts=120)
        until(lambda: status("retry") == "exited (0)", attempts=30)

    with test.step("Verify container 'never' was started 1 time, and has status 'exited (0)'"):
        assert starts("never") == 1, f"started {starts('never')} times"
        assert status("never") == "exited (0)", f"status {status('never')}"

    with test.step("Set hostname to 'changed'"):
        target.put_config_dicts({
            "ietf-system": {
                "system": {
                    "hostname": "changed"
                }
            }
        })

    with test.step("Verify container 'always' is started 3 more times"):
        count = starts("always")
        until(lambda: starts("always") >= count + 3, attempts=60)

    with test.step("Verify container 'never' is still started 1 time, and 'retry' 11 times"):
        assert starts("never") == 1, f"'never' started {starts('never')} times"
        assert starts("retry") == 11, f"'retry' started {starts('retry')} times"

    with test.step("Verify the start action starts container 'never' a 2nd time"):
        infamy.Container(target).action("never", "start")
        until(lambda: starts("never") == 2, attempts=30)
        time.sleep(10)
        assert starts("never") == 2, f"started {starts('never')} times"

    test.succeed()
