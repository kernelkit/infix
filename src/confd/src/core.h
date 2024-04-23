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
#include <sys/param.h>
#include <unistd.h>

#include <augeas.h>
#include <libite/lite.h>
#include <libite/queue.h>
#include <libyang/libyang.h>
#include <sysrepo.h>
#include <sysrepo/error_format.h>
#include <sysrepo/values.h>
#include <sysrepo/xpath.h>

#include <jansson.h>

#include <srx/common.h>
#include <srx/helpers.h>
#include <srx/systemv.h>

#include "dagger.h"

#define CB_PRIO_PRIMARY   65535
#define CB_PRIO_PASSIVE   65000

extern struct confd confd;


static inline void print_val(sr_val_t *val)
{
	char *str;

	if (sr_print_val_mem(&str, val))
		return;
	ERROR("%s", str);
	free(str);
}

static inline char *xpath_base(const char *xpath)
{
	char *path, *ptr;

	if (!xpath)
		return NULL;

	path = strdup(xpath);
	if (!path)
		return NULL;

	if (!(ptr = strstr(path, "]/"))) {
		free(path);
		return NULL;
	}
	ptr[1] = 0;

	return path;
}

#define REGISTER_CHANGE(s,m,x,f,c,a,u)				\
	if ((rc = register_change(s, m, x, f, c, a, u)))	\
		goto fail

#define REGISTER_MONITOR(s,m,x,f,c,a,u)				\
	if ((rc = register_monitor(s, m, x, f, c, a, u)))	\
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
	sr_session_ctx_t       *cand;    /* candidate datastore */
	sr_conn_ctx_t          *conn;
	sr_subscription_ctx_t  *sub;
	sr_subscription_ctx_t  *fsub;    /* factory-default sub */
	json_t                 *root;
	augeas                 *aug;
	struct dagger		netdag;
};

uint32_t core_hook_prio    (void);
int      core_pre_hook     (sr_session_ctx_t *, uint32_t, const char *, const char *, sr_event_t, unsigned, void *);
int      core_post_hook    (sr_session_ctx_t *, uint32_t, const char *, const char *, sr_event_t, unsigned, void *);
int      core_startup_save (sr_session_ctx_t *, uint32_t, const char *, const char *, sr_event_t, unsigned, void *);

static inline int register_change(sr_session_ctx_t *session, const char *module, const char *xpath,
	int flags, sr_module_change_cb cb, void *arg, sr_subscription_ctx_t **sub)
{
	struct confd *confd = (struct confd *)arg;
	int rc;

	if (!flags) {
		sr_module_change_subscribe(confd->session, module, xpath, core_pre_hook, NULL,
				0, SR_SUBSCR_PASSIVE, sub);
	}

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
		sr_module_change_subscribe(confd->session, module, xpath, core_post_hook, NULL,
				core_hook_prio(), SR_SUBSCR_PASSIVE, sub);
		sr_module_change_subscribe(confd->startup, module, xpath, core_startup_save, NULL,
				core_hook_prio(), SR_SUBSCR_PASSIVE, sub);
	}

	return 0;
}

/* Seconday callbacks, not responsible for the main property. */
static inline int register_monitor(sr_session_ctx_t *session, const char *module, const char *xpath,
	int flags, sr_module_change_cb cb, void *arg, sr_subscription_ctx_t **sub)
{
	int rc = sr_module_change_subscribe(session, module, xpath, cb, arg,
					    0, flags | SR_SUBSCR_PASSIVE, sub);
	if (rc) {
		ERROR("failed subscribing to monitor %s: %s", xpath, sr_strerror(rc));
		return rc;
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

/* infix-containers.c */
#ifdef CONTAINERS
int  infix_containers_init(struct confd *confd);
void infix_containers_pre_hook(sr_session_ctx_t *session, struct confd *confd);
void infix_containers_post_hook(sr_session_ctx_t *session, struct confd *confd);
#else
static inline int  infix_containers_init(struct confd *confd) { return 0; }
static inline void infix_containers_pre_hook(sr_session_ctx_t *session, struct confd *confd) {}
static inline void infix_containers_post_hook(sr_session_ctx_t *session, struct confd *confd) {}
#endif

/* infix-dhcp.c */
int infix_dhcp_init(struct confd *confd);

/* ietf-factory-default */
int ietf_factory_default_init(struct confd *confd);

/* ietf-routing */
int ietf_routing_init(struct confd *confd);

/* infix-factory.c */
int infix_factory_init(struct confd *confd);

/* infix-system-software.c */
int infix_system_sw_init(struct confd *confd);

/* infix-services.c */
int infix_services_init(struct confd *confd);

/* ietf-hardware.c */
int ietf_hardware_init(struct confd *confd);

#endif	/* CONFD_CORE_H_ */
