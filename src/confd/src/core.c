/* SPDX-License-Identifier: BSD-3-Clause */

#include <srx/common.h>
#include "core.h"

struct confd confd;


uint32_t core_hook_prio(void)
{
	static uint32_t hook_prio = CB_PRIO_PASSIVE;

	return hook_prio--;
}

int core_startup_save(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		      const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	/* skip in bootstrap, triggered by load script to initialize startup datastore */
	if (systemf("runlevel >/dev/null 2>&1"))
		return SR_ERR_OK;

	if (systemf("sysrepocfg -X/cfg/startup-config.cfg -d startup -f json"))
		return SR_ERR_SYS;

	return SR_ERR_OK;
}

static const char *ev2str(sr_event_t ev)
{
	switch (ev) {
	case SR_EV_UPDATE:  return "UPDATE";
	case SR_EV_CHANGE:  return "CHANGE";
	case SR_EV_DONE:    return "DONE";
	case SR_EV_ABORT:   return "ABORT";
	case SR_EV_ENABLED: return "ENABLED";
	case SR_EV_RPC:     return "ABORT";
	default:
		break;
	}

	return "UNKNOWN";
}

int core_pre_hook(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		  const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	return 0;
}

/*
 * Run on UPDATE to see how many modules have changes in the inbound changeset
 * Run on DONE, after the last module callback has run, to activate changes.
 * For details, see https://github.com/sysrepo/sysrepo/issues/2188
 */
int core_post_hook(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		   const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	static size_t num_changes = 0;

	switch (event) {
	case SR_EV_CHANGE:
		num_changes++;
		return SR_ERR_OK;
	case SR_EV_ABORT:
		num_changes = 0;
		return SR_ERR_OK;
	case SR_EV_DONE:
		num_changes--;
		if (num_changes > 0)
			return SR_ERR_OK;
		break;
	default:
		ERROR("core_post_hook() should not be called with event %s", ev2str(event));
		return SR_ERR_SYS;
	}

	/* skip reload in bootstrap, implicit reload in runlevel change */
	if (systemf("runlevel >/dev/null 2>&1")) {
		/* trigger any tasks waiting for confd to have applied *-config */
		system("initctl -nbq cond set bootstrap");
		return SR_ERR_OK;
	}

	if (systemf("initctl -b reload"))
		return SR_ERR_SYS;

	return SR_ERR_OK;
}

int sr_plugin_init_cb(sr_session_ctx_t *session, void **priv)
{
	int log_opts = LOG_PID | LOG_NDELAY;
	int rc = SR_ERR_SYS;
	const char *env;

	/* Convert into command line option+SIGUSR1 when converting to standalone confd */
	env = getenv("DEBUG");
	if (env) {
		log_opts |= LOG_PERROR;
		debug = 1;
	}

	openlog("confd", log_opts, LOG_DAEMON);

	/* Save context with default running config datastore for all our models */
	*priv = (void *)&confd;
	confd.session = session;
	confd.conn = sr_session_get_connection(session);
	confd.sub = NULL;
	confd.fsub = NULL;

	if (!confd.conn)
		goto err;

	/* The startup datastore is used for the core_startup_save() hook */
	rc = sr_session_start(confd.conn, SR_DS_STARTUP, &confd.startup);
	if (rc)
		goto err;

	/* Used by, e.g., ietf-interfaces, to update interfaces types */
	rc = sr_session_start(confd.conn, SR_DS_CANDIDATE, &confd.cand);
	if (rc)
		goto err;

	confd.root = json_load_file("/run/system.json", 0, NULL);
	if (!confd.root)
		goto err;

	/* An optional file that contains hardware specific quirks for
	 * the network interfaces on running board.
	 */
	confd.ifquirks = json_load_file("/etc/product/interface-quirks.json", 0, NULL);

	rc = ietf_interfaces_init(&confd);
	if (rc)
		goto err;
	rc = ietf_keystore_init(&confd);
	if (rc)
		goto err;
	rc = ietf_syslog_init(&confd);
	if (rc)
		goto err;
	rc = ietf_system_init(&confd);
	if (rc)
		goto err;
	rc = infix_containers_init(&confd);
	if (rc)
		goto err;
	rc = infix_dhcp_client_init(&confd);
	if (rc)
		goto err;
	rc = infix_dhcp_server_init(&confd);
	if (rc)
		goto err;
	rc = infix_factory_init(&confd);
	if (rc)
		goto err;
	rc = ietf_factory_default_init(&confd);
	if (rc)
		goto err;
	rc = ietf_routing_init(&confd);
	if (rc)
		goto err;
	rc = infix_meta_init(&confd);
	if (rc)
		goto err;
	rc = infix_system_sw_init(&confd);
	if (rc)
		goto err;
	rc = infix_services_init(&confd);
	if (rc)
		goto err;
	rc = ietf_hardware_init(&confd);
	if (rc)
		goto err;

	/* YOUR_INIT GOES HERE */

	return SR_ERR_OK;

err:
	ERROR("init failed: %s", sr_strerror(rc));
	if (confd.root)
		json_decref(confd.root);
	sr_unsubscribe(confd.sub);
	sr_unsubscribe(confd.fsub);

	return rc;
}

void sr_plugin_cleanup_cb(sr_session_ctx_t *session, void *priv)
{
	struct confd *ptr = (struct confd *)priv;

	sr_unsubscribe(ptr->sub);
	sr_unsubscribe(ptr->fsub);
	json_decref(ptr->root);
	closelog();
}
