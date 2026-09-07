/* SPDX-License-Identifier: BSD-3-Clause */

#include <stdbool.h>
#include <dirent.h>

#include <srx/common.h>
#include <srx/lyx.h>

#include "interfaces.h"

#define NUM_PRIO   8
#define MAX_TC     8

/*
 * IEEE 802.1Q-2022 Table 8-5, recommended priority to traffic class
 * mappings for an ordinary bridge.  Indexed by [num_tc - 1][priority].
 */
static const uint8_t tc_ieee[MAX_TC][NUM_PRIO] = {
	{ 0, 0, 0, 0, 0, 0, 0, 0 },
	{ 0, 0, 0, 0, 1, 1, 1, 1 },
	{ 0, 0, 0, 0, 1, 1, 2, 2 },
	{ 0, 0, 1, 1, 2, 2, 3, 3 },
	{ 0, 0, 1, 1, 2, 2, 3, 4 },
	{ 1, 0, 2, 2, 3, 3, 4, 5 },
	{ 1, 0, 2, 3, 4, 4, 5, 6 },
	{ 1, 0, 2, 3, 4, 5, 6, 7 },
};

/*
 * IEEE 802.1Q-2022 Table 34-1, priority to traffic class with SR
 * classes A (priority 3) and B (priority 2) on the two highest classes.
 */
static const uint8_t tc_ieee_sr[MAX_TC][NUM_PRIO] = {
	{ 0, 0, 0, 0, 0, 0, 0, 0 },
	{ 0, 0, 1, 1, 0, 0, 0, 0 },
	{ 0, 0, 1, 2, 0, 0, 0, 0 },
	{ 0, 0, 2, 3, 1, 1, 1, 1 },
	{ 0, 0, 3, 4, 1, 1, 2, 2 },
	{ 0, 0, 4, 5, 1, 1, 2, 3 },
	{ 0, 0, 5, 6, 1, 2, 3, 4 },
	{ 1, 0, 6, 7, 2, 3, 4, 5 },
};

/* RFC 4594 per-hop-behaviour groups to priority; other codepoints untrusted. */
static const uint8_t dscp_ietf[][2] = {
	{  0, 0 },
	{  8, 1 }, { 10, 1 }, { 12, 1 }, { 14, 1 },
	{ 16, 2 }, { 18, 2 }, { 20, 2 }, { 22, 2 },
	{ 24, 3 }, { 26, 3 }, { 28, 3 }, { 30, 3 },
	{ 32, 4 }, { 34, 4 }, { 36, 4 }, { 38, 4 },
	{ 40, 5 }, { 46, 5 },
	{ 48, 6 },
	{ 56, 7 },
};

enum tsa {
	TSA_STRICT,
	TSA_ETS,
	TSA_UNSUPPORTED,
};

struct qos_egress {
	int      num_tc;
	uint8_t  map[NUM_PRIO];
	enum tsa algo[MAX_TC];
	uint32_t weight[MAX_TC];
};

/*
 * A non-presence container with defaults always exists in the tree; only
 * a leaf, list entry or presence container that is not a default makes
 * it configuration.
 */
static bool qos_is_explicit(struct lyd_node *node)
{
	struct lyd_node *child;

	if (!node || !node->schema)
		return false;

	switch (node->schema->nodetype) {
	case LYS_LEAF:
	case LYS_LEAFLIST:
		return !(node->flags & LYD_DEFAULT);
	case LYS_LIST:
		return true;
	case LYS_CONTAINER:
		if (node->schema->flags & LYS_PRESENCE)
			return true;
		break;
	default:
		break;
	}

	LY_LIST_FOR(lyd_child(node), child)
		if (qos_is_explicit(child))
			return true;

	return false;
}

/*
 * Drivers with DCB rewrite support, and the trust orders their dcb
 * apptrust accepts; the kernel has no query for either.  Same table as
 * yanger's capabilities.  The rewrite table cannot be probed by trying:
 * a driver with dcbnl operations but no dcbnl_setrewr gets the kernel's
 * generic table, which accepts the entries and programs nothing.
 */
static const struct {
	const char *driver;
	const char *orders[4];
} dcb_drivers[] = {
	{ "sparx5-switch",  { "pcp", "dscp", "dscp-pcp", NULL } },
	{ "lan966x-switch", { "pcp", "dscp", "dscp-pcp", NULL } },
};

/*
 * One class per transmit queue, at most eight.  A single queue has no
 * queue structure to respect, so the kernel's eight classes apply.
 * Interfaces that do not exist yet get eight as well.
 */
static int qos_num_tc(const char *ifname)
{
	char path[PATH_MAX];
	struct dirent *d;
	int n = 0;
	DIR *dir;

	snprintf(path, sizeof(path), "/sys/class/net/%s/queues", ifname);
	dir = opendir(path);
	if (!dir)
		return MAX_TC;

	while ((d = readdir(dir)))
		if (!strncmp(d->d_name, "tx-", 3))
			n++;
	closedir(dir);

	return n > 1 ? MIN(n, MAX_TC) : MAX_TC;
}

/* Physical ports get the defaults rendered; virtual interfaces only on request. */
static bool qos_is_port(const char *ifname)
{
	char path[PATH_MAX];

	snprintf(path, sizeof(path), "/sys/class/net/%s/device", ifname);
	return access(path, F_OK) == 0;
}

static const char *qos_driver(const char *ifname, char *buf, size_t len)
{
	char path[PATH_MAX], line[128];
	const char *driver = NULL;
	FILE *fp;

	snprintf(path, sizeof(path), "/sys/class/net/%s/device/uevent", ifname);
	fp = fopen(path, "r");
	if (!fp)
		return NULL;

	while (fgets(line, sizeof(line), fp)) {
		if (strncmp(line, "DRIVER=", 7))
			continue;
		strlcpy(buf, chomp(line + 7), len);
		driver = buf;
		break;
	}
	fclose(fp);

	return driver;
}

static int qos_dcb_driver(const char *ifname)
{
	char buf[64];
	const char *driver = qos_driver(ifname, buf, sizeof(buf));
	size_t i;

	if (driver)
		for (i = 0; i < NELEMS(dcb_drivers); i++)
			if (!strcmp(dcb_drivers[i].driver, driver))
				return i;

	return -1;
}

/* Unknown drivers are not limited: without DCB the order is honoured in software. */
static bool qos_trust_supported(const char *ifname, const char *order)
{
	int i = qos_dcb_driver(ifname);
	size_t j;

	if (i < 0)
		return true;

	for (j = 0; dcb_drivers[i].orders[j]; j++)
		if (!strcmp(dcb_drivers[i].orders[j], order))
			return true;

	return false;
}

static enum tsa tsa_from_str(const char *val)
{
	const char *id = strrchr(val, ':');

	id = id ? id + 1 : val;

	if (!strcmp(id, "strict-priority"))
		return TSA_STRICT;
	if (!strcmp(id, "enhanced-transmission-selection"))
		return TSA_ETS;

	return TSA_UNSUPPORTED;
}

/* The trust leaf as dcb apptrust order words, none as an empty order. */
static const char *trust_order(struct lyd_node *ingress)
{
	const char *val = ingress ? lydx_get_cattr(ingress, "trust") : NULL;

	if (!val)
		return "pcp";
	if (!strcmp(val, "dscp-pcp"))
		return "dscp pcp";
	if (!strcmp(val, "pcp-dscp"))
		return "pcp dscp";
	if (!strcmp(val, "none"))
		return "";

	return val;
}

static int qos_parse_egress(struct lyd_node *egress, const char *ifname, struct qos_egress *eg)
{
	struct lyd_node *table, *tc;
	const char *val;
	int i;

	memset(eg, 0, sizeof(*eg));
	eg->num_tc = qos_num_tc(ifname);

	table = egress ? lydx_get_child(egress, "traffic-class-table") : NULL;

	/* Preset, or custom leaves with the ieee preset behind unset ones. */
	val = table ? lydx_get_cattr(table, "preset") : NULL;
	memcpy(eg->map, val && !strcmp(val, "ieee-sr") ? tc_ieee_sr[eg->num_tc - 1]
	       : tc_ieee[eg->num_tc - 1], sizeof(eg->map));
	for (i = 0; i < NUM_PRIO && table; i++) {
		char name[16];

		snprintf(name, sizeof(name), "priority%d", i);
		val = lydx_get_cattr(table, name);
		if (val)
			eg->map[i] = atoi(val);
	}

	for (i = 0; i < MAX_TC; i++) {
		eg->algo[i] = TSA_STRICT;
		eg->weight[i] = 1514;
	}

	if (!egress)
		return 0;

	LYX_LIST_FOR_EACH(lyd_child(egress), tc, "traffic-class") {
		int id = atoi(lydx_get_cattr(tc, "id"));

		if (id < 0 || id >= MAX_TC)
			return -EINVAL;

		val = lydx_get_cattr(tc, "algorithm");
		eg->algo[id] = val ? tsa_from_str(val) : TSA_STRICT;

		val = lydx_get_cattr(tc, "weight");
		if (val)
			eg->weight[id] = strtoul(val, NULL, 10);
	}

	return 0;
}

/*
 * Checks the YANG model cannot express: map values and class ids
 * against the port's class count, the algorithms rendered today, and
 * the layout tc ets can render (strict bands first, i.e. the highest
 * classes).
 */
static int qos_validate(sr_session_ctx_t *session, struct lyd_node *cif, struct lyd_node *qos)
{
	const char *ifname = lydx_get_cattr(cif, "name");
	struct lyd_node *egress, *tc;
	struct qos_egress eg;
	struct lyd_node *ingress = lydx_get_child(qos, "ingress");
	const char *trust;
	bool ets = false;
	int i;

	trust = ingress ? lydx_get_cattr(ingress, "trust") : NULL;
	if (trust && strcmp(trust, "none") && !qos_trust_supported(ifname, trust)) {
		sr_session_set_error_message(session, "%s: trust order %s not supported "
					     "by this port, see qos capabilities", ifname, trust);
		return -EINVAL;
	}

	egress = lydx_get_child(qos, "egress");
	if (qos_parse_egress(egress, ifname, &eg)) {
		sr_session_set_error_message(session, "%s: invalid traffic class table", ifname);
		return -EINVAL;
	}

	for (i = 0; i < NUM_PRIO; i++) {
		if (eg.map[i] >= eg.num_tc) {
			sr_session_set_error_message(session, "%s: priority%d maps to traffic "
						     "class %d, port has %d classes", ifname, i,
						     eg.map[i], eg.num_tc);
			return -EINVAL;
		}
	}

	if (!egress)
		return 0;

	LYX_LIST_FOR_EACH(lyd_child(egress), tc, "traffic-class") {
		int id = atoi(lydx_get_cattr(tc, "id"));

		if (id >= eg.num_tc) {
			sr_session_set_error_message(session, "%s: traffic class %d, port has "
						     "%d classes", ifname, id, eg.num_tc);
			return -EINVAL;
		}

		if (eg.algo[id] == TSA_UNSUPPORTED) {
			sr_session_set_error_message(session, "%s: traffic class %d: algorithm %s "
						     "not supported, use strict-priority or "
						     "enhanced-transmission-selection", ifname, id,
						     lydx_get_cattr(tc, "algorithm"));
			return -EINVAL;
		}
	}

	/* Walk from the top: strict classes, then weighted, never back. */
	for (i = eg.num_tc - 1; i >= 0; i--) {
		if (eg.algo[i] == TSA_ETS)
			ets = true;
		else if (ets) {
			sr_session_set_error_message(session, "%s: traffic class %d: strict-priority "
						     "classes must be the highest-numbered ones, "
						     "above all weighted classes", ifname, i);
			return -EINVAL;
		}
	}

	return 0;
}

enum trust {
	TRUST_PCP,
	TRUST_DSCP,
};

struct qos_ingress {
	const char *order;
	int        nfields;
	enum trust field[2];
	int        dflt;
	int8_t     pcp[NUM_PRIO][2];	/* [pcp][dei] to priority, -1 unset */
	int8_t     dscp[64];		/* to priority, -1 untrusted */
};

static void qos_parse_ingress(struct lyd_node *ingress, struct qos_ingress *in)
{
	struct lyd_node *map, *entry;
	const char *val, *preset, *word;
	int i;

	memset(in, 0, sizeof(*in));
	memset(in->pcp, -1, sizeof(in->pcp));
	memset(in->dscp, -1, sizeof(in->dscp));

	in->order = trust_order(ingress);
	for (word = in->order; *word; word = *word == ' ' ? word + 1 : word) {
		in->field[in->nfields++] = strncmp(word, "pcp", 3) ? TRUST_DSCP : TRUST_PCP;
		word += strcspn(word, " ");
	}

	if (ingress) {
		val = lydx_get_cattr(ingress, "default-priority");
		in->dflt = val ? atoi(val) : 0;
	}

	/* The one preset is the 802.1Q default decoding, PCP n to priority n. */
	map = ingress ? lydx_get_child(ingress, "pcp-map") : NULL;
	if (!map || !lydx_get_child(map, "entry")) {
		for (i = 0; i < NUM_PRIO; i++)
			in->pcp[i][0] = in->pcp[i][1] = i;
	} else {
		LYX_LIST_FOR_EACH(lyd_child(map), entry, "entry") {
			int pcp = atoi(lydx_get_cattr(entry, "pcp"));
			int dei = lydx_get_bool(entry, "dei") ? 1 : 0;

			in->pcp[pcp][dei] = atoi(lydx_get_cattr(entry, "priority"));
		}
	}

	map = ingress ? lydx_get_child(ingress, "dscp-map") : NULL;
	if (!map || !lydx_get_child(map, "entry")) {
		preset = map ? lydx_get_cattr(map, "preset") : NULL;
		if (preset && !strcmp(preset, "msb")) {
			for (i = 0; i < 64; i++)
				in->dscp[i] = i >> 3;
		} else {
			for (i = 0; i < (int)NELEMS(dscp_ietf); i++)
				in->dscp[dscp_ietf[i][0]] = dscp_ietf[i][1];
		}
	} else {
		LYX_LIST_FOR_EACH(lyd_child(map), entry, "entry") {
			if (!lydx_get_bool(entry, "trusted"))
				continue;

			in->dscp[atoi(lydx_get_cattr(entry, "dscp"))] =
				atoi(lydx_get_cattr(entry, "priority"));
		}
	}
}

/*
 * One DCB APP table per port, the trust order as dcb apptrust.  Drivers
 * without the operations fail the calls; app_err then selects the
 * software rendering below.
 */
static void gen_ingress_dcb(FILE *fp, const char *ifname, const struct qos_ingress *in)
{
	int i, dei, n;

	fputs("trust_err=0 app_err=0 rewr_err=0\n", fp);
	fprintf(fp, "dcb apptrust set dev %s order %s 2>/dev/null || trust_err=1\n", ifname, in->order);
	fprintf(fp, "dcb app flush dev %s default-prio pcp-prio dscp-prio 2>/dev/null || app_err=1\n",
		ifname);
	fprintf(fp, "dcb app add dev %s", ifname);

	for (i = 0, n = 0; i < NUM_PRIO; i++)
		for (dei = 0; dei < 2; dei++)
			if (in->pcp[i][dei] >= 0)
				fprintf(fp, "%s %d%s:%d", n++ ? "" : " pcp-prio", i,
					dei ? "de" : "nd", in->pcp[i][dei]);

	for (i = 0, n = 0; i < 64; i++)
		if (in->dscp[i] >= 0)
			fprintf(fp, "%s %d:%d", n++ ? "" : " dscp-prio", i, in->dscp[i]);

	/* default-prio takes every following word as a priority, so it goes last */
	fprintf(fp, " default-prio %d 2>/dev/null || app_err=1\n", in->dflt);
}

/*
 * Software classification: tc flower on a clsact ingress, one block of
 * rules per trusted field in trust order, then a catch-all for the
 * default priority.  First match wins.  flower cannot match DEI, so the
 * DEI 0 entry is used for both.  Tagged IP needs its own rules since the
 * DSCP then sits behind the VLAN header, and a pref holds one protocol,
 * so each variant gets its own.  A hundred rules per port is normal, so
 * they go through one tc batch rather than one process each.
 */
static void gen_ingress_flower(FILE *fp, const char *ifname, const struct qos_ingress *in)
{
	static const char *ipproto[] = {
		"protocol ip flower",
		"protocol ipv6 flower",
		"protocol 802.1Q flower vlan_ethtype ip",
		"protocol 802.1Q flower vlan_ethtype ipv6",
	};
	int f, i, p, pref = 100;

	fputs("tc -force -batch - <<EOF\n", fp);
	fprintf(fp, "qdisc add dev %s clsact\n", ifname);

	for (f = 0; f < in->nfields; f++, pref += 100) {
		if (in->field[f] == TRUST_PCP) {
			for (i = 0; i < NUM_PRIO; i++) {
				int prio = in->pcp[i][0] >= 0 ? in->pcp[i][0] : in->pcp[i][1];

				if (prio < 0)
					continue;
				fprintf(fp, "filter add dev %s ingress pref %d protocol 802.1Q "
					"flower vlan_prio %d action skbedit priority %d\n",
					ifname, pref, i, prio);
			}
			continue;
		}

		for (i = 0; i < 64; i++) {
			if (in->dscp[i] < 0)
				continue;
			for (p = 0; p < (int)NELEMS(ipproto); p++)
				fprintf(fp, "filter add dev %s ingress pref %d %s ip_tos 0x%02x/0xfc "
					"action skbedit priority %d\n", ifname, pref + p, ipproto[p],
					i << 2, in->dscp[i]);
		}
	}

	fprintf(fp, "filter add dev %s ingress pref 900 matchall action skbedit priority %d\n",
		ifname, in->dflt);
	fputs("EOF\n", fp);
}

static void gen_ingress(FILE *fp, const char *ifname, struct lyd_node *ingress)
{
	struct qos_ingress in;

	qos_parse_ingress(ingress, &in);
	gen_ingress_dcb(fp, ifname, &in);

	fputs("if [ $app_err -ne 0 ]; then\n", fp);
	gen_ingress_flower(fp, ifname, &in);
	fputs("fi\n", fp);
}

/*
 * Software DSCP remarking: tc basic filters on the egress side matching
 * the skb priority and the frame's ethertype, rewriting the DS field
 * with pedit and fixing the IPv4 header checksum.  Tagged frames carry
 * the ethertype behind the tag when the NIC has no VLAN offload, hence
 * the second pair of rules.  PCP has no software counterpart: act_vlan
 * cannot change the priority without also setting the VLAN ID.
 */
static void gen_remark_pedit(FILE *fp, const char *ifname)
{
	static const struct {
		const char *match;
		const char *munge;
		const char *csum;
	} variants[] = {
		{ "cmp(u16 at 12 layer link eq 0x0800)", "ip dsfield", " pipe action csum ip" },
		{ "cmp(u16 at 12 layer link eq 0x86dd)", "ip6 traffic_class", "" },
		{ "cmp(u16 at 12 layer link eq 0x8100) and cmp(u16 at 16 layer link eq 0x0800)",
		  "ip dsfield", " pipe action csum ip" },
		{ "cmp(u16 at 12 layer link eq 0x8100) and cmp(u16 at 16 layer link eq 0x86dd)",
		  "ip6 traffic_class", "" },
	};
	int i, v;

	fputs("tc -force -batch - <<EOF\n", fp);
	fprintf(fp, "qdisc add dev %s clsact\n", ifname);
	for (i = 0; i < NUM_PRIO; i++)
		for (v = 0; v < (int)NELEMS(variants); v++)
			fprintf(fp, "filter add dev %s egress pref %d protocol all basic match %s "
				"and meta(priority eq %d) action pedit ex munge %s set 0x%02x retain 0xfc%s\n",
				ifname, 100 + v, variants[v].match, i, variants[v].munge, i << 5,
				variants[v].csum);
	fputs("EOF\n", fp);
}

/*
 * Same DCB table as ingress, in the other direction.  prio-pcp takes
 * one entry per DEI value; both map priority to PCP 1:1 since nothing
 * assigns a drop precedence yet.  prio-dscp uses the class selector
 * codepoints, CS0 to CS7.
 */
static void gen_remark(FILE *fp, const char *ifname, struct lyd_node *remark)
{
	const char *pcp, *dscp;
	int i;

	pcp  = remark ? lydx_get_cattr(remark, "pcp") : NULL;
	dscp = remark ? lydx_get_cattr(remark, "dscp") : NULL;

	if (qos_dcb_driver(ifname) < 0) {
		fputs("rewr_err=1\n", fp);
	} else {
		fprintf(fp, "dcb rewr flush dev %s prio-pcp prio-dscp 2>/dev/null || rewr_err=1\n", ifname);
		/* One code point per priority, a DEI 1 entry would replace the DEI 0 one */
		if (pcp && !strcmp(pcp, "from-priority")) {
			fprintf(fp, "dcb rewr add dev %s prio-pcp", ifname);
			for (i = 0; i < NUM_PRIO; i++)
				fprintf(fp, " %d:%dnd", i, i);
			fputs(" 2>/dev/null || rewr_err=1\n", fp);
		}
		if (dscp && !strcmp(dscp, "from-priority")) {
			fprintf(fp, "dcb rewr add dev %s prio-dscp", ifname);
			for (i = 0; i < NUM_PRIO; i++)
				fprintf(fp, " %d:%d", i, i << 3);
			fputs(" 2>/dev/null || rewr_err=1\n", fp);
		}
	}

	if (dscp && !strcmp(dscp, "from-priority")) {
		fputs("if [ $rewr_err -ne 0 ]; then\n", fp);
		gen_remark_pedit(fp, ifname);
		fputs("fi\n", fp);
	}
}

/*
 * Only settings the user configured are reported; the defaults are
 * rendered on every port with a qos container and would flood the log
 * on hardware without DCB.  Software classification honours the trust
 * order itself, so trust_err only matters when the DCB table was taken.
 */
static void gen_dcb_log(FILE *fp, const char *ifname, struct lyd_node *ingress,
			struct lyd_node *remark)
{
	if (qos_is_explicit(ingress)) {
		fprintf(fp, "[ $app_err -eq 0 ] || logger -t confd -p user.notice "
			"\"%s: no DCB support in driver, classifying in software\"\n", ifname);
		fprintf(fp, "[ $trust_err -eq 0 ] || [ $app_err -ne 0 ] || logger -t confd -p user.notice "
			"\"%s: dcb apptrust unsupported by driver, trust order not applied\"\n",
			ifname);
	}

	if (qos_is_explicit(remark)) {
		const char *pcp = lydx_get_cattr(remark, "pcp");

		fprintf(fp, "[ $rewr_err -eq 0 ] || logger -t confd -p user.notice "
			"\"%s: no DCB rewrite support in driver, remarking DSCP in software\"\n",
			ifname);
		if (pcp && !strcmp(pcp, "from-priority"))
			fprintf(fp, "[ $rewr_err -eq 0 ] || logger -t confd -p user.notice "
				"\"%s: PCP remarking needs driver support, not applied\"\n", ifname);
	}
}

/*
 * tc mqprio takes the 802.1Q map as-is and offloads the class layout to
 * the driver.  tc ets is the software rendering: band 0 is dequeued
 * first, so class N-1 is band 0, strict bands come first, and quanta are
 * listed for the weighted bands in band order.
 */
static void gen_egress(FILE *fp, const char *ifname, struct qos_egress *eg)
{
	int i, nstrict = 0;

	fprintf(fp, "tc qdisc del dev %s root 2>/dev/null\n", ifname);
	if (eg->num_tc < 2)
		return;

	if (!iface_has_quirk(ifname, "broken-mqprio")) {
		fprintf(fp, "tc qdisc add dev %s root mqprio num_tc %d map", ifname, eg->num_tc);
		for (i = 0; i < NUM_PRIO; i++)
			fprintf(fp, " %d", eg->map[i]);
		fputs(" queues", fp);
		for (i = 0; i < eg->num_tc; i++)
			fprintf(fp, " 1@%d", i);
		fputs(" hw 1 2>/dev/null ||\n", fp);
	}

	for (i = eg->num_tc - 1; i >= 0 && eg->algo[i] == TSA_STRICT; i--)
		nstrict++;

	fprintf(fp, "tc qdisc add dev %s root ets bands %d strict %d", ifname, eg->num_tc, nstrict);
	if (nstrict < eg->num_tc) {
		fputs(" quanta", fp);
		for (i = eg->num_tc - 1 - nstrict; i >= 0; i--)
			fprintf(fp, " %u", eg->weight[i]);
	}
	fputs(" priomap", fp);
	for (i = 0; i < NUM_PRIO; i++)
		fprintf(fp, " %d", eg->num_tc - 1 - eg->map[i]);
	fputc('\n', fp);
}

static int gen_reset(struct dagger *net, const char *ifname)
{
	FILE *fp;

	fp = dagger_fopen_net_init(net, ifname, NETDAG_INIT_POST, "qos.sh");
	if (!fp)
		return -EIO;

	fprintf(fp, "tc qdisc del dev %s root 2>/dev/null\n", ifname);
	fprintf(fp, "tc qdisc del dev %s clsact 2>/dev/null\n", ifname);
	fprintf(fp, "dcb app flush dev %s default-prio pcp-prio dscp-prio 2>/dev/null\n", ifname);
	fprintf(fp, "dcb rewr flush dev %s prio-pcp prio-dscp 2>/dev/null\n", ifname);
	fprintf(fp, "dcb apptrust set dev %s order 2>/dev/null\n", ifname);
	fputs("exit 0\n", fp);
	fclose(fp);

	return 0;
}

/*
 * Whether a diff subtree carries a change to configuration.  A leaf
 * going back to its default shows up flagged default with the old value
 * in metadata, which lydx_get_diff() reads; a deleted list entry or
 * presence container always counts.
 */
static bool qos_has_change(struct lyd_node *node)
{
	struct lyd_node *child;
	struct lydx_diff nd;

	if (!node || !node->schema)
		return false;

	switch (node->schema->nodetype) {
	case LYS_LEAF:
	case LYS_LEAFLIST:
		return lydx_get_diff(node, &nd);
	case LYS_LIST:
		return true;
	case LYS_CONTAINER:
		if (node->schema->flags & LYS_PRESENCE)
			return true;
		break;
	default:
		break;
	}

	LY_LIST_FOR(lyd_child(node), child)
		if (qos_has_change(child))
			return true;

	return false;
}

int netdag_gen_qos(sr_session_ctx_t *session, struct dagger *net, struct lyd_node *cif,
		   struct lyd_node *dif)
{
	const char *ifname = lydx_get_cattr(cif, "name");
	struct lyd_node *qos, *dqos, *ingress, *remark;
	struct qos_egress eg;
	FILE *fp;
	int err;

	qos  = lydx_get_child(cif, "qos");
	dqos = lydx_get_child(dif, "qos");

	/*
	 * The defaults are a complete pipeline and render on every
	 * physical port, at creation and whenever qos changes.  Virtual
	 * interfaces get one only when configured; removing it there
	 * resets the interface.
	 */
	if (!qos_has_change(dqos) && lydx_get_op(dif) != LYDX_OP_CREATE)
		return 0;

	if (!qos_is_explicit(qos) && !qos_is_port(ifname)) {
		if (qos_has_change(dqos))
			return gen_reset(net, ifname);
		return 0;
	}

	err = qos_validate(session, cif, qos);
	if (err)
		return err;

	err = qos_parse_egress(lydx_get_child(qos, "egress"), ifname, &eg);
	if (err)
		return err;

	fp = dagger_fopen_net_init(net, ifname, NETDAG_INIT_POST, "qos.sh");
	if (!fp)
		return -EIO;

	ingress = lydx_get_child(qos, "ingress");
	remark  = lydx_get_descendant(lyd_child(qos), "egress", "remark", NULL);

	fprintf(fp, "tc qdisc del dev %s clsact 2>/dev/null\n", ifname);
	gen_ingress(fp, ifname, ingress);
	gen_remark(fp, ifname, remark);
	gen_dcb_log(fp, ifname, ingress, remark);

	/*
	 * Replacing the root qdisc reprograms the port's queues, which on
	 * offloading hardware drops traffic for a moment.  Only do it when
	 * the traffic classes changed, not for an ingress or remark edit.
	 */
	if (lydx_get_op(dif) == LYDX_OP_CREATE ||
	    qos_has_change(lydx_get_descendant(lyd_child(dqos), "egress", "traffic-class-table", NULL)) ||
	    qos_has_change(lydx_get_descendant(lyd_child(dqos), "egress", "traffic-class", NULL)))
		gen_egress(fp, ifname, &eg);
	fclose(fp);

	return 0;
}
