# Microchip LAN969x (Laguna)

Files shared by the boards built on the LAN969x switch family:

| Board                                           | Package                            |
|-------------------------------------------------|------------------------------------|
| [Microchip EV23X71A](../microchip-ev23x71a)     | `BR2_PACKAGE_MICROCHIP_EV23X71A`   |
| [Novarq Tactical 1000](../novarq-tactical-1000) | `BR2_PACKAGE_NOVARQ_TACTICAL_1000` |

`uboot/` holds the U-Boot config fragment and environment used by the
`*_boot_defconfig` of each board.  Kernel device tree and configuration
fixups stay with each board, see its `dts/` directory and `.mk` file.

A new Laguna board should select `BR2_PACKAGE_SYMREG` in its `Config.in`
and carry the `microchip,sparx5-symreg` node in its `dts/microchip/infix.dtsi`,
copied from one of the boards above, so the tool below works on it too.

## Debugging

### Register access by name

Every Laguna board ships `symreg`, Microchip's tool for reading and
writing switch registers by their datasheet names.  It talks to the
`symreg` debugfs driver, which maps the register space the device tree
node describes and exposes it as `/sys/kernel/debug/symreg/mem`.  Root
only, since debugfs is.

```
symreg --help                   # options and the register syntax
symreg -m                       # MAC table
symreg -v                       # VLAN table
symreg -c 0                     # VCAP instance 0
symreg -s                       # stream table

symreg 'DEV2G5[29]*'            # all registers of port 29's device block
symreg 'ASM*[29]*'              # ingress side of the same port
symreg -a 'DSM*[29]*'           # same, with the physical addresses

symreg REG[idx]                 # read one register
symreg REG[idx].FIELD           # read one field
symreg REG[idx].FIELD 1         # write one field, add -f for wildcards
```

Register and field names follow the LAN969x datasheet.  Quote the
wildcards or the shell expands them.  The dispatcher `symreg` picks the
SoC from `/proc/device-tree/compatible` and runs `symreg_lan969x`, which
can also be called directly.

The tool reads live hardware, so the useful pattern for an intermittent
fault is a snapshot while the port works, another once it has failed,
and a diff of the two.  For the management port, which is switch port 29
on both boards:

```bash
for blk in 'DEV2G5[29]*' 'ASM*[29]*' 'DSM*[29]*' 'QSYS*[29]*'; do
	symreg "$blk"
done >/tmp/port29-$(date +%s).txt
```

Check the PHY as well.  The management PHY sits on the GCB MDIO bus and
shares one level triggered interrupt line with the copper PHYs, so a
stale link state can come from a missed PHY interrupt as well as from
the switch.  Read it with `mdio` from mdio-tools, see the
[EV23X71A README](../microchip-ev23x71a/README.md#debug-and-analysis),
which also covers the VCAP debugfs and `ethtool` counters; both apply to
either board.

### Writing registers

Writes take effect immediately and bypass the driver, which keeps no
record of them.  Expect the driver's own view to disagree afterwards,
and reboot before trusting any behaviour again.
