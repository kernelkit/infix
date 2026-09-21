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
