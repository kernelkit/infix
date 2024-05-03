#!/usr/bin/env python3

import socket
import time

import infamy, infamy.neigh

def ll6ping(node):
    neigh = None

    cport, nport = env.ptop.get_mgmt_link(ctrl, node)
    neigh = infamy.neigh.ll6ping(cport, flags=["-w1", "-c1", "-L", "-n"])
    if neigh:
        print(f"Found {neigh} on {cport} (connected to {node}:{nport})")
        return neigh

    return None

def netconf_syn(neigh):
    try:
        ai = socket.getaddrinfo(neigh, 830, 0, 0, socket.SOL_TCP)
        sock = socket.socket(ai[0][0], ai[0][1], 0)
        sock.connect(ai[0][4])
        sock.close()
        print(f"{neigh} answers to TCP connections on port 830 (NETCONF)")
        return True
    except:
        return False

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env()

        ctrl = env.ptop.get_ctrl()
        infixen = env.ptop.get_infixen()

    with test.step(f"Reach {infixen}"):
        timeout = time.time() + 300

        print(f"Waiting a maximum of 5min for {infixen} to come up")

        while infixen and time.time() < timeout:
            time.sleep(1)
            retry = []
            for node in infixen:
                neigh = ll6ping(node)
                if neigh and netconf_syn(neigh):
                    continue

                retry.append(node)

            infixen = retry

        if infixen:
            print(f"Unable to reach {infixen}")
            test.fail()

    test.succeed()
