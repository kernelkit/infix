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
	   -i ietf-system@2014-08-06.yang				\
           	-e authentication					\
		-e local-users						\
		-e ntp							\
		-e ntp-udp-port						\
		-e timezone-name					\
	   -i nc-notifications@2008-07-14.yang				\
	   -i notifications@2008-07-14.yang				\
	   -i ietf-keystore@2019-07-02.yang				\
	   	-e keystore-supported					\
	   	-e local-definitions-supported				\
	   	-e key-generation					\
	   -i ietf-truststore@2019-07-02.yang				\
		-e truststore-supported					\
		-e x509-certificates					\
	   -i ietf-tcp-common@2019-07-02.yang				\
		-e keepalives-supported					\
	   -i ietf-ssh-server@2019-07-02.yang				\
		-e local-client-auth-supported				\
	   -i ietf-tls-server@2019-07-02.yang				\
		-e local-client-auth-supported				\
 	   -i ietf-netconf-server@2019-07-02.yang			\
	   	-e ssh-listen						\
		-e tls-listen						\
		-e ssh-call-home					\
		-e tls-call-home					\
	   -i ietf-interfaces@2018-02-20.yang    -e if-mib		\
	   -i ietf-ip@2018-02-22.yang					\
	   -i ietf-network-instance@2019-01-21.yang			\
	   -i ietf-netconf-monitoring@2010-10-04.yang			\
	   -i ietf-netconf-nmda@2019-01-07.yang				\
		-e origin						\
		-e with-defaults					\
	   -i ietf-subscribed-notifications@2019-09-09.yang		\
		-e encode-xml						\
		-e replay						\
		-e subtree						\
		-e xpath						\
	   -i ietf-yang-push@2019-09-09.yang     -e on-change		\
	   -i iana-if-type@2023-01-26.yang				\
	   -i ietf-if-extensions@2023-01-26.yang -e sub-interfaces	\
	   -i ieee802-dot1q-types@2022-10-29.yang			\
	   -i ietf-if-vlan-encapsulation@2023-01-26.yang		\
	   -i infix-ip@2023-04-24.yang					\
	   -i infix-if-type@2023-06-09.yang				\
	   -i infix-interfaces@2023-06-05.yang   -e vlan-filtering	\
	   -i infix-system@2023-04-11.yang				\
	   -I "${INIT_DATA}"
rc=$?

# Enable features required by netopeer2
sysrepoctl -c ietf-netconf						\
		-e writable-running					\
		-e candidate						\
		-e rollback-on-error					\
		-e validate						\
		-e startup						\
		-e url							\
		-e xpath						\
		-e confirmed-commit

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
