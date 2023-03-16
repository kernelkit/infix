include $(top_srcdir)/common.am

SUBDIRS = common core ietf-interfaces ietf-system

install-exec-hook:
	find $(DESTDIR)$(clixonlibdir) -name '*.la' -delete
