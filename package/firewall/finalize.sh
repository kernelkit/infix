#!/bin/sh
set -e

TARGET_DIR="$1"
FIREWALL_SERVICES_YANG="$2"
FIREWALL_DAEMON_DIR="${TARGET_DIR}/usr/lib/firewalld"

# Cleanup — remove unnecessary firewalld files and create required directories
cleanup()
{
    rm -rf "${TARGET_DIR}/etc/firewall"*
    rm -f  "${TARGET_DIR}/usr/bin/firewall-applet"
    rm -rf "${TARGET_DIR}/usr/share/firewalld"

    # Keep only the three zones required by firewalld (core/fw.py)
    find "${FIREWALL_DAEMON_DIR}/zones" -type f \
	 ! -name block.xml   \
	 ! -name drop.xml    \
	 ! -name trusted.xml \
	 -delete

    mkdir -p "${TARGET_DIR}/etc/firewalld/zones"
    mkdir -p "${TARGET_DIR}/etc/firewalld/policies"
    mkdir -p "${TARGET_DIR}/etc/firewalld/services"
    touch    "${TARGET_DIR}/etc/firewalld/firewalld.conf"
    mkdir -p "${FIREWALL_DAEMON_DIR}/services"
}

# Prune services — keep only those that match YANG enums
prune_services()
{
    if [ ! -f "${FIREWALL_SERVICES_YANG}" ]; then
	echo "ERROR: ${FIREWALL_SERVICES_YANG} not found"
	exit 1
    fi

    # Extract enum values from YANG model
    ENUMS=$(grep 'enum "' "${FIREWALL_SERVICES_YANG}" | \
	        sed 's/.*enum "\([^"]*\)".*/\1/')

    # Validate that all YANG enums have corresponding .xml files
    MISSING=0
    for service in ${ENUMS}; do
	if [ ! -f "${FIREWALL_DAEMON_DIR}/services/${service}.xml" ]; then
	    echo "Service ${service} is not a known firewalld service"
	    MISSING=1
	fi
    done

    if [ ${MISSING} -eq 1 ]; then
	exit 1
    fi

    # Remove .xml files that are not in YANG enums
    cd "${FIREWALL_DAEMON_DIR}/services/"
    for xmlfile in *.xml; do
	service="${xmlfile%.xml}"
	if ! echo "${ENUMS}" | grep -q "^${service}$"; then
	    rm "${xmlfile}"
	fi
    done
}

# Mark built-in zones and policies as immutable
mark_builtins()
{
    FIREWALL_XML_FILES="${FIREWALL_DAEMON_DIR}/policies/*.xml ${FIREWALL_DAEMON_DIR}/zones/*.xml"

    for xmlfile in ${FIREWALL_XML_FILES}; do
	[ -f "${xmlfile}" ] || continue
	grep -q "(immutable)" "${xmlfile}" && continue

	if grep -q '<short>' "${xmlfile}"; then
	    sed -i 's|<short>\(.*\)</short>|<short>\1 (immutable)</short>|' \
		"${xmlfile}"
	else
	    if echo "${xmlfile}" | grep -q "/policies/"; then
		sed -i 's|<policy|<short>(immutable)</short>\n&|' \
		    "${xmlfile}"
	    else
		sed -i 's|<zone|<short>(immutable)</short>\n&|' \
		    "${xmlfile}"
	    fi
	fi
    done
}

cleanup
prune_services
mark_builtins
