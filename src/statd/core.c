/* SPDX-License-Identifier: BSD-3-Clause */

#include <jansson.h>
#include <stdio.h>
#include <ctype.h>
#include <srx/common.h>
#include <srx/helpers.h>
#include <srx/lyx.h>

/* TODO: break out and find a reasonable value */
#define XPATH_MAX PATH_MAX
#define XPATH_IFACE_BASE "/ietf-interfaces:interfaces"

static void to_lowercase(char *str) {
	for (int i = 0; str[i]; i++)
		str[i] = tolower((unsigned char)str[i]);
}

static json_t *get_ip_link_json(char *ifname)
{
	char cmd[512] = {}; /* Size is arbitrary */
	json_error_t j_err;
	json_t *j_root;
	FILE *proc;

	if (ifname)
		snprintf(cmd, sizeof(cmd), "ip -d -j link show dev %s 2>/dev/null", ifname);
	else
		snprintf(cmd, sizeof(cmd), "ip -d -j link show 2>/dev/null");

	proc = popenf("re", cmd);
	if (!proc) {
		ERROR("Error running ip link command");
		return NULL;
	}

	j_root = json_loadf(proc, 0, &j_err);
	pclose(proc);
	if (!j_root) {
		ERROR("Error parsing ip link JSON");
		return NULL;
	}

	if (!json_is_array(j_root)) {
		ERROR("Expected a JSON array from ip link");
		json_decref(j_root);
		return NULL;
	}

	return j_root;
}

static int add_ip_link_data(const struct ly_ctx *ctx, struct lyd_node **parent,
			    char *xpath, json_t *iface)
{
	json_t *j_val;
	char *val;
	int err;

	j_val = json_object_get(iface, "ifindex");
	if (!json_is_integer(j_val)) {
		ERROR("Expected a JSON integer for 'ifindex'");
		return SR_ERR_SYS;
	}

	err = lydx_new_path(ctx, parent, xpath, "if-index", "%lld",
			    json_integer_value(j_val));
	if (err) {
		ERROR("Error adding 'if-index' to data tree, libyang error %d", err);
		return SR_ERR_LY;
	}

	j_val = json_object_get(iface, "operstate");
	if (!json_is_string(j_val)) {
		ERROR("Expected a JSON string for 'operstate'");
		return SR_ERR_SYS;
	}

	val = strdup(json_string_value(j_val));
	if (!val)
		return SR_ERR_SYS;

	to_lowercase(val);

	err = lydx_new_path(ctx, parent, xpath, "oper-status", val);
	if (err) {
		ERROR("Error adding 'oper-status' to data tree, libyang error %d", err);
		free(val);
		return SR_ERR_LY;
	}
	free(val);

	return SR_ERR_OK;
}

static int add_ip_link(const struct ly_ctx *ctx, struct lyd_node **parent, char *ifname)
{
	char xpath[XPATH_MAX] = {};
	json_t *j_root;
	json_t *j_iface;
	int err;

	j_root = get_ip_link_json(ifname);
	if (!j_root) {
		ERROR("Error parsing ip-link JSON");
		return SR_ERR_SYS;
	}
	if (json_array_size(j_root) != 1) {
		ERROR("Error expected JSON array of single iface");
		json_decref(j_root);
		return SR_ERR_SYS;
	}

	j_iface = json_array_get(j_root, 0);

	snprintf(xpath, sizeof(xpath), "%s/interface[name='%s']",
		 XPATH_IFACE_BASE, ifname);

	err = add_ip_link_data(ctx, parent, xpath, j_iface);
	if (err) {
		ERROR("Error adding ip-link info for %s", ifname);
		json_decref(j_root);
		return err;
	}
	json_decref(j_root);

	return SR_ERR_OK;
}

static int iface_cb(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		    const char *path, const char *request_path, uint32_t request_id,
		    struct lyd_node **parent, void *priv)
{
	const struct ly_ctx *ctx;
	char *ifname = priv;
	sr_conn_ctx_t *con;
	int err;

	con = sr_session_get_connection(session);
	if (!con)
		return SR_ERR_INTERNAL;

	ctx = sr_acquire_context(con);
	if (!ctx)
		return SR_ERR_INTERNAL;

	err = add_ip_link(ctx, parent, ifname);

	sr_release_context(con);

	return err;
}

static int reg_iface_handlers(sr_session_ctx_t *session)
{
	json_t *j_iface;
	json_t *j_root;
	size_t i;

	j_root = get_ip_link_json(NULL);
	if (!j_root) {
		ERROR("Error parsing ip-link JSON");
		return SR_ERR_SYS;
	}

	json_array_foreach(j_root, i, j_iface) {
		sr_subscription_ctx_t *sub = NULL;
		char xpath[XPATH_MAX] = {};
		json_t *j_ifname;
		char *ifname;
		int err;

		j_ifname = json_object_get(j_iface, "ifname");
		if (!json_is_string(j_ifname)) {
			ERROR("Got unexpected JSON type for 'ifname'");
			continue;
		}

		/* This shall be freed by callback cleanup during normal op */
		ifname = strdup(json_string_value(j_ifname));
		if (!ifname) {
			json_decref(j_root);
			return SR_ERR_SYS;
		}

		snprintf(xpath, sizeof(xpath), "%s/interface[name='%s']",
			 XPATH_IFACE_BASE, ifname);

		err = sr_oper_get_subscribe(session, "ietf-interfaces", xpath,
					   iface_cb, ifname, 0 | SR_SUBSCR_DEFAULT, &sub);
		if (err) {
			ERROR("Failed subscribing to %s oper: %s", xpath, sr_strerror(err));
			json_decref(j_root);
			free(ifname);
			return err;
		}

	}
	json_decref(j_root);

	return SR_ERR_OK;
}


int sr_plugin_init_cb(sr_session_ctx_t *session, void **priv)
{
	sr_session_ctx_t *op;
	sr_conn_ctx_t *conn;
	int err;

	openlog("statd", LOG_USER, 0);
	INFO("Sysrepo callback hello");

	conn = sr_session_get_connection(session);
	if (!conn) {
		ERROR("Error getting OP session");
		return SR_ERR_INTERNAL;
	}

	err = sr_session_start(conn, SR_DS_OPERATIONAL, &op);
	if (err) {
		ERROR("Error starting OP session: %s", sr_strerror(err));
		return err;
	}

	err = reg_iface_handlers(session);
	if (err) {
		ERROR("Error registering OP handlers: %s", sr_strerror(err));
		return err;
	}

	return SR_ERR_OK;
}

void sr_plugin_cleanup_cb(sr_session_ctx_t *session, void *priv)
{
	sr_unsubscribe((sr_subscription_ctx_t *)priv);
	/* Free ifname */
	free(priv);
	closelog();
}
