## Network Traffic Inspection

`tcpdump` is useful tool for analyzing and diagnosing network problems.
This document presents the limited feature set that exposed is in the
CLI.  Administrator level users with shell access can use the full
feature set, and is not described here.

The following section is useful for understanding how to use the tool.
A section called [Examples](#examples) follows that, which may be what
you want to scroll down to.


### Hardware Overview

Using `tcpdump` effectively requires an understanding of how the
underlying hardware works.  For a standard PC, or common single-board
computers (SBC), the network cards (NICs) are usually connected directly
to the CPU.

                          .---------.
                          |         +-- eth0
                          |   CPU   |
                          |         +-- eth1
                          '---------'

In this setup it is evident that traffic coming in on either eth0 or
eth1 reach the CPU, i.e., running `tcpdump eth0` captures all traffic.

However, on other types of networking hardware, e.g., dedicated switch
core setups depicted below, the flow of network traffic will likely
*not* pass through the CPU.  This depends of course on how the switch is
set up, for instance if routing between all ports is enabled, each flow
will reach the CPU, but in a plain switching setup it will not.

                          .---------.
                          |         |
                          |   CPU   |
                          |         |
                          '----+----'
                               |
                               |
                    .----------+---------.
                    |                    |
                 ---+                    +---
                E1  |         SC         |  E3
                 ---+                    +---
                E2  |                    |  E4
                    '--+--+--+--+--+--+--'
                       |  |  |  |  |  |
                      E5 E6 E7 E8 E9 E10

So, running `tcpdump e1` in a switching setup, inside the CPU, the only
traffic that will be captured is traffic ingressing port E1 destined for
the CPU itself.  To analyze traffic going through the switch, you need
something called *port mirroring*, or *port monitoring*, i.e., setting
up the switch core to mirror traffic ingressing and/or egressing a set
of ports to another port.  On this *monitor port* you can then run your
tcpdump, which means you need an external device (laptop).

> A planned feature is to support mirroring traffic to the CPU port,
> which would be an effective way to log and monitor traffic over a
> longer period of time.  Highly effective for diagnosing intermittent
> and other rare network issues.

If only "proof of life" is required, then sometimes port counters, also
called *RMON counters*, can be very useful too.  Seeing counters of a
particular type increment means traffic is ingressing or egressing.


### Examples

Listen to all traffic on an interface:

    admin@example:/> tcpdump e1

Listen to only ping traffic:

    admin@example:/> tcpdump e1 expression icmp

Listen to traffic on a given port:

    admin@example:/> tcpdump e1 expression "port 80"

Wait for one ping only:

    admin@example:/> tcpdump e1 count 1 expression icmp

Very verbose output:

    admin@example:/> tcpdump e1 verbose

