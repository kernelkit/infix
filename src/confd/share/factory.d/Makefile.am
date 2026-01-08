factorydir          	= $(pkgdatadir)/factory.d
dist_factory_DATA   	= 10-netconf-server.json 10-infix-services.json 10-system.json
nodist_factory_DATA 	= 10-nacm.json

EXTRA_DIST		= 10-nacm.json.in
CLEANFILES		= 10-nacm.json

# Generic rule to process .json.in templates with conditional markers
%.json: %.json.in Makefile
if CONTAINERS
	$(AM_V_GEN)sed -e 's/#CONTAINERS#//g' $< > $@
else
	$(AM_V_GEN)sed -e '/#CONTAINERS#/d' $< > $@
endif
