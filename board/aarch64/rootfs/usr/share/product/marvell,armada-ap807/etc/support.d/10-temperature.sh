#!/bin/sh

set -e

# Build a map of phandle -> device tree node path for all PHY nodes
build_phandle_map()
{
    for phy_node in /sys/firmware/devicetree/base/cp*/config-space*/mdio*/switch*/mdio/ethernet-phy@* \
                    /sys/firmware/devicetree/base/cp*/config-space*/mdio*/ethernet-phy@*; do
        [ -f "$phy_node/phandle" ] || continue

        phandle=$(od -An -t x4 -N 4 "$phy_node/phandle" 2>/dev/null | tr -d ' ')
        if [ -n "$phandle" ]; then
            echo "$phandle:$phy_node"
        fi
    done
}

build_phy_map()
{
    phandle_map=$(build_phandle_map)

    # Build a mapping of PHY of_node path -> interface name
    for iface in /sys/class/net/*; do
        [ -d "$iface" ] || continue

        iface_name=$(basename "$iface")

        # Try regular phydev approach first (for non-DSA interfaces)
        if [ -L "$iface/phydev/of_node" ]; then
            phy_of_node=$(readlink -f "$iface/phydev/of_node" 2>/dev/null)
            if [ -n "$phy_of_node" ]; then
                echo "$phy_of_node:$iface_name"
                continue
            fi
        fi

        # For DSA interfaces, resolve via of_node's phy-handle
        if [ -L "$iface/of_node" ]; then
            iface_of_node=$(readlink -f "$iface/of_node" 2>/dev/null)
            [ -n "$iface_of_node" ] || continue

            # Try to read phy-handle property (4-byte phandle)
            if [ -f "$iface_of_node/phy-handle" ]; then
                phy_phandle=$(od -An -t x4 -N 4 "$iface_of_node/phy-handle" 2>/dev/null | tr -d ' ')

                if [ -n "$phy_phandle" ]; then
                    # Look up the PHY node path from our phandle map
                    phy_of_node=$(echo "$phandle_map" | grep "^$phy_phandle:" | cut -d: -f2)
                    if [ -n "$phy_of_node" ]; then
                        echo "$phy_of_node:$iface_name"
                    fi
                fi
            fi
        fi
    done
}

zone_map()
{
    type="$1"

    case "$type" in
        ap-ic-thermal)
            echo "Application processor interconnect"
            ;;
        ap-cpu[0-9]*-thermal)
            cpu=${type#ap-cpu}
            cpu=${cpu%-thermal}
            echo "Application processor core $cpu"
            ;;
        cp[0-9]*-ic-thermal)
            cp=${type%%-*}
            cp=${cp#cp}
            echo "Communication processor $cp interconnect"
            ;;
        *)
            echo "$type"
            ;;
    esac
}

thermal_zones()
{
    echo "Thermal Zones"
    echo "============="
    echo

    for zone in /sys/class/thermal/thermal_zone*; do
        [ -d "$zone" ] || continue

        name=$(basename "$zone")
        type=$(cat "$zone/type" 2>/dev/null || echo "unknown")
        data=$(cat "$zone/temp" 2>/dev/null)
        desc=$(zone_map "$type")

        if [ -n "$data" ] && [ "$data" != "N/A" ]; then
            # Convert millidegrees to degrees Celsius
            temp_c=$(awk "BEGIN {printf \"%.1f\", $data / 1000}")
            printf "%-20s %8s°C    %s\n" "$name" "$temp_c" "$desc"
        else
            printf "%-20s %8s      %s\n" "$name" "N/A" "$desc"
        fi
    done
    echo
}

hwmon()
{
    tmpfile=$(mktemp)

    echo "Hardware Monitors"
    echo "================="
    echo

    phy_map=$(build_phy_map)

    for hwmon in /sys/class/hwmon/hwmon*; do
        [ -d "$hwmon" ] || continue

        name=$(basename "$hwmon")
        data=$(cat "$hwmon/temp1_input" 2>/dev/null)

        # Try to find the associated network interface
        iface=
        if [ -L "$hwmon/of_node" ]; then
            hwmon_of_node=$(readlink -f "$hwmon/of_node" 2>/dev/null)
            if [ -n "$hwmon_of_node" ]; then
                iface=$(echo "$phy_map" | grep "^$hwmon_of_node:" | cut -d: -f2)
            fi
        fi

        if [ -n "$iface" ]; then
            description="Phy $iface temperature"
        else
            description="N/A"
        fi

        if [ -n "$data" ] && [ "$data" != "N/A" ]; then
            # Convert millidegrees to degrees Celsius
            temp_c=$(awk "BEGIN {printf \"%.1f\", $data / 1000}")
            # Format: sortkey|hwmon|temp|description (sortkey for natural sort by interface)
            printf "%s|%-20s %8s°C    %s\n" "$iface" "$name" "$temp_c" "$description" >> "$tmpfile"
        else
            printf "%s|%-20s %8s      %s\n" "$iface" "$name" "N/A" "$description" >> "$tmpfile"
        fi
    done

    # Sort by interface name naturally (e2 before e10), with N/A entries at the end
    # Then strip the sort key before displaying
    sort -V -t'|' -k1,1 "$tmpfile" | cut -d'|' -f2-
    rm -f "$tmpfile"
    echo
}

[ -n "$1" ] || { echo "usage: $0 OUT-DIR"; exit 1; }
work="$1"/system
mkdir -p "${work}"

thermal_zones >  "${work}"/temperature.txt
hwmon         >> "${work}"/temperature.txt
