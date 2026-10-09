# PPPoE Client

Many Internet service providers connect their customers with PPP over
Ethernet (PPPoE, RFC 2516).  The device then logs in with a user name
and password, and gets its IPv4 address and DNS servers from the
provider's server.

A PPPoE client session is an interface of type `pppoe`, stacked on top
of the Ethernet interface, or VLAN, that connects to the provider.  The
interface exists as soon as it is configured, but is down until the
device has logged in, and goes down again when the session ends.  The
client keeps trying to log in until it succeeds, and logs in again when
a session is lost.

## Configuration

The password is stored in the [keystore](keystore.md), as a symmetric
key with `passphrase-key-format`:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit keystore symmetric-key isp</b>
admin@example:/config/keystore/…/isp/> <b>set key-format passphrase-key-format</b>
admin@example:/config/keystore/…/isp/> <b>edit cleartext-symmetric-key</b>
Passphrase: ********
Retype passphrase: ********
admin@example:/config/keystore/…/isp/> <b>end</b>
</code></pre>

Then create the PPPoE interface on top of the interface facing the
provider, here `eth0`.  The settings common to all PPP links, the user
name and password, are in `ppp`, and the PPPoE specific ones in `pppoe`:

<pre class="cli"><code>admin@example:/config/> <b>edit interface pppoe0</b>
admin@example:/config/interface/pppoe0/> <b>set pppoe lower-layer-if eth0</b>
admin@example:/config/interface/pppoe0/> <b>set ppp username user@isp.example</b>
admin@example:/config/interface/pppoe0/> <b>set ppp secret isp</b>
admin@example:/config/interface/pppoe0/> <b>show</b>
type pppoe;
ppp {
  username user@isp.example;
  secret isp;
}
pppoe {
  lower-layer-if eth0;
}
admin@example:/config/interface/pppoe0/> <b>leave</b>
</code></pre>

> [!TIP]
> If you name your PPPoE interface `pppoeN`, where `N` is a number, the
> CLI infers the interface type automatically.  Any other name, e.g.,
> `wan`, works too, with an explicit `set type pppoe`.

The password may not contain control characters, `"`, or `\`.

Some providers run several services, or several servers, on the same
network.  Set `service-name` or `ac-name` to only connect to a given
service, or a given access concentrator.

## Default Route and DNS

By default the session installs a default route, with route preference
5, the same as a route learned from a DHCP server.  Change it with `ppp
route-preference`, or turn the route off with `ppp default-route false`,
e.g., when the PPPoE session is a backup for another uplink.

The DNS servers from the provider are used by default.  Set `ppp
peer-dns false` to use only the DNS servers configured on the device.

## TCP MSS Clamping

PPPoE takes 8 bytes of every Ethernet frame, so the session MTU is 1492
bytes, lower than the 1500 bytes of the hosts on the local network.
Hosts rely on path MTU discovery to adjust, which fails where ICMP is
blocked on the way, and their TCP connections then stall on large
packets.

To avoid that, the device clamps the maximum segment size (MSS) in the
handshake of every TCP connection it forwards through the session, to
fit the session MTU.  This does not depend on the firewall being
enabled.  Set `pppoe mss-clamping false` to turn it off.

## IPv6

When IPv6 is enabled on the PPP interface the session also negotiates
IPv6, which gives the interface a link-local address.  Global addresses
and delegated prefixes come from the provider's router advertisements
and DHCPv6 server, enable the [DHCPv6 client](ip.md) on the PPP
interface to use them.

## Forwarding

To route traffic between the local network and the provider, enable
IPv4 (and IPv6) forwarding on the PPPoE interface and on the local
interfaces, see [IP Addressing](ip.md).
