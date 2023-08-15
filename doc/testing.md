Testing
=======

Infix comes with a test suite that is intended to provide end-to-end
verification of supported features.  Generally speaking, this means
that one or more DUTs are configured over NETCONF; the resulting
network is then black-box tested by injecting and inspecting network
traffic at various points.


TL;DR
-----

    make x86_64_defconfig
    make
    make test-qeneth

Runs the test suite on a set of virtual Infix nodes.


Tenets
------

- **Keep overhead to a minimum**.  Tests should be fast to both write
  and run.  Ideally, the developer should _want_ to add tests early in
  the development cycle because they instinctively feel that that is
  the quickest route to arrive at a correct and robust implementation.

- **Both physical and virtual hardware matters**.  Infix is primarily
  deployed on physical hardware, so being able to run the test suite
  on real devices is crucial to guarantee a high quality product.  At
  the same time, there is much value in running the same suite on
  virtual hardware, as it makes it easy to catch regressions early.
  It is also much more practical and economical to build large virtual
  networks than physical ones.

- **Avoid CLI scipting & scraping**.  Reliably interacting with a DUT
  over a serial line in a robust way is _very_ hard to get right.
  Given that we have a proper API (RESTCONF), we should leverage that
  when testing.  Front-ends can be tested by other means.


Architectural Overview
----------------------

![Infix Testing Architecture](testing-overview.svg)

The test system is made up of several independent components, which
are typically used in concert to run a full test suite.

### Test Cases

A test case is an executable, receiving the physical topology as a
positional argument, which produces [TAP][] compliant output on its
`stdout`. I.e., it is executed in the following manner:

    test-case [OPTS] <physical-topology>

Test cases are typically written in Python, using the
[Infamy](#infamy) library.  Ultimately though, it can be implemented
in any language, as long as it matches the calling convention above.

### Infamy

Rather than having each test case come up with its own implementation
of how to map topologies, how to push NETCONF data to a device, etc.,
we provide a library of functions to take care of all that, dubbed
"Infamy".  When adding a new test case, ask yourself if any parts of
it might belong in Infamy as a generalized component that can be
reused by other tests.

Some of the core functions provided by Infamy are:

- Mapping a logical topology to a physical one
- Finding and attaching to a device over an Ethernet interface, using
  NETCONF
- Pushing/pulling NETCONF data to/from a device
- Generating TAP compliant output

### 9PM

To run multiple tests, we employ [9PM][]. It let's us define test
suites as simple YAML files. Suites can also be hierarchically
structured, with a suite being made up of other suites, etc.

It also validates the TAP output, making sure to catch early exits
from a case, and produces a nice summary report.

### `/test/env`

A good way to ensure that nobody ever runs the test suite is to make
it _really_ hard to do so.  `/test/env`'s job is instead to make it
very _easy_ to create a reproducible environment in which tests can be
executed.

Several technologies are leveraged to accomplish this:

- **Containers**: The entire execution is optionally done inside a
  standardized container environment, using either `podman` or
  `docker`.  This ensures that the software needed to run the test
  suite is always available, no matter which distribution the user is
  running on their machine.

- **Python Virtual Environments**: To make sure that the expected
  versions of all Python packages are available, the execution is
  wrapped inside a `venv`.  This is true for containerized executions,
  where the container comes with a pre-installed environment, but it
  can also be sourced from the host system when running outside of the
  container.

- **Virtual Test Topology**: Using [Qeneth][], the environment can
  optionally be started with a virtual topology of DUTs to run the
  tests on.


Interactive Usage
-----------------

Some tests only require a single DUT.  These can therefore be run
against an Infix image started from `make run`. When the instance is
running, you can open a separate terminal and run `make test-run`, to
run the subset of the test suite that can be mapped to it.

Both `test-qeneth` and `test-run` targets have a respective target
with a `-sh` suffix.  These can be used to start an interactive
session in the reproducible environment, which is usually much easier
to work with during a debugging session.

Inside of the reproducible environment, a wrapper for Qeneth is
automatically created that will run it from the running network's
directory.  E.g., running a plain `qeneth status` inside a `make
test-qeneth-sh` environment will show the expected status information.


Physical and Logical Topologies
-------------------------------

Imagine that we want to create a test with three DUTs; one acting as a
DHCP server, and the other two as DHCP clients - with all three having
a management connection to the host PC running the test.  In other
words, the test requires a _logical_ topology like the one below.

<img align="right" src="testing-log.dot.svg" alt="Example Logical Topology">

    graph "dhcp-client-server" {
        host [
            label="host | { <c1> c1 | <srv> srv | <c2> c2 }",
            kind="controller",
        ];

        server [
            label="{ <mgmt> mgmt } | server | { <c1> c1 | <c2> c2 }",
            kind="infix",
        ];
        client1 [
            label="{ <mgmt> mgmt } | client1 | { <srv> srv }",
            kind="infix",
        ];
        client2 [
            label="{ <mgmt> mgmt } | client2 | { <srv> srv }",
            kind="infix",
        ];

        host:srv -- server:mgmt
        host:c1  -- client1:mgmt
        host:c2  -- client2:mgmt

        server:c1 -- client1:srv;
        server:c2 -- client2:srv;
    }

When running in a virtualized environment, one could simply create a
setup that matches the test's logical topology.  But in scenarios when
devices are physical systems, connected by real copper cables, this is
not possible (unless you have some wicked L1 relay matrix thingy).

Instead, the test implementation does not concern itself with the
exact nodes used to run the test, only that the _logical_ topology can
be _mapped_ to some subset of the _physical_ topology.  In
mathematical terms, the physical topology must contain a subgraph that
is _isomorphic_ to the logical topology.

Standing on the shoulders of giants (i.e. people with mathematics
degrees), we can deploy well-known algorithms to find such subgraphs.
Continuing our example, let's say we want to run our DHCP test on the
_physical_ topology below.

<img align="right" src="testing-phy.dot.svg" alt="Example Physical Topology">

    graph "quad-ring" {
        host [
            label="host | { <d1a> d1a | <d1b> d1b | <d1c> d1c | <d2a> d2a | <d2b> d2b | <d2c> d2c | <d3a> d3a | <d3b> d3b | <d3c> d3c | <d4a> d4a | <d4b> d4b | <d4c> d4c }",
            kind="controller",
        ];

        dut1 [
            label="{ <e1> e1 | <e2> e2 | <e3> e3 } | dut1 | { <e4> e4 | <e5> e5 }",
            kind="infix",
        ];
        dut2 [
            label="{ <e1> e1 | <e2> e2 | <e3> e3 } | dut2 | { <e4> e4 | <e5> e5 }",
            kind="infix",
        ];
        dut3 [
            label="{ <e1> e1 | <e2> e2 | <e3> e3 } | dut3 | { <e4> e4 | <e5> e5 }",
            kind="infix",
        ];
        dut4 [
            label="{ <e1> e1 | <e2> e2 | <e3> e3 } | dut4 | { <e4> e4 | <e5> e5 }",
            kind="infix",
        ];

        host:d1a -- dut1:e1
        host:d1b -- dut1:e2
        host:d1c -- dut1:e3

        host:d2a -- dut2:e1
        host:d2b -- dut2:e2
        host:d2c -- dut2:e3

        host:d3a -- dut3:e1
        host:d3b -- dut3:e2
        host:d3c -- dut3:e3

        host:d4a -- dut4:e1
        host:d4b -- dut4:e2
        host:d4c -- dut4:e3

        dut1:e5 -- dut2:e4
        dut2:e5 -- dut3:e4
        dut3:e5 -- dut4:e4
        dut4:e5 -- dut1:e4
    }

Our test (in fact, all tests) receives the physical topology as an
input parameter, and then maps the desired logical topology onto it,
producing a mapping from logical nodes and ports to their physical
counterparts.

    {
      "client1": "dut1",
      "client1:mgmt": "dut1:e1",
      "client1:srv": "dut1:e4",
      "client2": "dut3",
      "client2:mgmt": "dut3:e2",
      "client2:srv": "dut3:e5",
      "host": "host",
      "host:c1": "host:d1a",
      "host:c2": "host:d3b",
      "host:srv": "host:d4c",
      "server": "dut4",
      "server:c1": "dut4:e5",
      "server:c2": "dut4:e4",
      "server:mgmt": "dut4:e3"
    }

With this information, the test knows that, in this particular
environment, the server should be managed via the port called `d4c` on
the node called `host`; that the port connected to the server on
`client1` is `e4` on `dut1`, etc.  Thereby separating the
implementation of the test from any specific physical setup.

Testcases are not required to use a logical topology; they may choose
to accept whatever physical topology its given, and dynamically
determine the DUTs to use for testing.  As an example, an STP test
could accept an arbitrary physical topology, run the STP algorithm on
it offline, enable STP on all DUTs, and then verify that the resulting
spanning tree matches the expected one.


[9PM]:    https://github.com/rical/9pm
[Qeneth]: https://github.com/wkz/qeneth
[TAP]:    https://testanything.org/
