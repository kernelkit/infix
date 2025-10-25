/* SPDX-License-Identifier: BSD-3-Clause */

#include <srx/srx_val.h>
#include <srx/common.h>
#include <srx/lyx.h>
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


	return SR_ERR_OK;
}

static int change_cb(sr_session_ctx_t *session, uint32_t sub_id, const char *module_name,
		     const char *xpath, sr_event_t event, uint32_t request_id, void *_confd)
{
	struct lyd_node *diff = NULL, *config = NULL;
	static uint32_t last_event = -1;
	struct confd *confd = _confd;
	static uint32_t last_id = 0;
	confd_dependency_t result;
	sr_data_t *cfg = NULL;
	int rc = SR_ERR_OK;

	if (request_id == last_id && last_event == event)
		return SR_ERR_OK;
	last_id = request_id;
	last_event = event;

	if (event == SR_EV_CHANGE || event == SR_EV_DONE) {
		rc = srx_get_diff(session, &diff);
		if (rc != SR_ERR_OK) {
			ERROR("Failed to get diff: %d", rc);
			return rc;
		}
		rc = sr_get_data(session, "//.", 0, 0, 0, &cfg);
		if (rc || !cfg)
			goto free_diff;

		config = cfg->tree;
#if 0
		/* Debug: print diff to file */
		FILE *f = fopen("/tmp/confd-diff.json", "w");
		if (f) {
			lyd_print_file(f, diff, LYD_JSON, LYD_PRINT_WITHSIBLINGS);
			fclose(f);
		}
#endif
	}

	/* ietf-interfaces */
	if ((rc = ietf_interfaces_change(session, config, diff, event, confd)))
		goto free_diff;

	/* infix-dhcp-client*/
	if ((rc = infix_dhcp_client_change(session, config, diff, event, confd)))
		goto free_diff;

	/* ietf-keystore */
	if ((rc = ietf_keystore_change(session, config, diff, event, confd)))
		goto free_diff;

	/* infix-services */
	if ((rc = infix_services_change(session, config, diff, event, confd)))
		goto free_diff;

	/* ietf-syslog*/
	if ((rc = ietf_syslog_change(session, config, diff, event, confd)))
		goto free_diff;

	/* ietf-system */
	if ((rc = ietf_system_change(session, config, diff, event, confd)))
		goto free_diff;

	/* infix-containers */
	if ((rc = infix_containers_change(session, config, diff, event, confd)))
		goto free_diff;

	/* ietf-hardware */
	if ((rc = ietf_hardware_change(session, config, diff, event, confd)))
		goto free_diff;

	/* ietf-routing */
	if ((rc = ietf_routing_change(session, config, diff, event, confd)))
		goto free_diff;

	/* infix-dhcp-server */
	if ((rc = infix_dhcp_server_change(session, config, diff, event, confd)))
		goto free_diff;

	/* infix-firewall */
	if ((rc = infix_firewall_change(session, config, diff, event, confd)))
		goto free_diff;

	if (cfg)
		sr_release_data(cfg);

	if (event == SR_EV_DONE) {
		/* skip reload in bootstrap, implicit reload in runlevel change */
		if (systemf("runlevel >/dev/null 2>&1")) {
			/* trigger any tasks waiting for confd to have applied *-config */
			system("initctl -nbq cond set bootstrap");
			return SR_ERR_OK;
		}

		if (systemf("initctl -b reload")) {
			EMERG("initctl reload: failed applying new configuration!");
			return SR_ERR_SYS;
		}

		AUDIT("The new configuration has been applied.");
	}
free_diff:
      lyd_free_tree(diff);
      return rc;
}

static int subscribe_module(char *model, struct confd *confd, int flags) {
	return sr_module_change_subscribe(confd->session, model, "//.", change_cb, confd, CB_PRIO_PRIMARY, SR_SUBSCR_CHANGE_ALL_MODULES | SR_SUBSCR_DEFAULT | flags, &confd->sub);
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

	rc = subscribe_module("ietf-interfaces", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-interfaces");
		goto err;
	}
	rc = subscribe_module("ietf-netconf-acm", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-netconf-acm");
		goto err;
	}
	rc = subscribe_module("infix-dhcp-client", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to infix-dhcp-client");
		goto err;
	}
	rc = subscribe_module("ietf-keystore", &confd, SR_SUBSCR_UPDATE);
	if (rc) {
		ERROR("Failed to subscribe to ietf-keystore");
		goto err;
	}
	rc = subscribe_module("infix-services", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to infix-services");
		goto err;
	}
	rc = subscribe_module("ietf-system", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-system");
		goto err;
	}
	rc = subscribe_module("ieee802-dot1ab-lldp", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ieee802-dot1ab-lldp");
		goto err;
	}
#ifdef CONTAINERS
	rc = subscribe_module("infix-containers", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to infix-containers");
		goto err;
	}
#endif
	rc = subscribe_module("infix-dhcp-server", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to infix-dhcp-server");
		goto err;
	}
	rc = subscribe_module("ietf-routing", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-routing");
		goto err;
	}
	rc = subscribe_module("ietf-hardware", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-hardware");
		goto err;
	}
	rc = subscribe_module("infix-firewall", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to infix-firewall");
		goto err;
	}
	rc = subscribe_module("infix-meta", &confd, SR_SUBSCR_UPDATE);
	if (rc) {
		ERROR("Failed to subscribe to infix-meta");
		goto err;
	}

	rc = ietf_system_rpc_init(&confd);
	if (rc)
		goto err;
	rc = infix_containers_rpc_init(&confd);
	if (rc)
		goto err;
	rc = infix_dhcp_server_rpc_init(&confd);
	if (rc)
		goto err;

	rc = infix_factory_rpc_init(&confd);
	if (rc)
		goto err;

	rc = ietf_factory_default_rpc_init(&confd);
	if (rc)
		goto err;

	rc = infix_firewall_rpc_init(&confd);
	if (rc)
		goto err;

	rc = infix_system_sw_rpc_init(&confd);
	if (rc)
		goto err;

	/* Candidate infer configurations */
	rc = ietf_hardware_candidate_init(&confd);
	if (rc)
		goto err;

	rc = infix_firewall_candidate_init(&confd);
	if (rc)
		goto err;

	rc = infix_dhcp_server_candidate_init(&confd);
	if (rc)
		goto err;

	rc = infix_dhcp_client_candidate_init(&confd);
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
