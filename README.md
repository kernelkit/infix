[![License Badge][]][License] [![Release Badge][]][Release] [![GitHub Status][]][GitHub] [![Coverity Status][]][Coverity Scan] [![Discord][discord-badge]][discord-url]

<img align="right" src="doc/logo.png" alt="Infix — Immutable.Friendly.Secure" width=480 padding=10>

Turn any ARM or x86 device into a powerful, manageable network appliance
in minutes. From $35 Raspberry Pi boards to enterprise switches — deploy
routers, IoT gateways, edge devices, or custom network solutions that
just work.

## Our Values

**🔒 Immutable**  
Your system never breaks.  Read-only filesystem with atomic upgrades
means no configuration drift, no corrupted updates, and instant rollback
if something goes wrong.  Deploy once, trust forever.

**🤝 Friendly**  
Actually easy to use. Auto-generated CLI from standard YANG models comes
with built-in help for every command — just hit <kbd>?</kbd> or
<kbd>TAB</kbd> for context-aware assistance.

Familiar NETCONF & RESTCONF APIs and [comprehensive documentation][4]
mean you're never stuck.  Whether you're learning networking or managing
enterprise infrastructure.

**🛡️ Secure**  
Built with security as a foundation, not an afterthought.  Minimal
attack surface, separation between system and data, and container
isolation.  Sleep better knowing your infrastructure is protected.

## Why Choose Infix

**Hardware Flexibility**: Start with a $35 Raspberry Pi, scale to
enterprise switching hardware.  Same OS, same tools, same reliability.

**Standards-Based**: Built around YANG models and IETF standards. Learn
once, use everywhere - no vendor lock-in.

**Container Ready**: Run your applications alongside networking
functions.  GPIO access, dedicated Ethernet ports, custom protocols —
your device, your rules.

## Use Cases

1. **Home Labs & Hobbyists**:  
   Transform a Raspberry Pi into a full-featured router with WiFi  
1. **IoT & Edge Computing**:  
   Bridge devices to the cloud with reliable, updatable gateways  
1. **Small Business Networks**:  
   Enterprise-grade features without the complexity or cost  
1. **Developers & Makers**:  
   Test networking concepts, prototype IoT solutions, or build custom
   appliances
1. **Network Professionals**:  
   Consistent tooling from development to production deployment.  
   How about a digital twin using raw Qemu or [GNS3](https://gns3.com/infix)!

## Quick Example

Configure an interface in seconds - the CLI guides you with built-in help:

<pre><code>admin@infix-12-34-56:/> <b>configure</b>
admin@infix-12-34-56:/config/> <b>edit interface eth0</b>
admin@infix-12-34-56:/config/interface/eth0/> <b>set ipv4</b> <kbd>TAB</kbd>
      address     autoconf      bind-ni-name     dhcp 
      enabled     forwarding    mtu              neighbor
admin@infix-12-34-56:/config/interface/eth0/> <b>set ipv4 address 192.168.2.200 prefix-length 24</b>
admin@infix-12-34-56:/config/interface/eth0/> <b>show</b>
type ethernet;
ipv4 {
  address 192.168.2.200 {
    prefix-length 24;
  }
}
admin@infix-12-34-56:/config/interface/eth0/> <b>diff</b>
interfaces {
  interface eth0 {
+    ipv4 {
+      address 192.168.2.200 {
+        prefix-length 24;
+      }
+    }
  }
}
admin@infix-12-34-56:/config/interface/eth0/> <b>leave</b>
admin@infix-12-34-56:/> <b>show interfaces</b>
<u>INTERFACE       PROTOCOL   STATE       DATA                                  </u>
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
eth0            ethernet   UP          52:54:00:12:34:56
                ipv4                   192.168.2.200/24 (static)
                ipv6                   fe80::5054:ff:fe12:3456/64 (link-layer)
admin@infix-12-34-56:/> <b>copy running startup</b>
</code></pre>

Notice how <kbd>TAB</kbd> completion shows available options, `show`
displays current config, and `diff` shows exactly what changed before
you commit your changes with the `leave` command.

For more information, see [CLI documentation][3].

## Get Started

Get [pre-built images][5] for your hardware.  Use the CLI, web
interface, or standard NETCONF/RESTCONF tools, e.g., `curl`.  Add
containers for any custom functionality you need.

### Supported Platforms

- **Raspberry Pi 2B/3B/4B/CM4** - Perfect for home labs, learning, and prototyping
- **Banana Pi-R3** - Your next home router and gateway
- **NanoPi R2S** - Compact dual-port router in a tiny package
- **x86_64** - Run in VMs or on mini PCs for development and testing
- **Marvell CN9130 CRB, EspressoBIN** - High-performance ARM64 platforms
- **Microchip SparX-5i** - Enterprise switching capabilities
- **Microchip SAMA7G54-EK** - ARM Cortex-A7
- **NXP i.MX8MP EVK** - Highly capable ARM64 SoC
- **StarFive VisionFive2** - RISC-V architecture support

*Why start with Raspberry Pi?* It's affordable, widely available, has
built-in WiFi + Ethernet, and runs the exact same Infix OS you'd deploy
in production. Perfect for learning, prototyping, or even small-scale
deployments.

> 📖 **[Complete documentation][4]** • 💬 **[Join our Discord][discord-url]**

## Technical Details

<a href="https://bitsign.se">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://bitsign.se/assets/badges/bitsign-badge-dark-mode.png">
    <source media="(prefers-color-scheme: light)" srcset="https://bitsign.se/assets/badges/bitsign-badge-light-mode.png">
    <img alt="bitSign - Code Signing" src="https://bitsign.se/assets/badges/bitsign-badge-light-mode.png" align="right" width=150 padding=10>
  </picture>
</a>

Built on proven open-source foundations: [Linux][0], [Buildroot][1], and
[sysrepo][2] — for reliability you can trust:

- **Immutable OS**: Read-only filesystem, atomic updates, instant rollback
- **YANG Configuration**: Industry-standard models with auto-generated tooling
- **Hardware Acceleration**: Linux switchdev support for wire-speed packet processing
- **Container Integration**: Docker support with flexible network and hardware access
- **Memory Efficient**: Runs comfortably on devices with as little as 256 MB RAM
- **Code Signing**: Releases are cryptographically signed for integrity verification

Perfect for everything from resource-constrained edge devices to
high-throughput network appliances.

With the entire system modeled in YANG, scalability is no longer an
issue, be it in development, testing, or end users deploying and
monitoring their devices.  All knobs and dials are accessible from the
CLI (console/SSH), or remotely using the native NETCONF or RESTCONF
APIs.

> Check the *[Latest Build][]* for bleeding-edge features.

---

<div align="center">
  <a href="https://github.com/wires-se"><img src="https://raw.githubusercontent.com/wires-se/.github/main/profile/play.svg" width=300></a>
  <br />Infix development is sponsored by <a href="https://wires.se">Wires</a>
</div>

![Alt](https://repobeats.axiom.co/api/embed/5ce7a2a67edc923823afa0f60c327a6e8575b6e9.svg "Repobeats analytics image")

[0]: https://www.kernel.org
[1]: https://buildroot.org/ "Buildroot Homepage"
[2]: https://www.sysrepo.org/ "Sysrepo Homepage"
[3]: https://kernelkit.org/infix/latest/cli/introduction/
[4]: https://kernelkit.org/infix/
[5]: https://github.com/kernelkit/infix/releases
[Latest Build]:    https://github.com/kernelkit/infix/releases/tag/latest "Latest build"
[License]:         https://en.wikipedia.org/wiki/GPL_license
[License Badge]:   https://img.shields.io/badge/License-GPL%20v2-blue.svg
[Release]:         https://github.com/kernelkit/infix/releases
[Release Badge]:   https://img.shields.io/github/v/release/kernelkit/infix 
[GitHub]:          https://github.com/kernelkit/infix/actions/workflows/build.yml/
[GitHub Status]:   https://github.com/kernelkit/infix/actions/workflows/build.yml/badge.svg
[Coverity Scan]:   https://scan.coverity.com/projects/29393
[Coverity Status]: https://scan.coverity.com/projects/29393/badge.svg
[discord-badge]:   https://img.shields.io/discord/1182652155618918411.svg?logo=discord
[discord-url]:     https://discord.gg/6bHJWQNVxN
