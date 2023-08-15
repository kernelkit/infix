#!/bin/sh
# 1. Load all yang models with /cfg/factory-config
# 2. [ if /cfg/startup-config is missing :: copy {factory} -> /cfg/startup-config ]
# 3. Import /cfg/startup -> {startup} ]
# 4. Clear running-config :: import NULL -> {running}
# 5. Start sysrepo-plugind
# 6. Activate startup-config by :: copy {startup} -> {running}
#
# It's all really simple ... this script takes care of 1-4
#

FACTORY=$1
STARTUP=$2
INIT_DATA=/etc/sysrepo/factory-default.json
SEARCH=/usr/share/yang/modules/confd:/usr/share/yang/modules/libnetconf2:/usr/share/yang/modules/libyang:/usr/share/yang/modules/netopeer2:/usr/share/yang/modules/sysrepo

if [ -z "$FACTORY" -o -z "$STARTUP" ]; then
        echo "Missing argument to sysrepo-bootstrap.sh $FACTORY $STARTUP"
        exit 1
fi

# Drop all pre-initialized data from netopeer2 install, then re-create
# with required netopeer2 models, sysrepo implicitly installs its own,
# and then we initialize it all with our factory defaults.
rm -rf /etc/sysrepo/* /dev/shm/sr_*
mkdir -p /etc/sysrepo/
cp "$FACTORY" "$INIT_DATA"
sysrepoctl -s $SEARCH							\
	   -i ietf-system@2014-08-06.yang      -g wheel -p 0660		\
           	-e authentication					\
		-e local-users						\
		-e ntp							\
		-e ntp-udp-port						\
		-e timezone-name					\
	   -i iana-timezones@2013-11-19.yang   -g wheel -p 0660		\
	   -i nc-notifications@2008-07-14.yang -g wheel -p 0660		\
	   -i notifications@2008-07-14.yang    -g wheel -p 0660		\
	   -i ietf-keystore@2019-07-02.yang    -g wheel -p 0660		\
	   	-e keystore-supported					\
	   	-e local-definitions-supported				\
	   	-e key-generation					\
	   -i ietf-truststore@2019-07-02.yang	-g wheel -p 0660	\
		-e truststore-supported					\
		-e x509-certificates					\
	   -i ietf-tcp-common@2019-07-02.yang	-g wheel -p 0660	\
		-e keepalives-supported					\
	   -i ietf-ssh-server@2019-07-02.yang	-g wheel -p 0660	\
		-e local-client-auth-supported				\
	   -i ietf-tls-server@2019-07-02.yang	-g wheel -p 0660	\
		-e local-client-auth-supported				\
 	   -i ietf-netconf-server@2019-07-02.yang -g wheel -p 0660	\
	   	-e ssh-listen						\
		-e tls-listen						\
		-e ssh-call-home					\
		-e tls-call-home					\
	   -i ietf-interfaces@2018-02-20.yang   -g wheel -p 0660	\
		-e if-mib						\
	   -i ietf-ip@2018-02-22.yang		-g wheel -p 0660	\
	   -i ietf-network-instance@2019-01-21.yang -g wheel -p 0660	\
	   -i ietf-netconf-monitoring@2010-10-04.yang -g wheel -p 0660	\
	   -i ietf-netconf-nmda@2019-01-07.yang -g wheel -p 0660	\
		-e origin						\
		-e with-defaults					\
	   -i ietf-subscribed-notifications@2019-09-09.yang		\
		-g wheel -p 0660 					\
		-e encode-xml						\
		-e replay						\
		-e subtree						\
		-e xpath						\
	   -i ietf-yang-push@2019-09-09.yang    -g wheel -p 0660	\
		-e on-change						\
	   -i iana-if-type@2023-01-26.yang	-g wheel -p 0660	\
	   -i ietf-if-extensions@2023-01-26.yang -g wheel -p 0660	\
		-e sub-interfaces					\
	   -i ieee802-dot1q-types@2022-10-29.yang -g wheel -p 0660	\
	   -i ietf-if-vlan-encapsulation@2023-01-26.yang		\
		-g wheel -p 0660 					\
	   -i infix-ip@2023-04-24.yang		-g wheel -p 0660	\
	   -i infix-if-type@2023-06-09.yang	-g wheel -p 0660	\
	   -i infix-interfaces@2023-06-05.yang	-g wheel -p 0660	\
		-e vlan-filtering					\
	   -i infix-system@2023-08-15.yang	-g wheel -p 0660	\
	   -I "${INIT_DATA}"
rc=$?

# Unlike `sysrepoctl -i` the `-c` command requires separate invocations.
# NOTE: we ignore any errors from these at bootstrap since sysrepo may
#       already enable some of these feature, resulting in error here.
# Enable features required by netopeer2
sysrepoctl -c ietf-netconf			-g wheel -p 0660	\
		-e writable-running					\
		-e candidate						\
		-e rollback-on-error					\
		-e validate						\
		-e startup						\
		-e url							\
		-e xpath						\
		-e confirmed-commit
# Allow wheel group users (admin) to modify NACM
sysrepoctl -c ietf-netconf-acm -g wheel -p 0660

# On first boot, install factory-config as startup-config
# Otherwise, load startup-config to {startup}.  Due to a
# limitation in sysrepo we cannot initialize factory for
# ietf-netconf-acm, so we cheat, see sysrepo#3079
if [ -f "$STARTUP" ]; then
    sysrepocfg -f json -I"$STARTUP"
else
    sysrepocfg -f json -X"$STARTUP"
fi

# Clear running-config
echo "{}" > "$INIT_DATA"
sysrepocfg -f json -I"$INIT_DATA" -d running

exit $rc
