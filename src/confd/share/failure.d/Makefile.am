failuredir		= $(pkgdatadir)/failure.d
dist_failure_DATA	= 10-keystore.json 10-nacm.json \
			  10-infix-services.json 10-system.json

if !NETCONF_SUBSYSTEM
dist_failure_DATA  += 10-netconf-server.json
endif
