#!/usr/bin/env bash
# This is based on scripts/setup.sh from Netopeer2/libnetconf2
#
#set -x

# Source the include file, which contain the list of YANG models and
# their respective enabled features in a MODULES array.
# Example:
#           MODULES=("module@revision -e feature1 -e feature2")
source "$1"

# optional env variable override
if [ -n "$SYSREPOCTL_EXECUTABLE" ]; then
    SYSREPOCTL="$SYSREPOCTL_EXECUTABLE"
# avoid problems with sudo PATH
elif [ `id -u` -eq 0 ] && [ -n "$USER" ] && [ `command -v su` ]; then
    SYSREPOCTL=`command sysrepoctl -l $USER`
else
    SYSREPOCTL=`command sysrepoctl`
fi

MODDIR=${SEARCH_PATH}
PERMS="660"
#OWNER=root
#GROUP=wheel

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
    CMD="'$SYSREPOCTL' -U $MODDIR/$1  -s '$MODDIR' -v2"
    eval "$CMD"
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
	echo "*** Installing YANG model $name ..."
        INSTALL_MODULE_CMD "$i"
	continue
    fi

    sctl_revision=`echo "$SCTL_MODULE" | sed 's/[^|]*| \([^ ]*\).*/\1/'`
    revision=`echo "$i" | sed 's/[^@]*@\([^\.]*\).*/\1/'`
    if [ "$sctl_revision" \< "$revision" ]; then
        # update module without any features
        file=`echo "$i" | cut -d' ' -f 1`
	echo "*** Updating YANG model $name ($file) ..."
        UPDATE_MODULE "$file"
    fi

    sctl_owner=`echo "$SCTL_MODULE" | sed 's/\([^|]*|\)\{3\} \([^:]*\).*/\2/'`
    sctl_group=`echo "$SCTL_MODULE" | sed 's/\([^|]*|\)\{3\}[^:]*:\([^ ]*\).*/\2/'`
    sctl_perms=`echo "$SCTL_MODULE" | sed 's/\([^|]*|\)\{4\} \([^ ]*\).*/\2/'`
    if [ "$sctl_perms" != "$PERMS" ]; then
        # change permissions/owner
	echo "*** Changing YANG model $name permissions ..."
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
if [ -n "${CMD_INSTALL}" ]; then
    printf "*** Installing YANG models ...\n%s" "$CMD_INSTALL"
    eval $CMD_INSTALL
    rc=$?
    if [ $rc -ne 0 ]; then
        exit $rc
    fi
fi
