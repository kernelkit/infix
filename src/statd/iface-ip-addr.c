#include <stdio.h>
#include <stdlib.h>
#include <jansson.h>
#include <srx/lyx.h>

#include <srx/common.h>
#include <srx/helpers.h>

#include "shared.h"

static json_t *json_get_ip_addr(const char *ifname)
{
	char cmd[512] = {}; /* Size is arbitrary */

	if (ifname)
		snprintf(cmd, sizeof(cmd), "ip -j addr show dev %s 2>/dev/null", ifname);
	else
		snprintf(cmd, sizeof(cmd), "ip -j addr show 2>/dev/null");

	return json_get_output(cmd);
}

static const char *get_yang_origin(const char *protocol)
{
	size_t i;
	struct {
		const char *kern;
		const char *yang;

	} map[] = {
		{"kernel_ll",	"link-layer"},
		{"static",	"static"},
		{"dhcp",	"dhcp"},
		{"random",	"random"},
	};

	for (i = 0; i < sizeof(map) / sizeof(map[0]); i++) {
		if (strcmp(protocol, map[i].kern) != 0)
			continue;

		return map[i].yang;
	}

	return "other";
}

static int ly_add_ip_mtu(const struct ly_ctx *ctx, struct lyd_node *node,
			   char *xpath, json_t *j_iface, char *ifname)
{
	json_t *j_val;
	int err;

	/**
	 * TODO: Not sure how to handle loopback MTU (65536) which is
	 * out of bounds for both YANG and uint16_t. For now, we skip it.
	 */
	if (strcmp(ifname, "lo") == 0)
		return SR_ERR_OK;

	j_val = json_object_get(j_iface, "mtu");
	if (!json_is_integer(j_val)) {
		ERROR("Expected a JSON integer for 'mtu'");
		return SR_ERR_SYS;
	}

	err = lydx_new_path(ctx, &node, xpath, "mtu", "%lld",
			    json_integer_value(j_val));
	if (err) {
		ERROR("Error, adding 'mtu' to data tree, libyang error %d", err);
		return SR_ERR_LY;
	}

	return SR_ERR_OK;
}

static int ly_add_ip_addr_origin(const struct ly_ctx *ctx, struct lyd_node *addr_node,
				      char *addr_xpath, json_t *j_addr)
{
	const char *origin;
	json_t *j_val;
	int err;

	/* It's okey not to have an origin */
	j_val = json_object_get(j_addr, "protocol");
	if (!j_val)
		return SR_ERR_OK;

	if (!json_is_string(j_val)) {
		ERROR("Expected a JSON string for ip 'protocol'");
		return SR_ERR_SYS;
	}

	origin = get_yang_origin(json_string_value(j_val));

	/**
	 * kernel_ll/link-layer only has a link-layer origin if its address is
	 * based on the link layer address (addrgenmode eui64).
	 */
	if (strcmp(origin, "link-layer") == 0) {
		j_val = json_object_get(j_addr, "stable-privacy");
		if (j_val && json_is_boolean(j_val) && json_boolean_value(j_val))
			origin = "random";
	}

	err = lydx_new_path(ctx, &addr_node, addr_xpath, "origin", "%s", origin);
	if (err) {
		ERROR("Error, adding ip 'origin' to data tree, libyang error %d", err);
		return SR_ERR_LY;
	}

	return SR_ERR_OK;
}

static int ly_add_ip_addr_info(const struct ly_ctx *ctx, struct lyd_node *node,
			       char *xpath, json_t *j_addr)
{
	struct lyd_node *addr_node = NULL;
	char *addr_xpath;
	const char *ip;
	json_t *j_val;
	int err;

	j_val = json_object_get(j_addr, "local");
	if (!json_is_string(j_val)) {
		ERROR("Expected a JSON string for ip 'local'");
		return SR_ERR_SYS;
	}
	ip = json_string_value(j_val);

	err = asprintf(&addr_xpath, "%s/address[ip='%s']", xpath, ip);
	if (err == -1) {
		ERROR("Error, creating address xpath");
		return SR_ERR_SYS;
	}

	err = lyd_new_path(node, ctx, addr_xpath, NULL, 0, &addr_node);
	if (err) {
		ERROR("Failed adding ip 'address' node (%s), libyang error %d: %s",
		      addr_xpath, err, ly_errmsg(ctx));
		free(addr_xpath);
		return SR_ERR_LY;
	}

	j_val = json_object_get(j_addr, "prefixlen");
	if (!json_is_integer(j_val)) {
		ERROR("Expected a JSON integer for ip 'prefixlen'");
		free(addr_xpath);
		return SR_ERR_SYS;
	}

	err = lydx_new_path(ctx, &addr_node, addr_xpath, "prefix-length",
			    "%lld", json_integer_value(j_val));
	if (err) {
		ERROR("Error, adding ip 'prefix-length' to data tree, libyang error %d", err);
		free(addr_xpath);
		return SR_ERR_LY;
	}

	err = ly_add_ip_addr_origin(ctx, addr_node, addr_xpath, j_addr);

	free(addr_xpath);

	return err;
}


static int ly_add_ip_data(const struct ly_ctx *ctx, struct lyd_node **parent,
			  char *xpath, int version, json_t *j_iface, char *ifname)
{
	struct lyd_node *node = NULL;
	json_t *j_val;
	json_t *j_addr;
	size_t index;
	int err;

	err = lyd_new_path(*parent, ctx, xpath, NULL, 0, &node);
	if (err) {
		ERROR("Failed adding 'ip' node (%s), libyang error %d: %s",
		      xpath, err, ly_errmsg(ctx));
		return SR_ERR_LY;
	}

	err = ly_add_ip_mtu(ctx, node, xpath, j_iface, ifname);
	if (err) {
		ERROR("Error, adding ip MTU");
		return err;
	}

	j_val = json_object_get(j_iface, "addr_info");
	if (!json_is_array(j_val)) {
		ERROR("Expected a JSON array for 'addr_info'");
		return SR_ERR_SYS;
	}

	json_array_foreach(j_val, index, j_addr) {
		json_t *j_family;

		j_family = json_object_get(j_addr, "family");
		if (!json_is_string(j_family)) {
			ERROR("Expected a JSON string for ip 'family'");
			return SR_ERR_SYS;
		}

		if (version == 4 && (strcmp(json_string_value(j_family), "inet") == 0)) {
			err = ly_add_ip_addr_info(ctx, node, xpath, j_addr);
			if (err) {
				ERROR("Error, adding  address");
				return err;
			}
		} else if (version == 6 && (strcmp(json_string_value(j_family), "inet6") == 0)) {
			err = ly_add_ip_addr_info(ctx, node, xpath, j_addr);
			if (err) {
				ERROR("Error, adding  address");
				return err;
			}
		}
	}

	return SR_ERR_OK;
}


int ly_add_ip_addr(const struct ly_ctx *ctx, struct lyd_node **parent, char *ifname)
{
	char xpath[XPATH_MAX] = {};
	json_t *j_root;
	json_t *j_iface;
	int err;

	j_root = json_get_ip_addr(ifname);
	if (!j_root) {
		ERROR("Error, parsing ip-addr JSON");
		return SR_ERR_SYS;
	}
	if (json_array_size(j_root) != 1) {
		ERROR("Error, expected JSON array of single iface");
		json_decref(j_root);
		return SR_ERR_SYS;
	}

	j_iface = json_array_get(j_root, 0);

	snprintf(xpath, sizeof(xpath), "%s/interface[name='%s']/ietf-ip:ipv4",
		 XPATH_IFACE_BASE, ifname);

	err = ly_add_ip_data(ctx, parent, xpath, 4, j_iface, ifname);
	if (err) {
		ERROR("Error, adding ipv4 addr info for %s", ifname);
		json_decref(j_root);
		return err;
	}

	snprintf(xpath, sizeof(xpath), "%s/interface[name='%s']/ietf-ip:ipv6",
		 XPATH_IFACE_BASE, ifname);

	err = ly_add_ip_data(ctx, parent, xpath, 6, j_iface, ifname);
	if (err) {
		ERROR("Error, adding ipv6 addr info for %s", ifname);
		json_decref(j_root);
		return err;
	}

	json_decref(j_root);

	return SR_ERR_OK;
}

