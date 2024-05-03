#!/usr/bin/env -S ipython3 -i

import json

import infamy

def jq(yangdata):
    print(json.dumps(yangdata, indent=True))

env = infamy.Env()

ctrl = env.ptop.get_ctrl()
infixen = env.ptop.get_infixen()
for ix in infixen:
    cport, ixport = env.ptop.get_mgmt_link(ctrl, ix)
    print(f"Attaching to {ix}:{ixport} via {ctrl}:{cport}")
    exec(f"{ix} = env.attach(\"{ix}\", \"{ixport}\")")

print("\nGLHF")
