# -*- sh -*-
# REMEMBER TO UPDATE infix-interfaces ALSO IN containers.inc
MODULES=(
	"ietf-system@2014-08-06.yang -e authentication -e local-users -e ntp -e ntp-udp-port -e timezone-name"
	"iana-timezones@2013-11-19.yang"
	"notifications@2008-07-14.yang"
	"ietf-tcp-common@2023-12-28.yang -e keepalives-supported"
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
	# NOTE: ietf-tls-client must be version matched with ietf-tls-server, used by netopeer2!
	"ietf-tls-client@2023-12-28.yang" # used by ietf-syslog (remote)
	"ietf-syslog@2024-03-21.yang -e file-action -e file-limit-size -e remote-action"
	"infix-syslog@2024-07-19.yang"
	"iana-hardware@2018-03-13.yang"
	"ietf-hardware@2018-03-13.yang -e hardware-state"
	"infix-hardware@2024-04-25.yang"
	"ieee802-dot1q-types@2022-10-29.yang"
	"infix-ip@2024-09-16.yang"
	"infix-if-type@2024-10-13.yang"
	"infix-routing@2024-10-01.yang"
	"ieee802-dot1ab-lldp@2022-03-15.yang"
	"infix-lldp@2023-08-23.yang"
	"infix-dhcp-client@2024-09-20.yang"
	"infix-meta@2024-10-18.yang"
	"infix-system@2024-09-13.yang"
	"infix-services@2024-05-30.yang"
	"ieee802-ethernet-interface@2019-06-21.yang"
	"infix-ethernet-interface@2024-02-27.yang"
	"infix-factory-default@2023-06-28.yang"
	"infix-interfaces@2024-10-28.yang -e vlan-filtering"

	# from rousette
	"ietf-restconf@2017-01-26.yang"
	"ietf-yang-patch@2017-02-22.yang"
	"ietf-restconf-monitoring@2017-01-26.yang"

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
	"ietf-tls-server@2023-12-28.yang -e server-ident-raw-public-key -e server-ident-x509-cert"

)
