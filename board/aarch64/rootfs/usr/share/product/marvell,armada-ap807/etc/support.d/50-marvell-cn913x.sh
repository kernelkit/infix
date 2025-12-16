#!/bin/sh

set -e

ecc_stat()
{
    local chan=
    local base=

    for chan in 0 1; do
        base=$((0xf0020360 + 0x200 * chan))

        echo "DRAM Channel $chan ECC Status"
        echo -n "  Log config: "; devmem $((base + 0x0)) 32
        echo -n "  1b errors:  "; devmem $((base + 0x4)) 32
        echo -n "  Info 0:     "; devmem $((base + 0x8)) 32
        echo -n "  Info 1:     "; devmem $((base + 0xc)) 32
        echo
    done
}

[ -n "$1" ] || { echo "usage: $0 OUT-DIR"; exit 1; }
work="$1"/marvell-cn913x
mkdir -p "${work}"

ecc_stat >"${work}"/ecc-stat
