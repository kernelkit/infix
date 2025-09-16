[![License Badge][]][License] [![GitHub Status][]][GitHub] [![Coverity Status][]][Coverity Scan] [![Discord][discord-badge]][discord-url]

<img align="right" src="doc/logo.png" alt="Infix - Linux <3 NETCONF" width=480 border=10>

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
with built-in help for every command — just hit `?` or TAB for
context-aware assistance.  Familiar NETCONF/RESTCONF APIs and
[comprehensive documentation][4] mean you're never stuck.  Whether
you're learning networking or managing enterprise infrastructure.

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

## See It In Action

Configure an interface in seconds - the CLI guides you with built-in help:

<details><summary><b>Click Here for an example CLI Session</b></summary>

```bash
admin@infix-12-34-56:/> configure
admin@infix-12-34-56:/config/> edit interface eth0
admin@infix-12-34-56:/config/interface/eth0/> set ipv4 <TAB>
      address     autoconf bind-ni-name      enabled
      forwarding  mtu      neighbor
admin@infix-12-34-56:/config/interface/eth0/> set ipv4 address 192.168.2.200 prefix-length 24
admin@infix-12-34-56:/config/interface/eth0/> show
type ethernet;
ipv4 {
  address 192.168.2.200 {
    prefix-length 24;
  }
}
admin@infix-12-34-56:/config/interface/eth0/> diff
interfaces {
  interface eth0 {
+    ipv4 {
+      address 192.168.2.200 {
+        prefix-length 24;
+      }
+    }
  }
}
admin@infix-12-34-56:/config/interface/eth0/> leave
admin@infix-12-34-56:/> show interfaces
INTERFACE       PROTOCOL   STATE       DATA
eth0            ethernet   UP          52:54:00:12:34:56
                ipv4                   192.168.2.200/24 (static)
                ipv6                   fe80::5054:ff:fe12:3456/64 (link-layer)
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@infix-12-34-56:/> copy running-config startup-config
```

Notice how TAB completion shows available options, `show` displays
current config, and `diff` shows exactly what changed before you
commit your changes with the `leave` command.

</details>

> [Full CLI documentation →][3]

## Get Started

Get [pre-built images][5] for your hardware.  Use the CLI, web
interface, or standard NETCONF/RESTCONF tools, e.g., `curl`.  Add
containers for any custom functionality you need.

### Supported Platforms

- **Raspberry Pi 4B** - Perfect for home labs, learning, and prototyping
- **NanoPi R2S** - Compact dual-port router in a tiny package  
- **x86_64** - Run in VMs or on mini PCs for development and testing
- **Marvell CN9130 CRB, EspressoBIN** - High-performance ARM platforms
- **Microchip SparX-5i, NXP i.MX8MP EVK** - Enterprise switching capabilities
- **StarFive VisionFive2** - RISC-V architecture support

*Why start with Raspberry Pi?* It's affordable, widely available, has
built-in WiFi + Ethernet, and runs the exact same Infix OS you'd deploy
in production. Perfect for learning, prototyping, or even small-scale
deployments.

> 📖 **[Complete documentation][4]** • 💬 **[Join our Discord][discord-url]**

## Technical Details

Built on proven open-source foundations ([Buildroot][1] + [sysrepo][2])
for reliability you can trust:

- **Immutable OS**: Read-only filesystem, atomic updates, instant rollback
- **YANG Configuration**: Industry-standard models with auto-generated tooling
- **Hardware Acceleration**: Linux switchdev support for wire-speed packet processing
- **Container Integration**: Docker support with flexible network and hardware access
- **Memory Efficient**: Runs comfortably on devices with as little as 256 MB RAM

Perfect for everything from resource-constrained edge devices to
high-throughput network appliances.

> Check the *[Latest Build][]* for bleeding-edge features.

---

<div align="center">
  <a href="https://github.com/wires-se"><img src="https://raw.githubusercontent.com/wires-se/.github/main/profile/play.svg" width=300></a>
  <br />Infix development is sponsored by <a href="https://wires.se">Wires</a>
</div>

![Alt](https://repobeats.axiom.co/api/embed/5ce7a2a67edc923823afa0f60c327a6e8575b6e9.svg "Repobeats analytics image")

[1]: https://buildroot.org/ "Buildroot Homepage"
[2]: https://www.sysrepo.org/ "Sysrepo Homepage"
[3]: https://kernelkit.org/infix/latest/cli/introduction/
[4]: https://kernelkit.org/infix/
[5]: https://github.com/kernelkit/infix/releases
[Latest Build]:    https://github.com/kernelkit/infix/releases/tag/latest "Latest build"
[License]:         https://en.wikipedia.org/wiki/GPL_license
[License Badge]:   https://img.shields.io/badge/License-GPL%20v2-blue.svg
[GitHub]:          https://github.com/kernelkit/infix/actions/workflows/build.yml/
[GitHub Status]:   https://github.com/kernelkit/infix/actions/workflows/build.yml/badge.svg
[Coverity Scan]:   https://scan.coverity.com/projects/29393
[Coverity Status]: https://scan.coverity.com/projects/29393/badge.svg
[discord-badge]:   https://img.shields.io/discord/1182652155618918411.svg?logo=discord
[discord-url]:     https://discord.gg/6bHJWQNVxN
