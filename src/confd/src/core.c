/* SPDX-License-Identifier: BSD-3-Clause */

#include <srx/srx_val.h>
#include <srx/common.h>
#include <srx/lyx.h>
#include "core.h"

struct confd confd;


static int startup_save(sr_session_ctx_t *session, uint32_t sub_id, const char *model,
			const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	/* skip in bootstrap, triggered by load script to initialize startup datastore */
	if (systemf("runlevel >/dev/null 2>&1"))
		return SR_ERR_OK;

	if (systemf("sysrepocfg -X/cfg/startup-config.cfg -d startup -f json"))
		return SR_ERR_SYS;

	return SR_ERR_OK;
}

static confd_dependency_t add_dependencies(struct lyd_node **diff, const char *xpath, const char *value)
{
	struct lyd_node *new_node = NULL;
	struct lyd_node *target = NULL;
	struct lyd_node *root = NULL;

	if (!lydx_get_xpathf(*diff, "%s", xpath)) {
		int rc;

		/* Create the path, potentially creating a new tree */
		rc = lyd_new_path(NULL, LYD_CTX(*diff), xpath, value, LYD_NEW_PATH_UPDATE, &new_node);
		if (rc != LY_SUCCESS || !new_node) {
			ERROR("lyd_new_path failed with rc=%d", rc);
			return CONFD_DEP_ERROR;
		}

		root = new_node;
		while (root->parent)
			root = lyd_parent(root);

		rc = lyd_merge_siblings(diff, root, LYD_MERGE_DESTRUCT);
		if (rc != LY_SUCCESS) {
			ERROR("lyd_merge_siblings failed with rc=%d", rc);
			lyd_free_tree(root);
			return CONFD_DEP_ERROR;
		}

		target = lydx_get_xpathf(*diff, "%s", xpath);
		if (target) {
			lyd_new_meta(LYD_CTX(target), target, NULL,
				     "yang:operation", "replace", false, NULL);
		} else {
			return CONFD_DEP_ERROR;
		}

		return CONFD_DEP_ADDED;
	}

	return CONFD_DEP_DONE;
}

static confd_dependency_t handle_dependencies(struct lyd_node **diff, struct lyd_node *config)
{
	struct lyd_node *dkeys, *dkey, *hostname;
	confd_dependency_t result = CONFD_DEP_DONE;
	const char *key_name;

	dkeys = lydx_get_descendant(*diff, "keystore", "symmetric-keys", "symmetric-key", NULL);

	LYX_LIST_FOR_EACH(dkeys, dkey, "symmetric-key") {
		struct ly_set *ifaces;
		uint32_t i;

		key_name = lydx_get_cattr(dkey, "name");
		ifaces = lydx_find_xpathf(config, "/ietf-interfaces:interfaces/interface[infix-interfaces:wifi/secret='%s']", key_name);
		if (ifaces && ifaces->count > 0) {
			for (i = 0; i < ifaces->count; i++) {
				struct lyd_node *iface = ifaces->dnodes[i];
				const char *ifname;
				char xpath[256];
				ifname = lydx_get_cattr(iface, "name");
				snprintf(xpath, sizeof(xpath), "/ietf-interfaces:interfaces/interface[name='%s']/infix-interfaces:wifi/secret", ifname);
				result = add_dependencies(diff, xpath, key_name);
				if (result == CONFD_DEP_ERROR) {
					ERROR("Failed to add wifi node to diff for interface %s", ifname);
					ly_set_free(ifaces, NULL);
					return result;
				}
			}
			ly_set_free(ifaces, NULL);
		}
	}

	dkeys = lydx_get_descendant(*diff, "keystore", "asymmetric-keys", "asymmetric-key", NULL);
	LYX_LIST_FOR_EACH(dkeys, dkey, "asymmetric-key") {
		struct ly_set *hostkeys;
		uint32_t i;

		key_name = lydx_get_cattr(dkey, "name");
		hostkeys = lydx_find_xpathf(config, "/infix-services:ssh/hostkey[.='%s']", key_name);
		if (hostkeys && hostkeys->count > 0) {
			for (i = 0; i < hostkeys->count; i++) {
				char xpath[256];
				snprintf(xpath, sizeof(xpath), "/infix-services:ssh/hostkey[.='%s']", key_name);
				result = add_dependencies(diff, xpath, key_name);
				if (result == CONFD_DEP_ERROR) {
					ERROR("Failed to add ssh hostkey to diff for key %s", key_name);
					ly_set_free(hostkeys, NULL);
					return result;
				}
			}
			ly_set_free(hostkeys, NULL);
		}
	}

	hostname = lydx_get_xpathf(*diff, "/ietf-system:system/hostname");
	if (hostname) {
		struct lyd_node *mdns, *dhcp_server;

		dhcp_server = lydx_get_xpathf(config, "/infix-dhcp-server:dhcp-server/enabled");
		if(dhcp_server && lydx_is_enabled(dhcp_server, "enabled")) {
			result = add_dependencies(diff, "/infix-dhcp-server:dhcp-server/enabled", "true");
			if (result == CONFD_DEP_ERROR) {
				ERROR("Failed to add dhcp-server to diff on hostname change");
				return result;
			}
		}
		mdns = lydx_get_xpathf(config, "/infix-services:mdns");
		if (mdns && lydx_is_enabled(mdns, "enabled")) {
			result = add_dependencies(diff, "/infix-services:mdns/enabled", "true");
			if (result == CONFD_DEP_ERROR) {
				ERROR("Failed to add mdns to diff on hostname change");
				return result;
			}
		}
	}

	return result;
}

static int change_cb(sr_session_ctx_t *session, uint32_t sub_id, const char *model_name,
		     const char *xpath, sr_event_t event, uint32_t request_id, void *_confd)
{
	struct lyd_node *diff = NULL, *config = NULL;
	static sr_event_t last_event = -1;
	struct confd *confd = _confd;
	static uint32_t last_request = 0;
	confd_dependency_t result;
	sr_data_t *cfg = NULL;
	int rc = SR_ERR_OK;
	int max_dep = 10;

	if (request_id == last_request && last_event == event)
		return SR_ERR_OK;
	last_request = request_id;
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
		while ((result = handle_dependencies(&diff, config)) != CONFD_DEP_DONE) {
			if (max_dep == 0) {
				ERROR("Max dependency depth reached");
				return SR_ERR_INTERNAL;
			}
			if (result == CONFD_DEP_ERROR) {
				ERROR("Failed to add dependencies");
				return SR_ERR_INTERNAL;
			}
			max_dep--;
		}
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

static inline int subscribe_model(char *model, struct confd *confd, int flags)
{
	return  sr_module_change_subscribe(confd->session, model, "//.", change_cb, confd,
					   CB_PRIO_PRIMARY, SR_SUBSCR_CHANGE_ALL_MODULES |
					   SR_SUBSCR_DEFAULT | flags, &confd->sub) &&
		sr_module_change_subscribe(confd->startup, model, "//.", core_startup_save, NULL,
					   CB_PRIO_PASSIVE, SR_SUBSCR_CHANGE_ALL_MODULES |
					   SR_SUBSCR_PASSIVE, &confd->sub);
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

	/* The startup datastore is used for the startup_save() hook */
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

	rc = subscribe_model("ietf-interfaces", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-interfaces");
		goto err;
	}
	rc = subscribe_model("ietf-netconf-acm", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-netconf-acm");
		goto err;
	}
	rc = subscribe_model("infix-dhcp-client", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to infix-dhcp-client");
		goto err;
	}
	rc = subscribe_model("ietf-keystore", &confd, SR_SUBSCR_UPDATE);
	if (rc) {
		ERROR("Failed to subscribe to ietf-keystore");
		goto err;
	}
	rc = subscribe_model("infix-services", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to infix-services");
		goto err;
	}
	rc = subscribe_model("ietf-system", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-system");
		goto err;
	}
	rc = subscribe_model("ieee802-dot1ab-lldp", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ieee802-dot1ab-lldp");
		goto err;
	}
#ifdef CONTAINERS
	rc = subscribe_model("infix-containers", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to infix-containers");
		goto err;
	}
#endif
	rc = subscribe_model("infix-dhcp-server", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to infix-dhcp-server");
		goto err;
	}
	rc = subscribe_model("ietf-routing", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-routing");
		goto err;
	}
	rc = subscribe_model("ietf-hardware", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-hardware");
		goto err;
	}
	rc = subscribe_model("infix-firewall", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to infix-firewall");
		goto err;
	}
	rc = subscribe_model("infix-meta", &confd, SR_SUBSCR_UPDATE);
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
