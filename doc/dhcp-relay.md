DHCP Relay
==========

A DHCP relay agent lets clients get their address from a DHCP server
they cannot reach with a broadcast, or from one that needs to know
where in the network a client is connected.  The relay agent picks up
the clients' requests, sends them on to one or more servers, and relays
the replies back.

The relay agent can add the relay agent information option, option 82,
to each request.  It tells the server which port the request arrived
on, the *circuit-id*, and which relay agent sent it, the *remote-id*.
The server can then hand out an address per port instead of per device:
the camera on port `e2` always gets the same address, and so does the
camera that replaces it.

Only DHCPv4 is supported.


## Basic Configuration

The clients are connected to the ports of bridge `br0`, which has the
address 192.168.1.1/24.  The DHCP server, 192.168.2.2, is on another
network that the device has a route to:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit dhcp-relay</b>
admin@example:/config/dhcp-relay/> <b>set server 192.168.2.2</b>
admin@example:/config/dhcp-relay/> <b>set interface br0</b>
admin@example:/config/dhcp-relay/> <b>leave</b>
</code></pre>

Requests from all ports of the bridge are now relayed.  The address of
the interface, here 192.168.1.1, is the *gateway address*: the server
sends its replies to it and uses it to select the subnet to hand out an
address from.  The server therefore needs a route back to it.

Any other interface, or a VLAN interface, can be relayed from as well,
see [VLANs](#vlans) below.  When more than one server is set, requests
are sent to all of them.  A server set for an interface replaces the
global ones for that interface.

> [!NOTE]
> Requests on a bridge port that is relayed from are not forwarded by
> the bridge.  A DHCP server connected to another port of the same
> bridge only sees the relayed requests.


## Option 82

Enable option 82 for all interfaces:

<pre class="cli"><code>admin@example:/config/> <b>edit dhcp-relay</b>
admin@example:/config/dhcp-relay/> <b>set option82</b>
admin@example:/config/dhcp-relay/> <b>leave</b>
</code></pre>

The circuit-id is by default the name of the port, e.g., `e2`, and the
remote-id the base MAC address of the device.  Both can be set to a
string, `str`, or to binary data, `hex`, of at most 50 bytes:

<pre class="cli"><code>admin@example:/config/dhcp-relay/> <b>set option82 remote-id str switch-17</b>
admin@example:/config/dhcp-relay/> <b>edit interface br0</b>
admin@example:/config/dhcp-relay/…/br0/> <b>set port e2 circuit-id str camera-18</b>
admin@example:/config/dhcp-relay/…/br0/> <b>leave</b>
</code></pre>

The remote-id can also be the gateway address or the hostname, with
`set option82 remote-id type giaddr` or `hostname`.

A client should never send option 82 itself.  What happens to a request
that has it is set with `policy`:

| **Policy** | **Description**                                         |
|------------|---------------------------------------------------------|
| `replace`  | Replace it with the relay agent's own, the default      |
| `keep`     | Relay the request with the option it has                |
| `drop`     | Drop the request                                        |

To relay from one interface without adding option 82, while it is
enabled for the others:

<pre class="cli"><code>admin@example:/config/dhcp-relay/> <b>set interface e5 option82 false</b>
</code></pre>


## Port Settings

All ports of a bridge are relayed from.  The `port` list of an
interface holds the exceptions.  It is only available for a bridge and
for a VLAN interface on top of a bridge.

| **Setting**  | **Description**                                               |
|--------------|---------------------------------------------------------------|
| `enabled`    | Set to false to not relay from the port.  DHCP on it is then forwarded by the bridge as any other traffic |
| `trusted`    | Set to true for a port where another relay agent, or a switch that adds option 82, is connected |
| `circuit-id` | Circuit-id of the port, default the port name                 |

Disable relaying on the port a DHCP server is connected to, if nothing
else on the port needs an address:

<pre class="cli"><code>admin@example:/config/dhcp-relay/…/br0/> <b>set port e8 enabled false</b>
</code></pre>


## Chains and Rings of Switches

When several switches in the same subnet relay to the same server, each
switch adds option 82 for its own access ports, and the ports between
the switches are set as trusted:

<pre class="cli"><code>admin@example:/config/dhcp-relay/…/br0/> <b>set port e7 trusted true</b>
admin@example:/config/dhcp-relay/…/br0/> <b>set port e8 trusted true</b>
</code></pre>

On a trusted port:

- option 82 in a request is kept, whatever the `policy`
- a request already relayed by another relay agent is accepted.  From
  other ports such requests are dropped, since a client could send them
  to pick its own circuit-id
- a request without option 82 gets the option of this switch.  A
  neighboring switch that gets its own address with DHCP is thereby
  identified by the port it is connected to

A switch that gets its own address with DHCP starts to relay as soon
as it has its address.

> [!NOTE]
> In a ring, a switch that gets its address with DHCP is connected to
> two neighbors, and both relay its request, with different circuit-ids.
> Match such a switch on something else in the server, e.g., its MAC
> address.


## VLANs

On a VLAN filtering bridge, relay from the VLAN interfaces on top of
the bridge.  The address of the VLAN interface is the gateway address,
so each VLAN gets its addresses from its own subnet:

<pre class="cli"><code>admin@example:/config/dhcp-relay/> <b>set interface vlan10</b>
admin@example:/config/dhcp-relay/> <b>set interface vlan20</b>
</code></pre>

The ports of a VLAN interface are the bridge ports with that VLAN as
PVID, and the tagged members of the VLAN.  A trunk port is relayed from
in each of its VLANs.


## DHCP Server on the Same Device

The DHCP server can run on the device itself, so a single device hands
out an address per port.  Use 127.0.0.1 as server:

<pre class="cli"><code>admin@example:/config/> <b>edit dhcp-relay</b>
admin@example:/config/dhcp-relay/> <b>set server 127.0.0.1</b>
admin@example:/config/dhcp-relay/> <b>set option82</b>
admin@example:/config/dhcp-relay/> <b>set interface br0</b>
admin@example:/config/dhcp-relay/> <b>end</b>
admin@example:/config/> <b>edit dhcp-server subnet 192.168.1.0/24</b>
admin@example:/config/dhcp-server/…/192.168.1.0/24/> <b>set host 192.168.1.42 match option82 circuit-id str e2</b>
admin@example:/config/dhcp-server/…/192.168.1.0/24/> <b>set host 192.168.1.42 preempt true</b>
admin@example:/config/dhcp-server/…/192.168.1.0/24/> <b>leave</b>
</code></pre>

Whatever is connected to port `e2` now gets 192.168.1.42.  See [DHCP
Server](dhcp.md#address-per-port) for the `match` and `preempt`
settings.

> [!IMPORTANT]
> Option 82 must be enabled with a server on the same device.  The
> clients then also renew their leases through the relay agent.


## Forcing Renewals Through the Relay Agent

A client renews its lease directly with the server, so a renewal does
not pass the relay agent and has no option 82.  A server that hands out
addresses by option 82 handles that, but some need the option in every
request:

<pre class="cli"><code>admin@example:/config/dhcp-relay/> <b>set force-server-identity true</b>
</code></pre>

The clients are then told that the relay agent is their server, and
send their renewals to it.  The server must support the server
identifier override sub-option, RFC 5107.  Option 82 must be enabled.
