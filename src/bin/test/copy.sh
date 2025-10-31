#!/bin/sh

cfg1=cfg1-$(dd if=/dev/urandom bs=256 count=1 status=none | crc32)
cfg2=cfg2-$(dd if=/dev/urandom bs=256 count=1 status=none | crc32)
cfg3=cfg3-$(dd if=/dev/urandom bs=256 count=1 status=none | crc32)

copy_or_die()
{
    copy "$@" || {
	echo "FAIL: copy $@" >&2
	exit 1
    }
}

expect_sys_hostname()
{
    local actual=$(hostname)
    local expect=$1

    [ $actual = $expect ] || {
	echo "FAIL: Expected system hostname \"$expect\", saw \"$actual\"" >&2
	exit 1
    }
}

expect_file_hostname()
{
    local expect=$1
    local file=$2
    local actual=$(jq -r '."ietf-system:system".hostname' $file)

    [ $actual = $expect ] || {
	echo "FAIL: Expected file hostname \"$expect\", saw \"$actual\", in $file" >&2
	exit 1
    }
}

expect_ds_hostname()
{
    local expect=$1
    local ds=$2
    local actual=$(sysrepocfg -d $ds -G "/ietf-system:system/hostname")

    [ $actual = $expect ] || {
	echo "FAIL: Expected DS hostname \"$expect\", saw \"$actual\", in $ds" >&2
	exit 1
    }

}

gencfgs()
{
    echo -n "Generate configs... " >&2

    rm -f /cfg/cfg[123].cfg

    sysrepocfg -S /ietf-system:system/hostname --value $cfg1
    copy_or_die running /cfg/cfg1.cfg
    sysrepocfg -S /ietf-system:system/hostname --value $cfg2
    copy_or_die running /cfg/cfg2.cfg
    sysrepocfg -S /ietf-system:system/hostname --value $cfg3
    copy_or_die running /cfg/cfg3.cfg

    echo "OK" >&2
}

gencfgs

echo -n "Import to running... " >&2
copy_or_die /cfg/cfg1.cfg running-config
expect_sys_hostname $cfg1
expect_ds_hostname $cfg1 running
echo "OK" >&2

echo -n "Export to startup... " >&2
copy_or_die running-config startup-config
expect_file_hostname $cfg1 /cfg/startup-config.cfg
expect_ds_hostname $cfg1 startup
echo "OK" >&2

echo -n "Import to startup... " >&2
copy_or_die /cfg/cfg2.cfg startup-config
expect_file_hostname $cfg2 /cfg/startup-config.cfg
expect_ds_hostname $cfg2 startup
echo "OK" >&2

echo -n "Apply startup to running... " >&2
copy_or_die startup-config running-config
expect_ds_hostname $cfg2 running
expect_sys_hostname $cfg2
echo "OK" >&2

echo -n "Export to remote... " >&2
copy_or_die running-config file:///cfg/export.cfg
expect_file_hostname $cfg2 /cfg/export.cfg
echo "OK" >&2

echo -n "Local copy... " >&2
rm -f /cfg/import.cfg
copy_or_die /cfg/cfg3.cfg /cfg/import.cfg
expect_file_hostname $cfg3 /cfg/import.cfg
echo "OK" >&2

echo -n "Import from remote... " >&2
copy_or_die file:///cfg/import.cfg running-config
expect_ds_hostname $cfg3 running
expect_sys_hostname $cfg3
echo "OK" >&2

echo -n "Apply factory to running... " >&2
copy_or_die factory-config running-config
# Difficult to know what to expect here
echo "OK" >&2
