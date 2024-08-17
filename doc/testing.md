Regression Testing with Infamy
==============================

Infix comes with a test suite that is intended to provide end-to-end
verification of supported features.  Generally speaking, this means
that one or more DUTs are configured over NETCONF; the resulting
network is then black-box tested by injecting and inspecting network
traffic at various points.

This document is intended to be a practical guide on how to run,
develop and debug tests. There is a separate document describing the
[Test System Architecture](test-arch.md).


Modes of Testing
----------------

### Virtual Devices

By default, tests are run on a topology made up of virtual Infix nodes
using [Qeneth][]. To run the full regression test suite: build Infix
for `x86_64`:

    $ make x86_64_defconfig
    $ make
    $ make test

### Physical Devices

To run the tests on a preexisting topology from the host's network
namespace, specify the `host` `TEST_MODE`:

    $ make TEST_MODE=host test

This typically used when testing on physical hardware. By default the
topology will be sourced from `/etc/infamy.dot`, but this can be
overwritten by setting the `TOPOLOGY` variable:

    $ make TEST_MODE=host TOPOLOGY=~/my-topology.dot test


### `make run` Devices

Some tests only require a single DUT.  These can therefore be run
against an Infix image started from `make run`. This requires that the
instance is configured to use TAP networking.

When the instance is running, you can open a separate terminal and run
the subset of the test suite that can be mapped to it:

    $ make TEST_MODE=run test


Interactive Usage
-----------------

When developing and debugging tests, the overhead of repeatedly
setting up and tearing down the test environment can quickly start to
weigh you down. In these situation, you can start an interactive test
environment:

    $ make test-sh
    Info: Generating topology
    Info: Generating node YAML
    Info: Generating executables
    Info: Launching dut1
    Info: Launching dut2
    Info: Launching dut3
    Info: Launching dut4
    11:42:52 infamy0:test #

The example above uses the default mode (`qeneth`), but the `host` and
`run` modes also support the `test-sh` target.

It takes a little while to start up, but then we have a shell prompt
inside the container running Infamy.  It's a very limited environment,
but it has enough to easily run single tests, connect to the virtual
devices, and *step* your code.


### Running Subsets of Tests

Each test case is a separate executable, which can be run without
arguments:

    11:42:53 infamy0:test # ./case/infix_dhcp/dhcp_basic.py

To run a suite of tests, e.g., only the DHCP client tests, pass the
suite as an argument to [9PM][]:

    11:42:53 infamy0:test # ./9pm/9pm.py case/infix_dhcp/all.yaml


### Connecting to Infamy

The test system runs in a Docker container, so to get a shell prompt in
*another terminal* you need to connect to that container.  Infamy comes
with a helper script for this:

    $ ./test/shell
    11:42:53 infamy0:test #

By default it connect to the latest started Infamy instance.  If you for
some reason run multiple instances of Infamy the `shell` script takes an
optional argument "system", which is the hostname of the container you
want to connect to:

    $ ./test/shell infamy2
    11:42:53 infamy2:test #


### Connecting to a DUT

All DUTs in a virtual Infamy topology are emulated in Qemu instances
managed by qeneth.  If you want to watch what's happening on one of the
target systems, e.g., tail a log file or run `tcpdump` during the test,
there is another helper script in Infamy for this:

    $ ./test/console 1
    Trying 127.0.0.1...
    Connected to 127.0.0.1.

    Infix -- a Network Operating System v23.11.0-226-g0c144da (console)
    infix-00-00-00 login: admin
    Password:
    .-------.
    |  . .  | Infix -- a Network Operating System
    |-. v .-| https://kernelkit.github.io
    '-'---'-'

    Run the command 'cli' for interactive OAM

    admin@infix-00-00-00:~$

From here we can observe `dut1` freely while running tests.

> The `console` script uses `telnet` to connect to a port forwarded to
> `localhost` by the Docker container.  To exit Telnet, use Ctrl-] and
> then 'q' followed by enter.  This can be customized in `~/.telnetrc`

Like the `shell` script, `console` takes an optional "system" argument
in case you run multiple instances of Infamy:

    $ ./test/console 1 infamy2

You can also connect to the console of a DUT from within a `shell`:

    $ ./test/shell
    11:42:54 infamy0:test # qeneth status
    11:42:54 infamy0:test # qeneth console dut1
    login: admin
    password: *****
    admin@infix-00-00-00:~$

> **Note:** disconnect from the qeneth console by using bringing up the
> old Telnet "menu" using Ctrl-], compared to standard Telnet this is
> the BusyBox version so you press 'e' + enter instead of 'q' to quit.


### Attaching to MACVLANs

To fully isolate the host's interfaces from one another, many tests
will stack a MACVLAN on an interface, which is then placed in a
separate network namespace.

It is often useful to attach to those namespaces, so that you can
interactively inject traffic into the test setup.

First, connect to the Infamy instance in question:

    $ ./test/shell
    11:43:19 infamy0:test #

Then, attach to the MACVLAN namespace by running the `nsenter` helper
script from the test directory, supplying the base interface name as
the first argument:

    11:43:19 infamy0:test # ./nsenter d1b
    11:43:20 infamy0(d1b):test #

By default, an interactive shell is started, but you can also supply
another command:

    11:43:19 infamy0:test # ./nsenter d1b ip -br addr
    lo               UNKNOWN        127.0.0.1/8 ::1/128
    iface@if7        UP             10.0.0.1/24 fe80::38d0:88ff:fe77:b7cd/64

You can now freely debug the network activity of your test and the
responses from the DUT.


### Using the Python Debugger

The built in `breakpoint()` function in Python is very useful when you
want to run a test case to a certain point at which you might want to
interactively inspect either the test's or the device's state.

Simply insert a call to `breakpoint()` at the point of interest in
your test and run it as normal. Once Python executes the call, it will
drop you into the Python debugger:

    11:42:58 infamy0:test # ./case/infix_dhcp/dhcp_basic.py
    # Starting (2024-02-10 11:42:59)
    # Probing dut1 on port d1a for IPv6LL mgmt address ...
    # Connecting to mgmt IP fe80::ff:fe00:0%d1a:830 ...
    ok 1 - Initialize
    > /home/jocke/src/infix/test/case/infix_dhcp/dhcp_basic.py(44)<module>()
    (Pdb)

At this point you have full access to the test's state, but it is also
an opportunity to inspect the state of the DUTs (e.g. via their
console or over SSH).

It is also possible to run a test under Pdb from the get-go, if you
want to setup breakpoints without modifying the source, or simply step
through the code:

    11:42:58 infamy0:test # python -m pdb case/infix_dhcp/dhcp_basic.py


### Deterministic Topology Mappings

By default, mappings from logical to physical topologies are not
stable across test case executions. This can be very frustrating when
debugging a failing test, since logical nodes are suffled around
between phyical nodes. In such cases, supplying a `PYTHONHASHSEED`
variable (set to any 32-bit unsigned integer) when launching the test
environment will make sure that topology mappings are deterministic:

    $ make PYTHONHASHSEED=0 test-sh

If a seed is not supplied, a random value is chosen. This seed is
logged by the `meta/reproducible.py` test case when running a test
suite:

    $ make test
    Info: Generating topology
    Info: Generating node YAML
    Info: Generating executables
    Info: Launching dut1
    Info: Launching dut2
    Info: Launching dut3
    Info: Launching dut4
    9PM - Simplicity is the ultimate sophistication

    Starting test 0002-reproducible.py
    2024-05-03 10:40:30 # Starting (2024-05-03 10:40:30)
    2024-05-03 10:40:30 # Specify PYTHONHASHSEED=3773822171 to reproduce this test environment
    2024-05-03 10:40:30 ok 1 - $PYTHONHASHSEED is set
    2024-05-03 10:40:30 # Exiting (2024-05-03 10:40:30)
    2024-05-03 10:40:30 1..1
    ...

This is useful because this value can then be used to rerun a test (or
the whole suite) with identical topology mappings:

    $ make PYTHONHASHSEED=3773822171 INFIX_TESTS=case/ietf_system/hostname.py test

### Deterministic Transport Protocol

By default, the communication transport protocol (NETCONF/RESTCONF) is
chosen randomly.  If you supply a `PYTHONHASHSEED` as described above,
you get the same protocol used for that hash.  But if you want to choose
the protocol, add extra arguments to Infamy:

    $ make INFAMY_EXTRA_ARGS="--transport=restconf" INFIX_TESTS=case/ietf_system/hostname.py test

or, when running interactively:

    $ make test-sh
    09:08:17 infamy0:test # ./9pm/9pm.py -o "--transport=restconf" case/ietf_system/hostname.py


[9PM]:    https://github.com/rical/9pm
[Qeneth]: https://github.com/wkz/qeneth
