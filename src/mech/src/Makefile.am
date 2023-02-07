include $(top_srcdir)/common.am

CFLAGS  = -Wall -Wextra -Werror -Wno-unused-parameter
SUBDIRS = core

install-exec-hook:
	find $(DESTDIR)$(clixonlibdir) -name '*.la' -delete
