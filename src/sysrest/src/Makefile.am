sbin_PROGRAMS = sysrest

sysrest_SOURCES = main.c
sysrest_CFLAGS  = -Wall -Wextra -Werror -Wno-unused-parameter \
			$(sysrepo_CFLAGS) $(fcgi_CFLAGS)
sysrest_LDADD   = $(sysrepo_LIBS) $(fcgi_LIBS)
