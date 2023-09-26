#include <stdio.h>
#include <stdlib.h>
#include <jansson.h>
#include <srx/lyx.h>

#include <srx/common.h>
#include <srx/helpers.h>

#include "shared.h"

static json_t *json_get_ethtool(const char *ifname)
{
	char cmd[512] = {}; /* Size is arbitrary */

	snprintf(cmd, sizeof(cmd), "ethtool --json -S %s --all-groups 2>/dev/null", ifname);

	return json_get_output(cmd);
}

static int ly_add_eth_stats(const struct ly_ctx *ctx, struct lyd_node **parent,
			    const char *ifname, json_t *j_iface)
{
	struct {
		const char *ethtool;
		char *yang;

	} map[] = {
		{"FramesTransmittedOK",	"out-frames"},
		{"FramesReceivedOK",	"in-frames"},
	};
	json_t *j_mac;
	json_t *j_val;
	char xpath_base[XPATH_BASE_MAX] = {};
	char xpath[XPATH_MAX] = {};
	struct lyd_node *frame_node = NULL;
	struct lyd_node *stat_node = NULL;
	struct lyd_node *eth_node = NULL;
	size_t i;
	int err;

	j_mac = json_object_get(j_iface, "eth-mac");
	if (!j_mac)
		return SR_ERR_OK;

	snprintf(xpath_base, sizeof(xpath_base),
		 "%s/interface[name='%s']/ieee802-ethernet-interface:ethernet",
		 XPATH_IFACE_BASE, ifname);

	err = lyd_new_path(*parent, ctx, xpath_base, NULL, 0, &eth_node);
	if (err) {
		ERROR("Failed adding 'eth' node (%s), libyang error %d: %s",
		      xpath_base, err, ly_errmsg(ctx));
		return SR_ERR_LY;
	}

	snprintf(xpath, sizeof(xpath), "%s/statistics", xpath_base);
	err = lyd_new_path(eth_node, ctx, xpath, NULL, 0, &stat_node);
	if (err) {
		ERROR("Failed adding 'stat' node (%s), libyang error %d: %s",
		      xpath, err, ly_errmsg(ctx));
		return SR_ERR_LY;
	}

	snprintf(xpath, sizeof(xpath), "%s/statistics/frame", xpath_base);
	err = lyd_new_path(stat_node, ctx, xpath, NULL, 0, &frame_node);
	if (err) {
		ERROR("Failed adding 'frame' node (%s), libyang error %d: %s",
		      xpath, err, ly_errmsg(ctx));
		return SR_ERR_LY;
	}

	for (i = 0; i < sizeof(map) / sizeof(map[0]); i++) {
		j_val = json_object_get(j_mac, map[i].ethtool);
		if (!j_val)
			continue;

		if (!json_is_integer(j_val)) {
			ERROR("Error, expecting integer value for '%s'\n", map[i].ethtool);
			return SR_ERR_SYS;
		}
		err = lydx_new_path(ctx, &frame_node, xpath, map[i].yang,
				    "%lld", json_integer_value(j_val));
		if (err) {
			ERROR("Error, adding ethtool '%s' to data tree, libyang error %d",
			      map[i].yang, err);
			return SR_ERR_LY;
		}
	}

	return SR_ERR_OK;
}

int ly_add_ethtool(const struct ly_ctx *ctx, struct lyd_node **parent, char *ifname)
{
	json_t *j_iface;
	json_t *j_root;
	int err;

	j_root = json_get_ethtool(ifname);
	if (!j_root) {
		ERROR("Error, parsing ethtool JSON");
		return SR_ERR_SYS;
	}
	if (json_array_size(j_root) != 1) {
		ERROR("Error, expected JSON array of single iface");
		json_decref(j_root);
		return SR_ERR_SYS;
	}

	j_iface = json_array_get(j_root, 0);
	if (!j_iface) {
		ERROR("Error, getting JSON interface");
		json_decref(j_root);
		return SR_ERR_SYS;
	}

	err = ly_add_eth_stats(ctx, parent, ifname, j_iface);
	if (err) {
		ERROR("Error, adding ethtool statistics data");
		json_decref(j_root);
		return err;
	}

	json_decref(j_root);

	return SR_ERR_OK;
}

