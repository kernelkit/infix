aarch64
=======

Microchip SparX-5i PCB135 (eMMC)
--------------------------------

At present, only FIT images are supported via the SparX-5i eval
board's U-Boot, which must contain a valid load address. If you are
using this board, after selecting your base configuration, run the
following command to supplement the existing config with the required
FIT options:

    make board-enable-sparx-fit

### Unbricking

1. Don't load a corrupt bootloader to begin with
2. Done

If, for some reason, you didn't manage to follow step 1, you can use
the `FLASH_PROG`(`J4`) connector on the board to access the SPI flash
directly.

Using a [Bus Blaster][BB3] in combination with [dangerspi][dangerspi],
you can load a new bootloader. The schematic below details how to
connect the Bus Blaster to the board, but the concept should be
portable to any debugger/device based around an FTDI2232 chip.

```
Bus Blaster:         FLASH_PROG (J4):
       .---.              .---.
   VTG >o o| VTG      SCK >o o| GND
  TRST |o o| GND     MISO |o o| #RESET_FLASH
   TDI |o o| GND           o o| 3V3
   TMS |o o| GND      #CS |o o| #SYSRESET
   TCK  o o| GND     MOSI |o o| GND
  RTCK  o o| GND          '---'
   TDO |o o| GND
 TSRST |o o| GND
 DBGRQ |o o| GND
DBGACK |o o| GND
       '---'
```

| Bus Blaster | FLASH_PROG     |
|-------------|----------------|
| `VTG`       | `3V3`          |
| `GND`       | `GND`          |
|             |                |
| `VTG`       | `#RESET_FLASH` |
| `GND`       | `#SYSRESET`    |
|             |                |
| `TDI`       | `MOSI`         |
| `TMS`       | `#CS`          |
| `TCK`       | `SCK`          |
| `TDO`       | `MISO`         |

[BB3]: http://dangerousprototypes.com/docs/Bus_Blaster#Bus_Blaster_v3
[dangerspi]: https://github.com/wkz/dangerspi
