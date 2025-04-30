# -*- sh -*-
# Modules from netopeer2

MODULES=(
	"ietf-interfaces@2018-02-20.yang"
	"ietf-ip@2018-02-22.yang"
	"ietf-netconf@2013-09-29.yang -e writable-running -e candidate -e rollback-on-error -e validate -e startup -e url -e xpath -e confirmed-commit"
	"ietf-netconf-nmda@2019-01-07.yang -e origin -e with-defaults"
	"notifications@2008-07-14.yang"
	"nc-notifications@2008-07-14.yang"
	"ietf-netconf-monitoring@2010-10-04.yang"
	"ietf-network-instance@2019-01-21.yang"
	"ietf-subscribed-notifications@2019-09-09.yang -e encode-xml -e replay -e subtree -e xpath"
	"ietf-yang-push@2019-09-09.yang -e on-change"
	"ietf-system-capabilities@2022-02-17.yang"
	"ietf-notification-capabilities@2022-02-17.yang"
)
