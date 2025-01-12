#!/usr/bin/env bash
# This is based on scripts/setup.sh from Netopeer2/libnetconf2
#
#set -x

# Source the include file, which contain the list of YANG models and
# their respective enabled features in a MODULES array.
# Example:
#           MODULES=("module@revision -e feature1 -e feature2")

# shellcheck disable=SC1090
source "$1"

# optional env variable override
if [ -n "$SYSREPOCTL_EXECUTABLE" ]; then
    SYSREPOCTL="$SYSREPOCTL_EXECUTABLE"
elif [ "$(id -u)" -eq 0 ] && [ -n "$USER" ] && [ -n "$(command -v su)" ]; then
    SYSREPOCTL=$(command sysrepoctl -l "$USER")
else
    SYSREPOCTL=$(command sysrepoctl)
fi

MODDIR=${SEARCH_PATH}
PERMS="660"
#OWNER=root
#GROUP=wheel

CMD_INSTALL=


install()
{
    if [ -z "${CMD_INSTALL}" ]; then
        CMD_INSTALL="'$SYSREPOCTL' -s $MODDIR -v2"
    fi
    CMD_INSTALL="$CMD_INSTALL -i $MODDIR/$1 -p '$PERMS'"
    if [ -n "${OWNER}" ]; then
        CMD_INSTALL="$CMD_INSTALL -o '$OWNER'"
    fi
    if [ -n "${GROUP}" ]; then
        CMD_INSTALL="$CMD_INSTALL -g '$GROUP'"
    fi
}


update()
{
    local module="$1"
    local cmd="'$SYSREPOCTL' -U $MODDIR/$module -s '$MODDIR' -v2"

    local output rc
    output=$(eval "$cmd" 2>&1)
    rc=$?

    if [ $rc -ne 0 ]; then
        if echo "$output" | grep -q "Module .* already installed"; then
            echo "*** Warning: Module $module is already installed. Skipping update."
            return 0
        fi
        echo "*** Error: failed updating module $module: $output" >&2
        return $rc
    fi

    echo "*** Successfully updated module $module."
    return 0
}


chperm()
{
    CMD="'$SYSREPOCTL' -c $1 -p '$PERMS' -v2"
    if [ -n "${OWNER}" ]; then
        CMD="$CMD -o '$OWNER'"
    fi
    if [ -n "${GROUP}" ]; then
        CMD="$CMD -g '$GROUP'"
    fi
    eval "$CMD"
    local rc=$?
    if [ $rc -ne 0 ]; then
        exit $rc
    fi
}


enable()
{
    $SYSREPOCTL -c "$1" -e "$2" -v2
    local rc=$?
    if [ $rc -ne 0 ]; then
        exit $rc
    fi
}


# Skip first 5 lines of header and last 3 lines of footer
SCTL_MODULES=$($SYSREPOCTL -l |tail -n +5 |head -n -3)

for module in "${MODULES[@]}"; do
    name=$(echo "$module" | awk -F'[@.]' '{print $1}')
    date=$(echo "$module" | awk -F'[@.]' '{print $2}')

    SCTL_MODULE=$(echo "$SCTL_MODULES" | grep "^$name \+|[^|]*| I")
    if [ -z "$SCTL_MODULE" ]; then
        # prepare command to install module with all its features
	echo "*** Installing YANG model $name ..."
        install "$module"
	continue
    fi

    rev=$(echo "$SCTL_MODULE" | awk '{print $3}')
    if [ "$rev" != "$date" ] || echo "$FORCE_UPDATE" | grep -qw "$name"; then
        # update module without any features
        file=$(echo "$module" | cut -d' ' -f 1)
	echo "*** Updating YANG model $name ($file) ..."
        update "$file"
    fi

    #sctl_owner=`echo "$SCTL_MODULE" | sed 's/\([^|]*|\)\{3\} \([^:]*\).*/\2/'`
    #sctl_group=`echo "$SCTL_MODULE" | sed 's/\([^|]*|\)\{3\}[^:]*:\([^ ]*\).*/\2/'`
    sctl_perms=$(echo "$SCTL_MODULE" | sed 's/\([^|]*|\)\{4\} \([^ ]*\).*/\2/')
    if [ "$sctl_perms" != "$PERMS" ]; then
        # change permissions/owner
	echo "*** Changing YANG model $name permissions ..."
        chperm "$name"
    fi

    # parse sysrepoctl features and add extra space at the end for easier matching
    sctl_features="`echo "$SCTL_MODULE" | sed 's/\([^|]*|\)\{6\}\(.*\)/\2/'` "
    # parse features we want to enable
    features=`echo "$module" | sed 's/[^ ]* \(.*\)/\1/'`
    while [ "${features:0:3}" = "-e " ]; do
        # skip "-e "
        features=${features:3}
        # parse feature
        feature=$(echo "$features" | sed 's/\([^[:space:]]*\).*/\1/')

        # enable feature if not already
        sctl_feature=$(echo "$sctl_features" | grep " ${feature} ")
        if [ -z "$sctl_feature" ]; then
            # enable feature
            enable "$name" "$feature"
        fi

        # next iteration, skip this feature
        features=$(echo "$features" | sed 's/[^[:space:]]* \(.*\)/\1/')
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
