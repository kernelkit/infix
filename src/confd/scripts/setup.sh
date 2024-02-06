#!/usr/bin/env bash
set -x
# This is based on scripts/setup.sh from Netopeer2/libnetconf2
# env variables NP2_MODULE_DIR, NP2_MODULE_PERMS must be defined and NP2_MODULE_OWNER, NP2_MODULE_GROUP will be used if
# defined when executing this script!
#if [ -z "$NP2_MODULE_DIR" -o -z "$NP2_MODULE_PERMS" ]; then
#    echo "Required environment variables not defined!"
#    exit 1
#fi

# optional env variable override
if [ -n "$SYSREPOCTL_EXECUTABLE" ]; then
    SYSREPOCTL="$SYSREPOCTL_EXECUTABLE"
# avoid problems with sudo PATH
elif [ `id -u` -eq 0 ] && [ -n "$USER" ] && [ `command -v su` ]; then
    SYSREPOCTL=`command -v sysrepoctl -l $USER`
else
    SYSREPOCTL=`command -v sysrepoctl`
fi
MODDIR=${SEARCH_PATH}
PERMS="660"
#OWNER=root
#GROUP=wheel

# array of modules to install
MODULES=(
	"ietf-system@2014-08-06.yang -e authentication -e local-users -e ntp -e ntp-udp-port -e timezone-name"
	"iana-timezones@2013-11-19.yang"
	"notifications@2008-07-14.yang"
	"ietf-tcp-common@2019-07-02.yang -e keepalives-supported"
	"ietf-interfaces@2018-02-20.yang -e if-mib"
	"ietf-ip@2018-02-22.yang -e ipv6-privacy-autoconf"
	"ietf-network-instance@2019-01-21.yang"
	"ietf-netconf-monitoring@2010-10-04.yang"
	"ietf-netconf-nmda@2019-01-07.yang -e origin -e with-defaults"
	"ietf-subscribed-notifications@2019-09-09.yang -e encode-xml -e replay -e subtree -e xpath"
	"ietf-yang-push@2019-09-09.yang -e on-change"
	"ietf-routing@2018-03-13.yang"
	"ietf-ipv6-unicast-routing@2018-03-13.yang"
	"ietf-ipv4-unicast-routing@2018-03-13.yang"
	"ietf-ospf@2022-10-19.yang -e bfd -e explicit-router-id"
	"iana-if-type@2023-01-26.yang"
	"iana-hardware@2018-03-13.yang"
	"ietf-hardware@2018-03-13.yang -e hardware-state"
	"infix-hardware@2024-04-25.yang"
	"ieee802-dot1q-types@2022-10-29.yang"
	"infix-ip@2023-09-14.yang"
	"infix-if-type@2024-01-29.yang"
	"infix-routing@2024-03-06.yang"
	"ieee802-dot1ab-lldp@2022-03-15.yang"
	"infix-lldp@2023-08-23.yang"
	"infix-dhcp-client@2024-04-12.yang"
	"infix-shell-type@2023-08-21.yang"
	"infix-system@2024-04-12.yang"
	"infix-services@2024-04-08.yang"
	"ieee802-ethernet-interface@2019-06-21.yang"
	"infix-ethernet-interface@2024-02-27.yang"
	"infix-factory-default@2023-06-28.yang"
	# from sysrepo
	"sysrepo-plugind@2022-08-26.yang"
	# from netopeer
	"nc-notifications@2008-07-14.yang"
	"ietf-crypto-types@2023-12-28.yang -e encrypted-private-keys"
 	"ietf-netconf-server@2023-12-28.yang -e ssh-listen -e tls-listen -e ssh-call-home -e tls-call-home"
	"ietf-netconf-acm@2018-02-14.yang"
	"ietf-netconf@2013-09-29.yang -e writable-running -e candidate -e rollback-on-error -e validate -e startup -e url -e xpath -e confirmed-commit"
	"ietf-truststore@2023-12-28.yang -e central-truststore-supported -e certificates"
	"ietf-keystore@2023-12-28.yang -e central-keystore-supported -e inline-definitions-supported -e asymmetric-keys -e symmetric-keys"
	"ietf-ssh-server@2023-12-28.yang -e local-user-auth-password -e local-user-auth-publickey"
	"ietf-tls-server@2023-12-28.yang -e server-ident-raw-public-key -e server-ident-x509-cert"
	"ietf-restconf@2017-01-26.yang"
)
if [ -n "$USE_CONTAINERS" ]; then
	CONTAINERS=" -e containers";
	MODULES+=("infix-containers@2024-03-27.yang")
fi

MODULES+=("infix-interfaces@2024-01-15.yang $CONTAINERS -e vlan-filtering")

CMD_INSTALL=

# functions
INSTALL_MODULE_CMD() {
    if [ -z "${CMD_INSTALL}" ]; then
        CMD_INSTALL="'$SYSREPOCTL' -s $MODDIR -v2"
    fi
    CMD_INSTALL="$CMD_INSTALL -i $MODDIR/$1 -p '$PERMS'"
    if [ ! -z "${OWNER}" ]; then
        CMD_INSTALL="$CMD_INSTALL -o '$OWNER'"
    fi
    if [ ! -z "${GROUP}" ]; then
        CMD_INSTALL="$CMD_INSTALL -g '$GROUP'"
    fi
}

UPDATE_MODULE() {
	CMD="'$SYSREPOCTL' -U $MODDIR/$1  -s '$MODDIR -v2"
    eval $CMD
    local rc=$?
    if [ $rc -ne 0 ]; then
        exit $rc
    fi
}

CHANGE_PERMS() {
    CMD="'$SYSREPOCTL' -c $1 -p '$PERMS' -v2"
    if [ ! -z "${OWNER}" ]; then
        CMD="$CMD -o '$OWNER'"
    fi
    if [ ! -z "${GROUP}" ]; then
        CMD="$CMD -g '$GROUP'"
    fi
    eval $CMD
    local rc=$?
    if [ $rc -ne 0 ]; then
        exit $rc
    fi
}

ENABLE_FEATURE() {
    "$SYSREPOCTL" -c $1 -e $2 -v2
    local rc=$?
    if [ $rc -ne 0 ]; then
        exit $rc
    fi
}

# get current modules
SCTL_MODULES=`$SYSREPOCTL -l`
for i in "${MODULES[@]}"; do
    name=`echo "$i" | sed 's/\([^@]*\).*/\1/'`

    SCTL_MODULE=`echo "$SCTL_MODULES" | grep "^$name \+|[^|]*| I"`
    if [ -z "$SCTL_MODULE" ]; then
        # prepare command to install module with all its features
        INSTALL_MODULE_CMD "$i"
	continue
    fi

    sctl_revision=`echo "$SCTL_MODULE" | sed 's/[^|]*| \([^ ]*\).*/\1/'`
    revision=`echo "$i" | sed 's/[^@]*@\([^\.]*\).*/\1/'`
    if [ "$sctl_revision" \< "$revision" ]; then
        # update module without any features
        file=`echo "$i" | cut -d' ' -f 1`
        UPDATE_MODULE "$file"
    fi

    sctl_owner=`echo "$SCTL_MODULE" | sed 's/\([^|]*|\)\{3\} \([^:]*\).*/\2/'`
    sctl_group=`echo "$SCTL_MODULE" | sed 's/\([^|]*|\)\{3\}[^:]*:\([^ ]*\).*/\2/'`
    sctl_perms=`echo "$SCTL_MODULE" | sed 's/\([^|]*|\)\{4\} \([^ ]*\).*/\2/'`
    if [ "$sctl_perms" != "$PERMS" ]; then
        # change permissions/owner
        CHANGE_PERMS "$name"
    fi

    # parse sysrepoctl features and add extra space at the end for easier matching
    sctl_features="`echo "$SCTL_MODULE" | sed 's/\([^|]*|\)\{6\}\(.*\)/\2/'` "
    # parse features we want to enable
    features=`echo "$i" | sed 's/[^ ]* \(.*\)/\1/'`
    while [ "${features:0:3}" = "-e " ]; do
        # skip "-e "
        features=${features:3}
        # parse feature
        feature=`echo "$features" | sed 's/\([^[:space:]]*\).*/\1/'`

        # enable feature if not already
        sctl_feature=`echo "$sctl_features" | grep " ${feature} "`
        if [ -z "$sctl_feature" ]; then
            # enable feature
            ENABLE_FEATURE $name $feature
        fi

        # next iteration, skip this feature
        features=`echo "$features" | sed 's/[^[:space:]]* \(.*\)/\1/'`
    done
done
# install all the new modules
if [ ! -z "${CMD_INSTALL}" ]; then
    eval $CMD_INSTALL
    rc=$?
    if [ $rc -ne 0 ]; then
        exit $rc
    fi
fi
