/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_COMMON_H_
#define CONFD_COMMON_H_

#include <syslog.h>
#include <sysrepo.h>
#include <sysrepo.h>
#include "srx_module.h"
#include "common.h"

extern int debug;

#ifndef HAVE_VASPRINTF
int vasprintf(char **strp, const char *fmt, va_list ap);
#endif
#ifndef HAVE_ASPRINTF
int asprintf(char **strp, const char *fmt, ...);
#endif

/*
 * Only NOTICE and above are logged by default, see setlogmask() for details.
 * Use `set DEBUG=1` at top of package/confd/confd.conf for logmask = debug
 */
#define DEBUG(fmt, ...) do { if (debug) syslog(LOG_DEBUG, fmt, ##__VA_ARGS__); } while (0)
#define INFO(fmt, ...)  syslog(LOG_INFO, fmt, ##__VA_ARGS__)
#define NOTE(fmt, ...)  syslog(LOG_NOTICE, fmt, ##__VA_ARGS__)
#define WARN(fmt, ...)  syslog(LOG_WARNING, fmt, ##__VA_ARGS__)
#define ERROR(fmt, ...) syslog(LOG_ERR, fmt, ##__VA_ARGS__)
#define ERRNO(fmt, ...) syslog(LOG_ERR, fmt ": %s", ##__VA_ARGS__, strerror(errno))

#endif	/* CONFD_COMMON_H_ */
