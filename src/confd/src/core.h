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
#include <libyang/libyang.h>
#include <sysrepo.h>
#include <sysrepo/values.h>
#include <sysrepo/xpath.h>


#define NELEMS(arr) (sizeof(arr) / sizeof(arr[0]))

#define DEBUG(frmt, ...)
//#define DEBUG(frmt, ...) syslog(LOG_DEBUG, "%s: "frmt, __func__, ##__VA_ARGS__)
#define ERROR(frmt, ...) syslog(LOG_ERR, "%s: " frmt, __func__, ##__VA_ARGS__)
#define ERRNO(frmt, ...) syslog(LOG_ERR, "%s: " frmt ": %s", __func__, ##__VA_ARGS__, strerror(errno))

#define REGISTER_CHANGE(s,x,c,a,u) \
	if (rc = register_change(s, x, c, a, u))\
		goto err

#define REGISTER_RPC(s,x,c,a,u) \
	if (rc = register_rpc(s, x, c, a, u))	\
		goto err

struct confd {
	sr_session_ctx_t       *session;
	sr_conn_ctx_t          *conn;
	sr_subscription_ctx_t  *sub;

	augeas                 *aug;
};

static inline int register_change(sr_session_ctx_t *session, const char *xpath,
	sr_module_change_cb cb, void *arg, sr_subscription_ctx_t **sub)
{
	int rc = sr_module_change_subscribe(session, "ietf-system", xpath, cb, arg, 0,
					    SR_SUBSCR_DEFAULT | SR_SUBSCR_ENABLED, sub);
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


/* core.c */
int run(const char *fmt, ...);

/* ietf-syste.c */
int ietf_system_init(struct confd *confd);

#endif	/* CONFD_CORE_H_ */
