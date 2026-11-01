/* SPDX-License-Identifier: BSD-3-Clause */

#include <ctype.h>
#include <fnmatch.h>
#include <stdbool.h>
#include <stdint.h>
#include <inttypes.h>
#include <jansson.h>
#include <arpa/inet.h>
#include <net/if.h>
#include <linux/ethtool.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "interfaces.h"

/*
 * Map IEEE pmd-type identity suffixes (everything after
 * "ieee802-ethernet-phy-type:pmd-type-") to the Linux
 * ETHTOOL_LINK_MODE_*_BIT_* indices that ethtool's --advertise mask
 * uses.  Half/full are separate bits in the kernel API but a single
 * PMD identity in IEEE; the half_bit field is -1 for PMDs that have
 * no half-duplex variant (everything past 1G).
 *
 * Where the kernel collapses several IEEE variants into one
 * "family" bit (e.g. 1000baseX covers LX/SX/ZX/CX) the same bit
 * appears in multiple rows by design — selecting any of them yields
 * the same on-wire behaviour because ethtool can't distinguish them
 * either.
 */
struct pmd_link_mode {
	const char *pmd;
	int speed_mbps;
	int half_bit;
	int full_bit;
};

#define NO_BIT (-1)

static const struct pmd_link_mode pmd_link_modes[] = {
	{"10BASE-T",      10,    ETHTOOL_LINK_MODE_10baseT_Half_BIT,
				 ETHTOOL_LINK_MODE_10baseT_Full_BIT},
	{"100BASE-TX",    100,   ETHTOOL_LINK_MODE_100baseT_Half_BIT,
				 ETHTOOL_LINK_MODE_100baseT_Full_BIT},
	{"100BASE-FX",    100,   NO_BIT, ETHTOOL_LINK_MODE_100baseFX_Full_BIT},
	{"1000BASE-T",    1000,  ETHTOOL_LINK_MODE_1000baseT_Half_BIT,
				 ETHTOOL_LINK_MODE_1000baseT_Full_BIT},
	/* 1000baseX_Full covers the LX/SX/ZX/CX family in the kernel —
	 * the API can't distinguish them; selecting any yields the same
	 * on-wire behaviour. */
	{"1000BASE-LX",   1000,  NO_BIT, ETHTOOL_LINK_MODE_1000baseX_Full_BIT},
	{"1000BASE-SX",   1000,  NO_BIT, ETHTOOL_LINK_MODE_1000baseX_Full_BIT},
	{"1000BASE-ZX",   1000,  NO_BIT, ETHTOOL_LINK_MODE_1000baseX_Full_BIT},
	{"1000BASE-CX",   1000,  NO_BIT, ETHTOOL_LINK_MODE_1000baseX_Full_BIT},
	{"2.5GBASE-T",    2500,  NO_BIT, ETHTOOL_LINK_MODE_2500baseT_Full_BIT},
	{"2.5GBASE-X",    2500,  NO_BIT, ETHTOOL_LINK_MODE_2500baseX_Full_BIT},
	{"5GBASE-T",      5000,  NO_BIT, ETHTOOL_LINK_MODE_5000baseT_Full_BIT},
	{"10GBASE-T",     10000, NO_BIT, ETHTOOL_LINK_MODE_10000baseT_Full_BIT},
	{"10GBASE-SR",    10000, NO_BIT, ETHTOOL_LINK_MODE_10000baseSR_Full_BIT},
	{"10GBASE-LR",    10000, NO_BIT, ETHTOOL_LINK_MODE_10000baseLR_Full_BIT},
	{"10GBASE-LRM",   10000, NO_BIT, ETHTOOL_LINK_MODE_10000baseLRM_Full_BIT},
	{"10GBASE-ER",    10000, NO_BIT, ETHTOOL_LINK_MODE_10000baseER_Full_BIT},
	/* SFP+ DAC has no standardised IEEE pmd-type identity for 10G;
	 * users can't restrict advertise to DAC-only at this rate. */
	{"25GBASE-CR",    25000, NO_BIT, ETHTOOL_LINK_MODE_25000baseCR_Full_BIT},
	{"25GBASE-SR",    25000, NO_BIT, ETHTOOL_LINK_MODE_25000baseSR_Full_BIT},
	/* Kernel collapses LR/SR onto the same 25G bit. */
	{"25GBASE-LR",    25000, NO_BIT, ETHTOOL_LINK_MODE_25000baseSR_Full_BIT},
	{"40GBASE-CR4",   40000, NO_BIT, ETHTOOL_LINK_MODE_40000baseCR4_Full_BIT},
	{"40GBASE-SR4",   40000, NO_BIT, ETHTOOL_LINK_MODE_40000baseSR4_Full_BIT},
	{"40GBASE-LR4",   40000, NO_BIT, ETHTOOL_LINK_MODE_40000baseLR4_Full_BIT},
	{"100GBASE-CR4",  100000, NO_BIT, ETHTOOL_LINK_MODE_100000baseCR4_Full_BIT},
	{"100GBASE-SR4",  100000, NO_BIT, ETHTOOL_LINK_MODE_100000baseSR4_Full_BIT},
	{"100GBASE-LR4",  100000, NO_BIT, ETHTOOL_LINK_MODE_100000baseLR4_ER4_Full_BIT},
	{NULL, 0, NO_BIT, NO_BIT}
};

static const struct pmd_link_mode *pmd_lookup(const char *identity)
{
	const char *suffix;
	const struct pmd_link_mode *m;

	if (!identity)
		return NULL;
	suffix = strchr(identity, ':');
	suffix = suffix ? suffix + 1 : identity;
	if (strncmp(suffix, "pmd-type-", 9) == 0)
		suffix += 9;

	for (m = pmd_link_modes; m->pmd; m++) {
		if (strcmp(m->pmd, suffix) == 0)
			return m;
	}
	return NULL;
}


static bool iface_uses_autoneg(struct lyd_node *cif)
{
	struct lyd_node *aneg = lydx_get_descendant(lyd_child(cif), "ethernet",
						    "auto-negotiation", NULL);

	/* `auto-negotiation` is a presence container; when absent the port
	 * auto-negotiates (the modern default).  When present, `enable`
	 * defaults to true and is materialised in the config tree, so
	 * lydx_get_bool() reads it correctly. */
	return !aneg || lydx_get_bool(aneg, "enable");
}

/*
 * XXX: always disable flow control, for now, until we've added
 *      configurable support for flow-control/pause/direction and
 *      flow-control/force-flow-control
 */
static int netdag_gen_ethtool_flow_control(struct dagger *net, struct lyd_node *cif)
{
	const char *ifname = lydx_get_cattr(cif, "name");
	enum netdag_init phase = NETDAG_INIT_PHYS;
	FILE *fp;

	/* Skip flow control configuration for NICs with broken support */
	if (iface_has_quirk(ifname, "broken-flow-control"))
		return 0;

	if (iface_has_quirk(ifname, "phy-detached-when-down"))
		phase = NETDAG_INIT_POST;

	fp = dagger_fopen_net_init(net, ifname, phase, "ethtool-flow-control.sh");
	if (!fp)
		return -EIO;

	/* Check if the NIC supports pause frames at all */
	fprintf(fp, "[ -n \"$(ethtool --json %s | jq '.[] | select(.\"supported-pause-frame-use\" == \"No\")')\" ] && exit 0\n", ifname);

	/* Disable flow control */
	fprintf(fp, "ethtool --pause %s autoneg %s rx off tx off\n",
		ifname, iface_uses_autoneg(cif) ? "on" : "off");
	fclose(fp);

	return 0;
}

/* Walk the advertised-pmd-types leaf-list + duplex constraint, OR matching
 * bits into *mask.  *unmapped is filled with the first identity suffix we
 * didn't recognise so the caller can produce a useful sysrepo error.
 */
static void pmd_list_to_mask(struct lyd_node *aneg, const char *duplex,
			     uint64_t *mask, const char **unmapped)
{
	struct lyd_node *node;

	*mask = 0;
	LYX_LIST_FOR_EACH(lyd_child(aneg), node, "advertised-pmd-types") {
		const char *id = lyd_get_value(node);
		const struct pmd_link_mode *m = pmd_lookup(id);

		if (!m) {
			if (*unmapped == NULL)
				*unmapped = id;
			continue;
		}
		if ((!duplex || strcmp(duplex, "half") == 0) && m->half_bit != NO_BIT)
			*mask |= (1ULL << m->half_bit);
		if ((!duplex || strcmp(duplex, "full") == 0) && m->full_bit != NO_BIT)
			*mask |= (1ULL << m->full_bit);
	}
}

static int netdag_gen_ethtool_autoneg(struct dagger *net, struct lyd_node *cif)
{
	struct lyd_node *eth = lydx_get_child(cif, "ethernet");
	struct lyd_node *aneg = lydx_get_child(eth, "auto-negotiation");
	const char *ifname = lydx_get_cattr(cif, "name");
	enum netdag_init phase = NETDAG_INIT_PHYS;
	const char *duplex, *mdix, *unmapped = NULL;
	const char *mdix_arg = "";
	uint64_t mask = 0;
	int err = 0;
	FILE *fp;

	if (iface_has_quirk(ifname, "broken-autoneg"))
		return SR_ERR_OK;

	if (iface_has_quirk(ifname, "phy-detached-when-down"))
		phase = NETDAG_INIT_POST;

	fp = dagger_fopen_net_init(net, ifname, phase, "ethtool-aneg.sh");
	if (!fp)
		return -EIO;

	fprintf(fp, "[ -n \"$(ethtool --json %s | jq '.[] | select(.\"supports-auto-negotiation\" == false)')\" ] && exit 0\n", ifname);

	duplex = lydx_get_cattr(eth, "duplex");

	/* MDI/MDI-X pinout.  The `mdi-x` boolean has no default, so an absent
	 * leaf (read as NULL here, hence lydx_get_cattr not lydx_get_bool) means
	 * Auto-MDIX — emit nothing, leaving the PHY alone and not poking drivers
	 * that reject the `mdix` arg.  ethtool's `mdix on` is MDI-X (crossover),
	 * `off` is MDI. */
	mdix = lydx_get_cattr(eth, "mdi-x");
	if (mdix && !strcmp(mdix, "true"))
		mdix_arg = " mdix on";
	else if (mdix && !strcmp(mdix, "false"))
		mdix_arg = " mdix off";

	/* enable=false: force a fixed speed/duplex with autoneg off, for link
	 * partners that don't run autoneg (legacy fixed-speed switches, some
	 * back-to-back direct copper links).  Standards-correct fixed-mode is
	 * advertised-pmd-types with autoneg on; this branch is the escape
	 * hatch for peers that won't even speak autoneg.  Requires exactly one
	 * advertised-pmd-types entry — that PMD picks the speed; duplex comes
	 * from the explicit leaf or defaults to the only variant available.
	 */
	if (aneg && !lydx_get_bool(aneg, "enable")) {
		const struct pmd_link_mode *forced = NULL;
		const char *fixed_duplex;
		struct lyd_node *node;
		int n = 0;

		LYX_LIST_FOR_EACH(lyd_child(aneg), node, "advertised-pmd-types") {
			const char *id = lyd_get_value(node);
			const struct pmd_link_mode *m = pmd_lookup(id);

			if (!m) {
				if (!unmapped)
					unmapped = id;
			} else if (!forced) {
				forced = m;
			}
			n++;
		}
		if (unmapped) {
			sr_session_set_error_message(net->session,
				"%s: advertised-pmd-types entry \"%s\" is not a known PMD type",
				ifname, unmapped);
			err = -EINVAL;
			goto out;
		}
		if (n != 1 || !forced) {
			sr_session_set_error_message(net->session,
				"%s: auto-negotiation/enable=false requires exactly one "
				"advertised-pmd-types entry (have %d)", ifname, n);
			err = -EINVAL;
			goto out;
		}

		if (duplex)
			fixed_duplex = duplex;
		else if (forced->full_bit != NO_BIT)
			fixed_duplex = "full";
		else
			fixed_duplex = "half";

		fprintf(fp, "ethtool --change %s autoneg off speed %d duplex %s%s\n",
			ifname, forced->speed_mbps, fixed_duplex, mdix_arg);
		goto out;
	}

	if (aneg)
		pmd_list_to_mask(aneg, duplex, &mask, &unmapped);

	if (unmapped) {
		sr_session_set_error_message(net->session,
			"%s: advertised-pmd-types entry \"%s\" is not a known PMD type",
			ifname, unmapped);
		err = -EINVAL;
		goto out;
	}

	if (mask) {
		/* Restrict autoneg to the advertised set (single entry == fixed). */
		fprintf(fp, "ethtool --change %s autoneg on advertise 0x%" PRIx64 "%s\n",
			ifname, mask, mdix_arg);
	} else {
		/* No advertise restriction configured → advertise everything the
		 * PHY supports.  Plain `autoneg on` is a no-op when autoneg is
		 * already enabled and does NOT restore a previously narrowed
		 * advertise mask, so query supported and re-enable each mode.
		 * ethtool's symbolic advertise syntax is bare NAME on|off pairs
		 * (despite what `--help` suggests, there is no `mode` keyword). */
		fprintf(fp,
			"args=$(ethtool --json %s | jq -r '.[0][\"supported-link-modes\"][]? | \"\\(.) on\"' | tr '\\n' ' ')\n"
			"if [ -n \"$args\" ]; then\n"
			"\tethtool --change %s autoneg on advertise $args%s\n"
			"else\n"
			"\tethtool --change %s autoneg on%s\n"
			"fi\n",
			ifname, ifname, mdix_arg, ifname, mdix_arg);
	}

out:
	fclose(fp);
	return err;
}

int netdag_gen_ethtool(struct dagger *net, struct lyd_node *cif, struct lyd_node *dif)
{
	struct lyd_node *eth = lydx_get_child(dif, "ethernet");
	int err;

	if (!eth)
		return 0;

	if (dagger_is_bootstrap(net) ||
	    lydx_get_descendant(lyd_child(eth), "auto-negotiation", NULL)) {
		err = netdag_gen_ethtool_flow_control(net, cif);
		if (err)
			return err;
	}

	if (dagger_is_bootstrap(net) ||
	    lydx_get_descendant(lyd_child(eth), "auto-negotiation", NULL) ||
	    lydx_get_child(eth, "duplex") ||
	    lydx_get_child(eth, "mdi-x")) {
		err = netdag_gen_ethtool_autoneg(net, cif);
		if (err)
			return err;
	}

	return 0;
}

/* The standard ethtool counters behind one frame counter, summed */
struct eth_source {
	const char *group;
	const char *name;
};

struct eth_counter {
	const char *leaf;
	struct eth_source src[6];
};

static const struct eth_counter eth_counters[] = {
	{ "in-total-frames", {
		{ "eth-mac", "FramesReceivedOK" },
		{ "eth-mac", "FrameCheckSequenceErrors" },
		{ "eth-mac", "FramesLostDueToIntMACRcvError" },
		{ "eth-mac", "AlignmentErrors" },
		{ "rmon", "etherStatsOversizePkts" },
		{ "rmon", "etherStatsJabbers" } } },
	{ "in-frames",                    { { "eth-mac", "FramesReceivedOK" } } },
	{ "in-multicast-frames",          { { "eth-mac", "MulticastFramesReceivedOK" } } },
	{ "in-broadcast-frames",          { { "eth-mac", "BroadcastFramesReceivedOK" } } },
	{ "in-error-fcs-frames",          { { "eth-mac", "FrameCheckSequenceErrors" } } },
	{ "in-error-undersize-frames",    { { "rmon",    "etherStatsUndersizePkts" } } },
	{ "in-error-oversize-frames",     { { "rmon",    "etherStatsOversizePkts" },
					    { "rmon",    "etherStatsJabbers" } } },
	{ "in-error-mac-internal-frames", { { "eth-mac", "FramesLostDueToIntMACRcvError" } } },
	{ "in-good-octets",               { { "eth-mac", "OctetsReceivedOK" } } },
	{ "out-frames",                   { { "eth-mac", "FramesTransmittedOK" } } },
	{ "out-multicast-frames",         { { "eth-mac", "MulticastFramesXmittedOK" } } },
	{ "out-broadcast-frames",         { { "eth-mac", "BroadcastFramesXmittedOK" } } },
	{ "out-good-octets",              { { "eth-mac", "OctetsTransmittedOK" } } },
};

static bool valid_ifname(const char *name)
{
	size_t len = strnlen(name, IFNAMSIZ);

	if (!len || len >= IFNAMSIZ)
		return false;
	for (; *name; name++) {
		if (!isalnum(*name) && !strchr("._-", *name))
			return false;
	}

	return true;
}

/* The get-statistics action, the IEEE 802.3 counters ethtool reads from the port */
static int ethernet_get_statistics(sr_session_ctx_t *session, uint32_t sub_id, const char *op_path,
				   const struct lyd_node *input, sr_event_t event, uint32_t request_id,
				   struct lyd_node *output, void *priv)
{
	struct lyd_node *iface, *frame;
	json_t *root, *stats;
	const char *ifname;
	json_error_t jerr;
	FILE *pp;
	size_t i;

	if (event != SR_EV_RPC)
		return SR_ERR_OK;

	iface = lyd_parent(lyd_parent(input));
	ifname = iface ? lydx_get_cattr(iface, "name") : NULL;
	if (!ifname || !valid_ifname(ifname) || !if_nametoindex(ifname))
		return rpc_failed(session, "No such interface");

	pp = popenf("re", "ethtool --json -S %s --all-groups", ifname);
	if (!pp)
		return rpc_failed(session, "Failed reading Ethernet statistics");
	root = json_loadf(pp, 0, &jerr);
	pclose(pp);

	stats = json_array_get(root, 0);
	if (!json_is_object(stats)) {
		json_decref(root);
		return rpc_failed(session, "Failed reading Ethernet statistics");
	}

	if (lyd_new_inner(output, NULL, "frame", 1, &frame)) {
		json_decref(root);
		return rpc_failed(session, "Failed building output");
	}

	for (i = 0; i < NELEMS(eth_counters); i++) {
		const struct eth_counter *c = &eth_counters[i];
		json_int_t sum = 0;
		bool found = false;
		char buf[32];
		size_t j;

		for (j = 0; j < NELEMS(c->src) && c->src[j].group; j++) {
			json_t *val = json_object_get(json_object_get(stats, c->src[j].group),
						      c->src[j].name);

			if (!json_is_integer(val))
				continue;
			sum += json_integer_value(val);
			found = true;
		}
		if (!found)
			continue;

		snprintf(buf, sizeof(buf), "%" JSON_INTEGER_FORMAT, sum);
		lyd_new_term(frame, NULL, c->leaf, buf, LYD_NEW_VAL_OUTPUT, NULL);
	}
	json_decref(root);

	if (!lyd_child(frame))
		lyd_free_tree(frame);

	return SR_ERR_OK;
}

int ethernet_rpc_init(struct confd *confd)
{
	int rc;

	REGISTER_RPC_TREE(confd->session, IF_XPATH "/ieee802-ethernet-interface:ethernet"
			  "/infix-ethernet-interface:get-statistics",
			  ethernet_get_statistics, NULL, &confd->sub);
	return SR_ERR_OK;
fail:
	ERROR("Init ethernet rpc failed: %s", sr_strerror(rc));
	return rc;
}
