#!/usr/bin/env python3

import time
import infamy
import infamy.neigh


TIMEOUT = 300


def ll6ping(node):
    cport, nport = env.ptop.get_mgmt_link(ctrl, node)
    neigh = infamy.neigh.ll6ping(cport, flags=["-w1", "-c1", "-L", "-n"])
    if neigh:
        print(f"Found {neigh} on {cport} (connected to {node}:{nport})")
        return neigh

    return None


def is_reachable(node, env):
    neigh = ll6ping(node)
    if not neigh:
        return False

    return infamy.util.is_reachable(neigh, env, env.ptop.get_password(node))


with infamy.Test() as test:
    with test.step("Initialize"):
        # The test is designed to be run on a physical topology.
        env = infamy.Env(ltop=False)

        ctrl = env.ptop.get_ctrl()
        infixen = env.ptop.get_infixen()

    with test.step(f"Reach {infixen}"):
        print(f"Waiting for {infixen} to come up, timeout: {TIMEOUT / 60} min")
        timeout = time.time() + TIMEOUT

        while infixen and time.time() < timeout:
            time.sleep(1)
            infixen = [node for node in infixen if not is_reachable(node, env)]

        if infixen:
            print(f"Unable to reach {infixen}")
            test.fail()

    test.succeed()
