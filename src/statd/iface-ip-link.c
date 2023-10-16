#include <stdio.h>
#include <stdlib.h>
#include <jansson.h>
#include <srx/lyx.h>

#include <srx/common.h>
#include <srx/helpers.h>

#include "shared.h"

static json_t *json_get_ip_link(const char *ifname)
{
	char cmd[512] = {}; /* Size is arbitrary */

	snprintf(cmd, sizeof(cmd), "ip -s -d -j link show dev %s 2>/dev/null", ifname);

	return json_get_output(cmd);
}

static const char *get_yang_operstate(const char *operstate)
{
	size_t i;
	struct {
		const char *kern;
		const char *yang;

	} map[] = {
		{"DOWN",                "down"},
		{"UP",                  "up"},
		{"DORMANT",             "dormant"},
		{"TESTING",             "testing"},
		{"LOWERLAYERDOWN",      "lower-layer-down"},
		{"NOTPRESENT",          "not-present"},
	};

	for (i = 0; i < sizeof(map) / sizeof(map[0]); i++) {
		if (strcmp(operstate, map[i].kern) != 0)
			continue;

		return map[i].yang;
	}

	return "unknown";
}

static const char *get_yang_link_type(char *xpath, json_t *iface)
{
	json_t *j_type;
	const char *type;

	j_type = json_object_get(iface, "link_type");
	if (!json_is_string(j_type)) {
		ERROR("Expected a JSON string for 'link_type'");
		/* This will throw a YANG / ly error */
		return "";
	}

	type = json_string_value(j_type);

	if (strcmp(type, "ether") == 0) {
		const char *kind;
		json_t *j_val;

		j_val = json_object_get(iface, "linkinfo");
		if (!j_val)
			return "infix-if-type:ethernet";

		j_val = json_object_get(j_val, "info_kind");
		if (!j_val)
			return "infix-if-type:ethernet";

		if (!json_is_string(j_val)) {
			ERROR("Expected a JSON string for 'info_kind'");
			return "infix-if-type:other";
		}
		kind = json_string_value(j_val);

		if (strcmp(kind, "veth") == 0)
			return "infix-if-type:veth";
		if (strcmp(kind, "vlan") == 0)
			return "infix-if-type:vlan";
		if (strcmp(kind, "bridge") == 0)
			return "infix-if-type:bridge";
		if (strcmp(kind, "dsa") == 0)
			return "infix-if-type:ethernet";

		/**
		 * We could return ethernetCsmacd here, but it might hide some
		 * special type that we actually want to explicitly identify.
		 */
		ERROR("Unable to determine info_kind for \"%s\"", xpath);

		return "infix-if-type:other";
	}

	if (strcmp(type, "loopback") == 0)
		return "infix-if-type:loopback";

	ERROR("Unable to determine infix-if-type for \"%s\"", xpath);

	return "infix-if-type:other";
}

static int ly_add_ip_link_stat(const struct ly_ctx *ctx, struct lyd_node **parent,
			       char *xpath, json_t *j_iface)
{
	struct lyd_node *node = NULL;
	char *stat_xpath;
	json_t *j_stat;
	json_t *j_val;
	int err;

	err = asprintf(&stat_xpath, "%s/statistics", xpath);
	if (err == -1) {
		ERROR("Error, creating statistics xpath");
		return SR_ERR_SYS;
	}

	j_stat = json_object_get(j_iface, "stats64");
	if (!j_stat) {
		ERROR("Didn't find 'stats64' in JSON data");
		goto err_out;
	}

	err = lyd_new_path(*parent, ctx, stat_xpath, NULL, 0, &node);
	if (err) {
		ERROR("Failed adding 'statistics' node, libyang error %d: %s",
		      err, ly_errmsg(ctx));
		goto err_out;
	}

	j_val = json_object_get(j_stat, "tx");
	if (!j_val) {
		ERROR("Didn't find 'tx' in JSON stats64 data");
		goto err_out;
	}

	j_val = json_object_get(j_val, "bytes");
	if (!j_val) {
		ERROR("Didn't find 'bytes' in JSON stats64 tx data");
		goto err_out;
	}
	if (!json_is_integer(j_val)) {
		ERROR("Didn't get integer for 'bytes' in JSON stats64 tx data");
		goto err_out;
	}
	err = lydx_new_path(ctx, &node, stat_xpath, "out-octets", "%lld",
			    json_integer_value(j_val));
	if (err) {
		ERROR("Error, adding 'out-octets' to data tree, libyang error %d", err);
		goto err_out;
	}

	j_val = json_object_get(j_stat, "rx");
	if (!j_val) {
		ERROR("Didn't find 'rx' in JSON stats64 data");
		goto err_out;
	}

	j_val = json_object_get(j_val, "bytes");
	if (!j_val) {
		ERROR("Didn't find 'bytes' in JSON stats64 rx data");
		goto err_out;
	}
	if (!json_is_integer(j_val)) {
		ERROR("Didn't get integer for 'bytes' in JSON stats64 rx data");
		goto err_out;
	}
	err = lydx_new_path(ctx, &node, stat_xpath, "in-octets", "%lld",
			    json_integer_value(j_val));
	if (err) {
		ERROR("Error, adding 'in-octets' to data tree, libyang error %d", err);
		goto err_out;
	}

	free(stat_xpath);

	return SR_ERR_OK;

err_out:
	free(stat_xpath);

	return SR_ERR_SYS;
}

static int ly_add_ip_link_br(const struct ly_ctx *ctx, struct lyd_node **parent,
			     char *xpath, json_t *j_iface)
{
	struct lyd_node *br_node = NULL;
	char *br_xpath;
	json_t *j_val;
	int err;

	j_val = json_object_get(j_iface, "master");
	if (!j_val) {
		/* Interface has no bridge */
		return SR_ERR_OK;
	}

	if (!json_is_string(j_val)) {
		ERROR("Expected a JSON string for bridge 'master'");
		return SR_ERR_SYS;
	}

	err = asprintf(&br_xpath, "%s/infix-interfaces:bridge-port", xpath);
	if (err == -1) {
		ERROR("Error, creating bridge xpath");
		return SR_ERR_SYS;
	}

	err = lyd_new_path(*parent, ctx, br_xpath, NULL, 0, &br_node);
	if (err) {
		ERROR("Failed adding 'bridge' node (%s), libyang error %d: %s",
		      br_xpath, err, ly_errmsg(ctx));
		free(br_xpath);
		return SR_ERR_LY;
	}

	err = lydx_new_path(ctx, &br_node, br_xpath, "bridge",
			"%s", json_string_value(j_val));
	if (err) {
		ERROR("Error, adding 'bridge' to data tree, libyang error %d", err);
		free(br_xpath);
		return SR_ERR_LY;
	}
	free(br_xpath);

	return SR_ERR_OK;
}

static int ip_link_kind_is_dsa(json_t *j_iface)
{
	json_t *j_linkinfo;
	json_t *j_val;

	j_linkinfo = json_object_get(j_iface, "linkinfo");
	if (!j_linkinfo)
		return 0;

	j_val = json_object_get(j_linkinfo, "info_kind");
	if (!j_val)
		return 0;

	if (!json_is_string(j_val))
		return 0;

	if (strcmp("dsa", json_string_value(j_val)) != 0)
		return 0;

	/* This interface has linkinfo -> info_kind = "dsa" */
	return 1;
}

static int ly_add_ip_link_parent(const struct ly_ctx *ctx, struct lyd_node **parent,
				 char *xpath, json_t *j_iface)
{
	json_t *j_val;
	int err;

	j_val = json_object_get(j_iface, "link");
	if (!j_val) {
		/* Interface has no parent */
		return SR_ERR_OK;
	}

	if (ip_link_kind_is_dsa(j_iface)) {
		/* Skip adding parent interface data for dsa child */
		return SR_ERR_OK;
	}

	if (!json_is_string(j_val)) {
		ERROR("Expected a JSON string for 'link'");
		return SR_ERR_SYS;
	}

	err = lydx_new_path(ctx, parent, xpath,
			    "ietf-if-extensions:parent-interface", "%s",
			    json_string_value(j_val));
	if (err) {
		ERROR("Error, adding 'link' to data tree, libyang error %d", err);
		return SR_ERR_LY;
	}

	return SR_ERR_OK;
}

static int ly_add_ip_link_data(const struct ly_ctx *ctx, struct lyd_node **parent,
			       char *xpath, json_t *j_iface)
{
	const char *val;
	const char *type;
	json_t *j_val;
	int err;

	j_val = json_object_get(j_iface, "ifindex");
	if (!json_is_integer(j_val)) {
		ERROR("Expected a JSON integer for 'ifindex'");
		return SR_ERR_SYS;
	}

	err = lydx_new_path(ctx, parent, xpath, "if-index", "%lld",
			    json_integer_value(j_val));
	if (err) {
		ERROR("Error, adding 'if-index' to data tree, libyang error %d", err);
		return SR_ERR_LY;
	}

	j_val = json_object_get(j_iface, "operstate");
	if (!json_is_string(j_val)) {
		ERROR("Expected a JSON string for 'operstate'");
		return SR_ERR_SYS;
	}

	val = get_yang_operstate(json_string_value(j_val));
	err = lydx_new_path(ctx, parent, xpath, "oper-status", val);
	if (err) {
		ERROR("Error, adding 'oper-status' to data tree, libyang error %d", err);
		return SR_ERR_LY;
	}

	err = ly_add_ip_link_stat(ctx, parent, xpath, j_iface);
	if (err) {
		ERROR("Error, adding 'stats64' to data tree");
		return err;
	}

	type = get_yang_link_type(xpath, j_iface);
	err = lydx_new_path(ctx, parent, xpath, "type", type);
	if (err) {
		ERROR("Error, adding 'type' to data tree, libyang error %d", err);
		return SR_ERR_LY;
	}

	j_val = json_object_get(j_iface, "address");
	if (!json_is_string(j_val)) {
		ERROR("Expected a JSON string for 'address'");
		return SR_ERR_SYS;
	}

	err = lydx_new_path(ctx, parent, xpath, "phys-address", json_string_value(j_val));
	if (err) {
		ERROR("Error, adding 'phys-address' to data tree, libyang error %d", err);
		return SR_ERR_LY;
	}

	err = ly_add_ip_link_br(ctx, parent, xpath, j_iface);
	if (err) {
		ERROR("Error, adding bridge to data tree, libyang error %d", err);
		return err;
	}

	err = ly_add_ip_link_parent(ctx, parent, xpath, j_iface);
	if (err) {
		ERROR("Error, adding parent iface to data tree, libyang error %d", err);
		return err;
	}

	return SR_ERR_OK;
}

int ly_add_ip_link(const struct ly_ctx *ctx, struct lyd_node **parent, char *ifname)
{
	char xpath[XPATH_MAX] = {};
	json_t *j_root;
	json_t *j_iface;
	int err;

	j_root = json_get_ip_link(ifname);
	if (!j_root) {
		ERROR("Error, parsing ip-link JSON");
		return SR_ERR_SYS;
	}
	if (json_array_size(j_root) != 1) {
		ERROR("Error, expected JSON array of single iface");
		json_decref(j_root);
		return SR_ERR_SYS;
	}

	j_iface = json_array_get(j_root, 0);

	snprintf(xpath, sizeof(xpath), "%s/interface[name='%s']",
		 XPATH_IFACE_BASE, ifname);

	err = ly_add_ip_link_data(ctx, parent, xpath, j_iface);
	if (err) {
		ERROR("Error, adding ip-link info for %s", ifname);
		json_decref(j_root);
		return err;
	}

	json_decref(j_root);

	return SR_ERR_OK;
}

/* Returns 1 if the group is "group", 0 if it's not and -1 on error */
int ip_link_check_group(char *ifname, const char *group)
{
	json_t *j_iface;
	json_t *j_root;
	json_t *j_val;

	j_root = json_get_ip_link(ifname);
	if (!j_root) {
		ERROR("Error, parsing ip-link JSON");
		return -1;
	}
	if (json_array_size(j_root) != 1) {
		ERROR("Error, expected JSON array of single iface");
		json_decref(j_root);
		return -1;
	}

	j_iface = json_array_get(j_root, 0);

	j_val = json_object_get(j_iface, "group");
	if (!json_is_string(j_val)) {
		ERROR("Error, expected a JSON string for 'group'");
		json_decref(j_root);
		return -1;
	}
	if (strcmp(json_string_value(j_val), group) == 0) {
		json_decref(j_root);
		return 1;
	}

	json_decref(j_root);

	return 0;
}
