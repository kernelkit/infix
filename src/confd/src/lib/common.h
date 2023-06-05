/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_COMMON_H_
#define CONFD_COMMON_H_

#include <syslog.h>
#include <sysrepo.h>
#include <sysrepo.h>
#include "srx_module.h"

#ifndef HAVE_VASPRINTF
int vasprintf(char **strp, const char *fmt, va_list ap);
#endif
#ifndef HAVE_ASPRINTF
int asprintf(char **strp, const char *fmt, ...);
#endif

#define DEBUG(fmt, ...)
//#define DEBUG(fmt, ...) syslog(LOG_DEBUG, "%s: "fmt, __func__, ##__VA_ARGS__)
#define INFO(fmt, ...) syslog(LOG_INFO, fmt, ##__VA_ARGS__)
#define ERROR(fmt, ...) syslog(LOG_ERR, "%s: " fmt, __func__, ##__VA_ARGS__)
#define ERRNO(fmt, ...) syslog(LOG_ERR, "%s: " fmt ": %s", __func__, ##__VA_ARGS__, strerror(errno))

#endif	/* CONFD_COMMON_H_ */
