lib_LTLIBRARIES     = libsrx.la

libsrx_la_CPPFLAGS  = -D_DEFAULT_SOURCE -D_XOPEN_SOURCE -D_GNU_SOURCE
libsrx_la_CFLAGS    = -W -Wall -Wextra -std=gnu11
libsrx_la_LDFLAGS   = $(AM_LDFLAGS) -version-info 0:0:0
libsrx_la_SOURCES   = common.c      common.h		\
                      helpers.c     helpers.h		\
                      lyx.c         lyx.h		\
                      srx_module.c  srx_module.h	\
                      srx_val.c     srx_val.h		\
                      systemv.c     systemv.h

pkgincludedir       = $(includedir)/srx
pkginclude_HEADERS  = common.h helpers.h lyx.h srx_module.h srx_val.h systemv.h

pkgconfigdir        = $(libdir)/pkgconfig
pkgconfig_DATA      = libsrx.pc
