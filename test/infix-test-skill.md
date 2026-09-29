# Infix test skill

Info about the Infix regression tests.

- Tests live under `test/case/`, are written in Python and use the infamy
  framework in `test/infamy/`.
- A test is a chain of `test.step()` blocks that configure one or more DUTs
  over NETCONF or RESTCONF and verify the result with real traffic. The step
  text in the failing TAP line is where to start reading.
- DUTs run Infix, either as virtual machines (Qeneth, `test/virt/`) or as real
  hardware. The fault is often in the product rather than the test. Device code
  is in `src/`, YANG models in `src/confd/yang/`.
- Each test has a logical `topology.dot` with names like `target:data`. Infamy
  maps it onto the physical topology the test was given, honouring `requires`
  and `provides` attributes, and prints the mapping in the log. A test is
  skipped when no mapping fits.
- Logs: `test/.log/<log id>/output/`. Full guide: `doc/testing.md`.

## Debugging a local QEMU run

- `make test-sh` keeps the DUTs and the `infamy0` container running after
  a failure, `make test` tears them down. The DUTs are QEMU guests with
  384 MB RAM, one host tap per port (`d2a` is port a of dut2, and so on).
- The log maps the test's logical names to DUTs (`R1: dut2`) and prints
  the mgmt address it connected to, e.g. `fe80::2a0:85ff:fe00:201%d2a`.
- Run commands on a DUT over SSH from inside the container, admin/admin:

      podman exec infamy0 sshpass -p admin ssh -o StrictHostKeyChecking=no \
          -o UserKnownHostsFile=/dev/null admin@fe80::2a0:85ff:fe00:201%d2a \
          'vtysh -c "show ip ospf neighbor"'

  `admin` can `sudo -n` and is in `frrvty`, so `vtysh`, `/var/log/messages`,
  `dmesg`, `initctl status` and `sysrepocfg -X -d operational -x <xpath>`
  are all reachable this way. `test/console dut2` attaches to the serial
  console instead. The system is Finit and sysklogd, there is no journal.
- Run a subset: list the tests in a yaml under `test/case/` with `case:`
  paths relative to that directory, then
  `make test TESTS=$PWD/test/case/subset.yaml`. Repeat an entry under
  different names to loop a flaky test. `INFAMY_ARGS=--transport=netconf`
  (or `restconf`) forces the transport, otherwise it is picked per run.
