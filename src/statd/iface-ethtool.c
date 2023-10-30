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

/* We print errors here, but don't return them */
static void ly_add_lld(const struct ly_ctx *ctx, struct lyd_node *node, char *xpath,
		       json_t *json, const char *yang, const char *ethtool)
{
	json_t *j_val;
	int err;

	j_val = json_object_get(json, ethtool);
	if (!j_val)
		return;

	if (!json_is_integer(j_val)) {
		ERROR("Error, expecting integer value for '%s'\n", ethtool);
		return;
	}

	err = lydx_new_path(ctx, &node, xpath, (char *)yang, "%lld",
			    json_integer_value(j_val));
	if (err)
		ERROR("Error, adding ethtool '%s' to data tree, libyang error %d", yang, err);
}

static uint64_t json_get_lld(json_t *json, int *found, const char *name)
{
	json_t *j_val;

	j_val = json_object_get(json, name);
	if (!j_val)
		return 0;

	if (!json_is_integer(j_val)) {
		ERROR("Error, expecting integer value for '%s'\n", name);
		return 0;
	}

	(*found)++;

	return json_integer_value(j_val);
}

static void ly_add_in_tot_frames(const struct ly_ctx *ctx, struct lyd_node *node,
				 char *xpath, json_t *j_mac, json_t *j_rmon)
{
	char *yang = "in-total-frames";
	long long int tot = 0;
	int found = 0;
	int err;

	tot += json_get_lld(j_mac, &found, "FramesReceivedOK");
	tot += json_get_lld(j_mac, &found, "FrameCheckSequenceErrors");
	tot += json_get_lld(j_mac, &found, "FramesLostDueToIntMACRcvError");
	tot += json_get_lld(j_mac, &found, "AlignmentErrors");
	tot += json_get_lld(j_rmon, &found, "etherStatsOversizePkts");
	tot += json_get_lld(j_rmon, &found, "etherStatsJabbers");

	 /* Don't add 0 counters for missing data (missing != 0) */
	if (!found)
		return;

	err = lydx_new_path(ctx, &node, xpath, yang, "%lld", tot);
	if (err)
		ERROR("Error, adding ethtool '%s', libyang error %d", yang, err);
}

static void ly_add_in_err_oz_frames(const struct ly_ctx *ctx, struct lyd_node *node,
				    char *xpath, json_t *j_rmon)
{
	char *yang = "in-error-oversize-frames";
	long long int tot = 0;
	int found = 0;
	int err;

	tot += json_get_lld(j_rmon, &found, "etherStatsOversizePkts");
	tot += json_get_lld(j_rmon, &found, "etherStatsJabbers");

	 /* Don't add 0 counters for missing data (missing != 0) */
	if (!found)
		return;

	err = lydx_new_path(ctx, &node, xpath, yang, "%lld", tot);
	if (err)
		ERROR("Error, adding ethtool '%s', libyang error %d", yang, err);
}

static int ly_add_eth_stats(const struct ly_ctx *ctx, struct lyd_node **parent,
			    const char *ifname, json_t *j_iface)
{
	char xpath_base[XPATH_BASE_MAX] = {};
	char xpath[XPATH_MAX] = {};
	struct lyd_node *frame = NULL;
	struct lyd_node *stat = NULL;
	struct lyd_node *eth = NULL;
	json_t *j_mac;
	json_t *j_rmon;
	int err;

	j_mac = json_object_get(j_iface, "eth-mac");
	if (!j_mac)
		return SR_ERR_OK;

	j_rmon = json_object_get(j_iface, "rmon");
	if (!j_rmon)
		return SR_ERR_OK;

	snprintf(xpath_base, sizeof(xpath_base),
		 "%s/interface[name='%s']/ieee802-ethernet-interface:ethernet",
		 XPATH_IFACE_BASE, ifname);

	err = lyd_new_path(*parent, ctx, xpath_base, NULL, 0, &eth);
	if (err) {
		ERROR("Failed adding 'eth' node (%s), libyang error %d: %s",
		      xpath_base, err, ly_errmsg(ctx));
		return SR_ERR_LY;
	}

	snprintf(xpath, sizeof(xpath), "%s/statistics", xpath_base);
	err = lyd_new_path(eth, ctx, xpath, NULL, 0, &stat);
	if (err) {
		ERROR("Failed adding 'stat' node (%s), libyang error %d: %s",
		      xpath, err, ly_errmsg(ctx));
		return SR_ERR_LY;
	}

	snprintf(xpath, sizeof(xpath), "%s/statistics/frame", xpath_base);
	err = lyd_new_path(stat, ctx, xpath, NULL, 0, &frame);
	if (err) {
		ERROR("Failed adding 'frame' node (%s), libyang error %d: %s",
		      xpath, err, ly_errmsg(ctx));
		return SR_ERR_LY;
	}

	ly_add_lld(ctx, frame, xpath, j_mac, "out-frames", "FramesTransmittedOK");
	ly_add_lld(ctx, frame, xpath, j_mac, "out-multicast-frames", "MulticastFramesXmittedOK");
	ly_add_lld(ctx, frame, xpath, j_mac, "out-broadcast-frames", "BroadcastFramesXmittedOK");
	ly_add_lld(ctx, frame, xpath, j_mac, "in-frames", "FramesReceivedOK");
	ly_add_lld(ctx, frame, xpath, j_mac, "in-multicast-frames", "MulticastFramesReceivedOK");
	ly_add_lld(ctx, frame, xpath, j_mac, "in-broadcast-frames", "BroadcastFramesReceivedOK");
	ly_add_lld(ctx, frame, xpath, j_mac, "in-error-fcs-frames", "FrameCheckSequenceErrors");
	ly_add_lld(ctx, frame, xpath, j_mac, "in-error-undersize-frames", "undersize_pkts");

	ly_add_lld(ctx, frame, xpath, j_mac, "in-error-mac-internal-frames",
		   "FramesLostDueToIntMACRcvError");

	ly_add_in_tot_frames(ctx, frame, xpath, j_mac, j_rmon);
	ly_add_in_err_oz_frames(ctx, frame, xpath, j_rmon);

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

