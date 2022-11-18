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

