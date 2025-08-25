#!/usr/bin/env python3

import time
import infamy
from infamy.neigh import ll6ping
from infamy.util import until, is_reachable

TIMEOUT = 300

def infixen_reachable(node, ctrl, env):
    cport, _ = env.ptop.get_mgmt_link(ctrl, node)
    ip = ll6ping(cport)
    if not ip:
        return False

    return is_reachable(ip, env, env.ptop.get_password(node))

with infamy.Test() as test:
    with test.step("Initialize"):
        # The test is designed to be run on a physical topology.
        env = infamy.Env(ltop=False)
        ctrl = env.ptop.get_ctrl()
        infixen = env.ptop.get_infixen()
        env.args.transport = None # To wait for RESTCONF and NETCONF

    with test.step(f"Reach {infixen}"):
        print(f"Waiting for {infixen} to come up, timeout: {TIMEOUT / 60} min")
        timeout = time.time() + TIMEOUT

        while infixen and time.time() < timeout:
            time.sleep(1)
            infixen = [node for node in infixen if not infixen_reachable(node, ctrl, env)]

        if infixen:
            print(f"Unable to reach {infixen}")
            test.fail()

    test.succeed()
