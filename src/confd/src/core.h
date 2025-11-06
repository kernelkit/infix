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
#include <stdbool.h>

#include <libite/lite.h>
#include <libite/queue.h>
#include <libyang/libyang.h>
#include <sysrepo.h>
#include <sysrepo/error_format.h>
#include <sysrepo/values.h>
#include <sysrepo/xpath.h>

#include <jansson.h>

#include <srx/lyx.h>
#include <srx/common.h>
#include <srx/helpers.h>
#include <srx/systemv.h>

#include "dagger.h"

#define CB_PRIO_PRIMARY   65535
#define CB_PRIO_PASSIVE   65000

struct snippet {
	FILE *fp;
	char *data;
	size_t size;
};

static inline int snippet_close(struct snippet *s, FILE *out)
{
	int err = 0;

	if (s->fp)
		fclose(s->fp);

	if (!s->size)
		return 0;

	if (out)
		if (fwrite(s->data, s->size, 1, out) != 1)
			err = -EIO;

	free(s->data);
	return err;
}

static inline int snippet_open(struct snippet *s)
{
	memset(s, 0, sizeof(*s));

	s->fp = open_memstream(&s->data, &s->size);
	if (!s->fp)
		return -ENOMEM;

	return 0;
}

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
typedef enum {
	CONFD_DEP_DONE = 0,
	CONFD_DEP_ADDED = 1,
	CONFD_DEP_ERROR = 2
} confd_dependency_t;

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
	json_t                 *ifquirks;
	struct dagger		netdag;
};


static inline int register_change(sr_session_ctx_t *session, const char *module, const char *xpath,
	int flags, sr_module_change_cb cb, void *arg, sr_subscription_ctx_t **sub)
{
	int rc = sr_module_change_subscribe(session, module, xpath, cb, arg,
				CB_PRIO_PRIMARY, flags | SR_SUBSCR_DEFAULT, sub);
	if (rc) {
		ERROR("failed subscribing to changes of %s: %s", xpath, sr_strerror(rc));
		return rc;
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
int ietf_interfaces_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd);
int ietf_interfaces_cand_init(struct confd *confd);

/* ietf-syslog.c */
int ietf_syslog_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd);

/* ietf-system.c */
int ietf_system_rpc_init (struct confd *confd);
int hostnamefmt      (struct confd *confd, const char *fmt, char *hostnm, size_t hostlen, char *domain, size_t domlen);
int ietf_system_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd);

/* infix-containers.c */
#ifdef CONTAINERS
int infix_containers_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd);
int infix_containers_rpc_init(struct confd *confd);
#else
static inline int infix_containers_rpc_init(struct confd *confd) { return 0; }
static inline int infix_containers_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd) { return 0; }
#endif

/* infix-dhcp-common.c */
int dhcp_option_lookup(const struct lyd_node *id);

/* infix-dhcp-client.c */
int infix_dhcp_client_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd);
int infix_dhcp_client_candidate_init(struct confd *confd);

/* infix-dhcp-server.c */
int infix_dhcp_server_candidate_init(struct confd *confd);
int infix_dhcp_server_rpc_init(struct confd *confd);
int infix_dhcp_server_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd);

/* ietf-routing */
int ietf_routing_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd);

/* infix-factory.c */
int infix_factory_rpc_init(struct confd *confd);

/* ietf-factory-default */
int ietf_factory_default_rpc_init(struct confd *confd);

/* infix-meta.c */
int infix_meta_change_cb(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd);

/* infix-system-software.c */
int infix_system_sw_rpc_init(struct confd *confd);

/* infix-services.c */
int infix_services_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd);

/* ietf-hardware.c */
int ietf_hardware_candidate_init(struct confd *confd);
int ietf_hardware_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd);

/* ietf-keystore.c */
#define SSH_HOSTKEYS "/etc/ssh/hostkeys"
#define SSH_HOSTKEYS_NEXT SSH_HOSTKEYS"+"
int ietf_keystore_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd);

/* infix-firewall.c */
int infix_firewall_rpc_init(struct confd *confd);
int infix_firewall_candidate_init(struct confd *confd);
int infix_firewall_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd);

#endif	/* CONFD_CORE_H_ */
