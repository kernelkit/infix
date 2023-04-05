sbin_PROGRAMS = sysrest

sysrest_SOURCES = main.c
sysrest_CFLAGS  = -Wall -Wextra -Werror -Wno-unused-parameter \
			$(libyang_CFLAGS) $(sysrepo_CFLAGS) $(fcgi_CFLAGS)
sysrest_LDADD   = $(libyang_LIBS) $(sysrepo_LIBS) $(fcgi_LIBS)
