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
#include <libite/queue.h>
#include <libyang/libyang.h>
#include <sysrepo.h>
#include <sysrepo/values.h>
#include <sysrepo/xpath.h>

#include "dagger.h"
#include "helpers.h"
#include "systemv.h"

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

#define CB_PRIO_PRIMARY   65535
#define CB_PRIO_PASSIVE   65000

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
		goto fail

#define REGISTER_OPER(s,m,x,c,a,f,u)				\
	if ((rc = register_oper(s, m, x, c, a, f, u)))		\
		goto fail

#define REGISTER_RPC(s,x,c,a,u)					\
	if ((rc = register_rpc(s, x, c, a, u)))			\
		goto fail

struct confd {
	sr_session_ctx_t       *session; /* running datastore */
	sr_session_ctx_t       *startup; /* startup datastore */
	sr_conn_ctx_t          *conn;
	sr_subscription_ctx_t  *sub;

	augeas                 *aug;
	struct dagger		netdag;
};

uint32_t core_hook_prio    (void);
int      core_commit_done  (sr_session_ctx_t *, uint32_t, const char *, const char *, sr_event_t, unsigned, void *);
int      core_startup_save (sr_session_ctx_t *, uint32_t, const char *, const char *, sr_event_t, unsigned, void *);

static inline int register_change(sr_session_ctx_t *session, const char *module, const char *xpath,
	int flags, sr_module_change_cb cb, void *arg, sr_subscription_ctx_t **sub)
{
	struct confd *confd = (struct confd *)arg;
	int rc;

	rc = sr_module_change_subscribe(session, module, xpath, cb, arg,
				CB_PRIO_PRIMARY, flags | SR_SUBSCR_DEFAULT, sub);
	if (rc) {
		ERROR("failed subscribing to changes of %s: %s", xpath, sr_strerror(rc));
		return rc;
	}

	/*
	 * For standard subscribtions we hook into the callback chain
	 * for all modules to figure out, per changeset, which of the
	 * callbacks is the last one.  This is where we want to call the
	 * global commit-done hook for candidate -> running changes and
	 * the startup-save hook for running -> startup copying.
	 */
	if (!flags) {
		sr_module_change_subscribe(confd->session, module, xpath, core_commit_done, NULL,
				core_hook_prio(), SR_SUBSCR_PASSIVE, sub);
		sr_module_change_subscribe(confd->startup, module, xpath, core_startup_save, NULL,
				core_hook_prio(), SR_SUBSCR_PASSIVE, sub);
	}

	return 0;
}

static inline int register_oper(sr_session_ctx_t *session, const char *module, const char *xpath,
	sr_oper_get_items_cb cb, void *arg, int flags, sr_subscription_ctx_t **sub)
{
	int rc = sr_oper_get_subscribe(session, module, xpath, cb, arg,
				flags | SR_SUBSCR_DEFAULT, sub);
	if (rc)
		ERROR("failed subscribing to %s oper: %s", xpath, sr_strerror(rc));
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
