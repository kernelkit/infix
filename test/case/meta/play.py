#!/usr/bin/env -S ipython3 -i

import json

import infamy
from infamy.util import parallel

def jq(yangdata):
    print(json.dumps(yangdata, indent=True))

env = infamy.Env()

ctrl = env.ptop.get_ctrl()
infixen = env.ptop.get_infixen()

def attach(ix):
    cport, ixport = env.ptop.get_mgmt_link(ctrl, ix)
    print(f"Attaching to {ix}:{ixport} via {ctrl}:{cport}")
    return env.attach(ix, ixport)

globals().update(zip(infixen, parallel(*(lambda ix=ix: attach(ix)
                                         for ix in infixen))))

print("\nGLHF")
