#!/usr/bin/env python3
"""Prune leftovers from earlier runs on all DUTs.

Other tests leave state behind that Infix keeps on purpose and that the
following tests then trip over.  Remove it before running the suites:

- Stray podman containers, which Infix cannot prune itself due to
  limitations in podman.  See Infix issue "Stray containers are not
  pruned": https://github.com/kernelkit/infix/issues/1614
- The DHCP server lease file, which survives reconfiguration and reboot
  so that clients keep their addresses, and is only read by dnsmasq
  while a DHCP server is configured.  Left alone it keeps old, even
  expired, leases in the operational data.
"""
import infamy

with infamy.Test() as test:
    with test.step("Discover topology and attach to available DUTs"):
        env = infamy.Env(False)
        ctrl = env.ptop.get_ctrl()
        duts = {}
        for ix in env.ptop.get_infixen():
            cport, ixport = env.ptop.get_mgmt_link(ctrl, ix)
            print(f"Attaching to {ix}:{ixport} via {ctrl}:{cport}")
            duts[ix] = env.attach(ix, ixport, protocol="ssh", test_reset=False)

    with test.step("Prune stopped containers"):
        for name, tgt in duts.items():
            print(f"{name}: pruning containers")
            rc = tgt.runsh("sudo rm -rf /var/lib/containers")
            print(rc.stdout)
            if rc.returncode != 0:
                test.fail()

    with test.step("Remove stale DHCP server leases"):
        for name, tgt in duts.items():
            print(f"{name}: removing DHCP server leases")
            rc = tgt.runsh("sudo rm -f /var/lib/misc/dnsmasq.leases")
            print(rc.stdout)
            if rc.returncode != 0:
                test.fail()

    test.succeed()
