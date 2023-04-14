/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_CORE_H_
#define CONFD_CORE_H_

#include <errno.h>
#include <stdio.h>
#include <syslog.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <sys/time.h>
#include <unistd.h>

#include <augeas.h>
#include <libite/lite.h>
#include <libyang/libyang.h>
#include <sysrepo.h>
#include <sysrepo/values.h>
#include <sysrepo/xpath.h>

#ifndef HAVE_VASPRINTF
int vasprintf(char **strp, const char *fmt, va_list ap);
#endif
#ifndef HAVE_ASPRINTF
int asprintf(char **strp, const char *fmt, ...);
#endif

#define DEBUG(fmt, ...)
//#define DEBUG(fmt, ...) syslog(LOG_DEBUG, "%s: "fmt, __func__, ##__VA_ARGS__)
#define ERROR(fmt, ...) syslog(LOG_ERR, "%s: " fmt, __func__, ##__VA_ARGS__)
#define ERRNO(fmt, ...) syslog(LOG_ERR, "%s: " fmt ": %s", __func__, ##__VA_ARGS__, strerror(errno))

static inline void print_val(sr_val_t *val)
{
	char *str;

	if (sr_print_val_mem(&str, val))
		return;
	ERROR("%s", str);
	free(str);
}

#define REGISTER_CHANGE(s,m,x,f,c,a,u)				\
	if ((rc = register_change(s, m, x, f, c, a, u)))	\
		goto err

#define REGISTER_RPC(s,x,c,a,u)				\
	if ((rc = register_rpc(s, x, c, a, u)))		\
		goto err

struct confd {
	sr_session_ctx_t       *session;
	sr_conn_ctx_t          *conn;
	sr_subscription_ctx_t  *sub;

	augeas                 *aug;
};

static inline int register_change(sr_session_ctx_t *session, const char *module, const char *xpath,
			int flags, sr_module_change_cb cb, void *arg, sr_subscription_ctx_t **sub)
{
	int rc = sr_module_change_subscribe(session, module, xpath, cb, arg, 0, flags | SR_SUBSCR_DEFAULT, sub);
	if (rc)
		ERROR("failed subscribing to changes of %s: %s", xpath, sr_strerror(rc));
	return rc;
}

static inline int register_rpc(sr_session_ctx_t *session, const char *xpath,
	sr_rpc_cb cb, void *arg, sr_subscription_ctx_t **sub)
{
	int rc = sr_rpc_subscribe(session, xpath, cb, arg, 0, SR_SUBSCR_DEFAULT, sub);
	if (rc)
		ERROR("failed subscribing to %s rpc: %s", xpath, sr_strerror(rc));
	return rc;
}

/* ietf-interfaces.c */
int ietf_interfaces_init(struct confd *confd);

/* ietf-system.c */
int ietf_system_init(struct confd *confd);

#endif	/* CONFD_CORE_H_ */
