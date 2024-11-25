Many of the defaults here are are taken from the Frr recommendations [1].
Below are relevant excerpts from the kernel documentation.


accept_ra, accept Router Advertisements; autoconfigure using them, also
           determines whether or not to transmit Router Solicitations.
           If and only if the functional setting is to accept Router
           Advertisements, Router Solicitations will be transmitted.

  0 - Do not accept Router Advertisements.

  1 - Accept Router Advertisements if forwarding is disabled.

  2 - Overrule forwarding behaviour. Accept Router Advertisements even
      if forwarding is enabled.

  Default:
    - enabled if local forwarding is disabled
    - disabled if local forwarding is enabled


accept_ra_pinfo, learn Prefix Information in Router Advertisement.

   Default:
    - enabled if accept_ra is enabled
    - disabled if accept_ra is disabled


autoconf, autoconfigure IPv6 addresses using Prefix Information in
          Router Advertisements.

   Default:
    - enabled if accept_ra_pinfo is enabled
    - disabled if accept_ra_pinfo is disabled


arp_announce, define restriction level for announcing the local source
              address from IP packets in ARP requests sent on interface:

  0 - (default) Use any local address, configured on any interface

  1 - Try to avoid local addresses that are not in the target’s subnet
      for this interface.  Useful when target hosts reachable via this
      interface require the source IP address in ARP requests to be part
      of their logical network configured on the receiving interface.
      When we generate the request we will check all our subnets that
      include the target IP and will preserve the source address if it
      is from such subnet. If there is no such subnet we select source
      address according to the rules for level 2.

  2 - Always use the best local address for this target. In this mode we
      ignore the source address in the IP packet and try to select local
      address that we prefer for talks with the target host. Such local
      address is selected by looking for primary IP addresses on all our
      subnets on the outgoing interface that include the target address.
      If no suitable local address is found we select the first local
      address we have on the outgoing interface or on all other
      interfaces, with the hope we will receive reply for our request
      and even sometimes no matter the source IP address we announce.


arp_notify, define mode for notification of address and device changes.

  0 - (default): do nothing
  1 - generate gratuitous arp requests when device is brought up or
      hardware address changes.


arp_ignore, define different modes for sending replies in response to
            received ARP requests that resolve local target addresses:

  0 - (default): reply for any local target IP address, configured on
      any interface

  1 - reply only if the target IP address is a local address configured
      on the incoming interface

  2 - reply only if the target IP address is local address configured on
      the incoming interface and both with the sender’s IP address are part
      from same subnet on this interface

  3 - do not reply for local addresses configured with scope host, only
      resolutions for global and link addresses are replied

  4-7 - reserved

  8 - do not reply for all local addresses


arp_accept, define behavior for accepting gratuitous ARP (garp) frames
      from devices that are not already present in the ARP table:

  0 - don’t create new entries in the ARP table

  1 - create new entries in the ARP table

  2 - create new entries only if the source IP address is in the same
      subnet as an address configured on the interface that received
      the garp message.

  Both replies and requests type gratuitous arp will trigger the ARP
  table to be updated, if this setting is on.  If the ARP table already
  contains the IP address of the gratuitous arp frame, the arp table
  will be updated regardless if this setting is on or off.


icmp_errors_use_inbound_ifaddr

  0 - (default): icmp error messages are sent with the primary address
      of the exiting interface.

  1 - the message will be sent with the primary address of the interface
      that received the packet that caused the icmp error. This is the
      behaviour many network administrators will expect from a router.
      And it can make debugging complicated network layouts much easier.

  Note, if no primary address exists for the interface selected, then
  the primary address of the first non-loopback interface that has one
  will be used regardless of this setting.


rp_filter, reverse path source filtering:

  0 - (default): no source validation.

  1 - Strict mode as defined in RFC3704, 'Strict Reverse Path'.  Each
      incoming packet is tested against the FIB and if the interface is
      not the best reverse path the packet check will fail.  By default
      failed packets are discarded.

  2 - Loose mode as defined in RFC3704, 'Loose Reverse Path'.  Each
      incoming packet’s source address is also tested against the FIB
      and if the source address is not reachable via any interface the
      packet check will fail.

  Current recommended practice in RFC3704 is to enable strict mode to
  prevent IP spoofing from DDos attacks. If using asymmetric routing or
  other complicated routing, then loose mode is recommended.

  The max value from conf/{all,interface}/rp_filter is used when doing
  source validation on the {interface}.



[1]: https://github.com/FRRouting/frr/blob/master/doc/user/Useful_Sysctl_Settings.md
