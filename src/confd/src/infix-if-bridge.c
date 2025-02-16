/* SPDX-License-Identifier: BSD-3-Clause */

#include <fnmatch.h>
#include <stdbool.h>
#include <jansson.h>
#include <arpa/inet.h>
#include <net/if.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "ietf-interfaces.h"

struct ixif_br {
	const char *name;
	struct lyd_node *cif;
	struct lyd_node *dif;

	FILE *ip;
	struct snippet bropts;

	struct {
		struct snippet vlan;
		struct snippet mcast;
		struct snippet mdb;
	} init;
	struct {
		struct snippet mdb;
		struct snippet mcast;
		struct snippet vlan;
	} exit;
};


/* MDB */

static int gen_mdb_filter(struct ixif_br *br, struct lyd_node *filter,
			  const char *vidstr)
{
	const char *group, *iface;
	struct lyd_node *port;
	enum lydx_op gop, pop;

	group = lydx_get_cattr(filter, "group");
	gop = lydx_get_op(filter);

	LYX_LIST_FOR_EACH(lyd_child(filter), port, "ports") {
		iface = lydx_get_cattr(port, "port");
		pop = lydx_get_op(port);

		switch (gop) {
		case LYDX_OP_NONE:
		case LYDX_OP_CREATE:
		case LYDX_OP_REPLACE:
			if (pop == LYDX_OP_DELETE)
				/* The group was modified, but this
				 * particular port was removed.
				 */
				goto delete;

			fprintf(br->init.mdb.fp, "mdb replace dev %s port %s grp %s %s %s",
				br->name, iface, group, vidstr ? : "",
				lydx_get_cattr(port, "state"));
			break;
		case LYDX_OP_DELETE:
		delete:
			fprintf(br->exit.mdb.fp, "mdb del dev %s port %s grp %s %s",
				br->name, iface, group, vidstr ? : "");
			break;
		}
	}

	return 0;
}

static int gen_mdb(struct ixif_br *br, struct lyd_node *ctx)
{
	struct lyd_node *filters, *filter;
	char *vidstr = NULL;
	const char *vid;
	int err = 0;

	filters = lydx_get_child(ctx, "multicast-filters");
	if (!filters)
		return 0;

	vid = lydx_get_cattr(ctx, "vid");
	if (vid)
		asprintf(&vidstr, "vid %s", vid);

	LYX_LIST_FOR_EACH(lyd_child(filters), filter, "multicast-filter") {
		err = gen_mdb_filter(br, filter, vidstr);
		if (err)
			break;
	}

	free(vidstr);

	return err;
}

/* MDB */

/* MCAST */

static bool has_vlan_mcast_snooping(struct ixif_br *br)
{
	struct lyd_node *vlans, *vlan, *mcast;

	vlans = lydx_get_descendant(lyd_child(br->cif), "bridge", "vlans", NULL);
	if (!vlans)
		return false;

	LYX_LIST_FOR_EACH(lyd_child(vlans), vlan, "vlan") {
		mcast = lydx_get_descendant(lyd_child(vlan), "multicast", NULL);

		if (mcast && lydx_is_enabled(mcast, "snooping"))
			return true;
	}

	return false;
}

static int gen_vlan_mcast(struct ixif_br *br, struct lyd_node *vlan)
{
	struct lyd_node *mcast;
	bool snooping;
	int vid;

	mcast = lydx_get_descendant(lyd_child(vlan), "multicast", NULL);
	if (!mcast)
		return 0;

	vid = atoi(lydx_get_cattr(vlan, "vid"));
	snooping = lydx_is_enabled(mcast, "snooping");

	fprintf(br->init.mcast.fp, "vlan global set vid %d dev %s mcast_snooping %d",
		vid, br->name, snooping ? 1 : 0);
	fprintf(br->init.mcast.fp, " mcast_igmp_version 3 mcast_mld_version 2\n");

	return 0;
}

static int gen_vlans_mcast(struct ixif_br *br)
{
	struct lyd_node *vlans, *vlan;
	int err;

	vlans = lydx_get_descendant(lyd_child(br->cif), "bridge", "vlans", NULL);
	if (!vlans)
		return 0;

	LYX_LIST_FOR_EACH(lyd_child(vlans), vlan, "vlan") {
		err = gen_vlan_mcast(br, vlan);
		if (err)
			return err;
	}

	return 0;
}

static int gen_ieee_forward(struct ixif_br *br)
{
	struct lyd_node *node, *proto;
	int fwd_mask = 0;

	node = lydx_get_descendant(lyd_child(br->cif), "bridge", NULL);
	if (!node)
		return 0;

	LYX_LIST_FOR_EACH(lyd_child(node), proto, "ieee-group-forward") {
		struct lyd_node_term  *leaf  = (struct lyd_node_term *)proto;
		struct lysc_node_leaf *sleaf = (struct lysc_node_leaf *)leaf->schema;

		if ((sleaf->nodetype & (LYS_LEAF | LYS_LEAFLIST)) && (sleaf->type->basetype == LY_TYPE_UNION)) {
			struct lyd_value *actual = &leaf->value.subvalue->value;
			int val;

			if (actual->realtype->basetype == LY_TYPE_UINT8)
				val = actual->uint8;
			else
				val = actual->enum_item->value;

			fwd_mask |= 1 << val;
		}
	}

	fprintf(br->bropts.fp, " group_fwd_mask %d", fwd_mask);
	return 0;
}

static int gen_mcast(struct ixif_br *br)
{
	bool vlan_snooping = has_vlan_mcast_snooping(br);
	struct lyd_node *mcast;
	bool snooping = false;
	int err, interval = 0;

	err = gen_ieee_forward(br);
	if (err)
		return err;

	mcast = lydx_get_descendant(lyd_child(br->cif), "bridge", "multicast", NULL);
	if (mcast) {
		snooping = lydx_is_enabled(mcast, "snooping");
		interval = atoi(lydx_get_cattr(mcast, "query-interval"));
	}

	fprintf(br->bropts.fp, " mcast_snooping %d mcast_querier 0",
		(snooping || vlan_snooping) ? 1 : 0);
	fprintf(br->bropts.fp, " mcast_vlan_snooping %d", vlan_snooping ? 1 : 0);

	if (snooping)
		fprintf(br->bropts.fp, " mcast_igmp_version 3 mcast_mld_version 2");

	if (interval)
		fprintf(br->bropts.fp, " mcast_query_interval %d", interval * 100);

	if (vlan_snooping)
		err = gen_vlans_mcast(br);

	return err;
}

/* MCAST */

/* VLAN */

static int gen_vlan_membership(struct ixif_br *br, struct lyd_node *vlan, const char *mode)
{
	struct lyd_node *portentry;
	enum lydx_op pop, vop;
	const char *iface;
	int pvid, vid;

	vid = atoi(lydx_get_cattr(vlan, "vid"));
	vop = lydx_get_op(vlan);

	LYX_LIST_FOR_EACH(lyd_child(vlan), portentry, mode) {
		iface = lyd_get_value(portentry);
		pop = lydx_get_op(portentry);

		switch (vop) {
		case LYDX_OP_NONE:
		case LYDX_OP_CREATE:
		case LYDX_OP_REPLACE:
			if (pop == LYDX_OP_DELETE)
				/* This VLAN was modified, but this
				 * particular port was removed.
				 */
				goto delete;

			pvid = 0;
			srx_get_int(confd.netdag.session, &pvid, SR_UINT16_T,
				    IF_XPATH "[name='%s']/bridge-port/pvid", iface);

			fprintf(br->init.vlan.fp, "vlan add vid %d dev %s %s %s %s\n",
				vid, iface, vid == pvid ? "pvid" : "", mode,
				!strcmp(br->name, iface) ? "self" : "");
			break;
		case LYDX_OP_DELETE:
		delete:
			fprintf(br->exit.vlan.fp, "vlan del vid %d dev %s %s\n",
				vid, iface, !strcmp(br->name, iface) ? "self" : "");
			break;
		}
	}

	return 0;
}

static int gen_vlan(struct ixif_br *br)
{
	static const char *modes[] = { "tagged", "untagged", NULL };
	struct lyd_node *vlans, *vlan;
	const char **mode;
	int err;

	vlans = lydx_get_descendant(lyd_child(br->cif), "bridge", "vlans", NULL);
	if (!vlans) {
		fputs(" vlan_filtering 0", br->bropts.fp);
		return 0;
	}

	fputs(" vlan_filtering 1", br->bropts.fp);
	fprintf(br->bropts.fp, " vlan_protocol %s",
		bridge_tagtype2str(lydx_get_cattr(vlans, "proto")));

	vlans = lydx_get_descendant(lyd_child(br->dif), "bridge", "vlans", NULL);
	if (!vlans)
		return 0;

	LYX_LIST_FOR_EACH(lyd_child(vlans), vlan, "vlan") {
		for (mode = modes; *mode; mode++) {
			err = gen_vlan_membership(br, vlan, *mode);
			if (err)
				return err;
		}

		err = gen_mdb(br, vlan);
		if (err)
			return err;
	}

	return 0;
}

/* VLAN */

/* STP */

int bridge_mstpd_gen(struct lyd_node *cifs)
{
	struct ly_set *stp_brs;
	int n, err = 0;
	FILE *cond;

	if (lyd_find_xpath(cifs, "../interface/bridge/stp", &stp_brs))
		return -EINVAL;

	n = stp_brs->count;
	ly_set_free(stp_brs, NULL);

	if (n) {
		cond = dagger_fopen_next(&confd.netdag, "init", "@pre", 50, "enable-mstpd.sh");
		if (!cond)
			return -EIO;

		fputs("initctl -bnq start mstpd\n"
		      "/usr/libexec/confd/mstpd-wait-online || exit 1\n", cond);
		fclose(cond);
	} else if (!dagger_is_bootstrap(&confd.netdag)) {
		cond = dagger_fopen_current(&confd.netdag, "exit", "@post", 50, "disable-mstpd.sh");
		if (!cond)
			return -EIO;

		fputs("initctl -bnq stop mstpd\n", cond);
		fclose(cond);
	}

	return err;
}

static int gen_stp(struct ixif_br *br)
{
	struct lyd_node *stp;
	FILE *mstpctl;

	stp = lydx_get_descendant(lyd_child(br->cif), "bridge", "stp", NULL);
	if (!stp) {
		fputs(" stp_state 0", br->bropts.fp);
		return 0;
	}

	fputs(" stp_state 1", br->bropts.fp);

	if (lydx_get_descendant(lyd_child(br->cif), "bridge", "vlans", NULL))
		/* Use the MSTP compatible mode of managing port
		 * states, rather then the legacy (and default)
		 * per-VLAN mode. */
		fputs(" mst_enabled 1", br->bropts.fp);

	mstpctl = dagger_fopen_net_init(&confd.netdag, br->name,
					NETDAG_INIT_DAEMON, "init-br.mstpctl");
	if (!mstpctl)
		return -EIO;

	fputs("#!/sbin/mstpctl -b\n", mstpctl);

	fprintf(mstpctl, "setforcevers %s %s\n", br->name,
		lydx_get_cattr(stp, "force-protocol"));

	fprintf(mstpctl, "sethello %s %s\n", br->name,
		lydx_get_cattr(stp, "hello-time"));

	fprintf(mstpctl, "setfdelay %s %s\n", br->name,
		lydx_get_cattr(stp, "forward-delay"));

	fprintf(mstpctl, "setmaxage %s %s\n", br->name,
		lydx_get_cattr(stp, "max-age"));

	fprintf(mstpctl, "settxholdcount %s %s\n", br->name,
		lydx_get_cattr(stp, "transmit-hold-count"));

	fprintf(mstpctl, "setmaxhops %s %s\n", br->name,
		lydx_get_cattr(stp, "max-hops"));

	fprintf(mstpctl, "settreeprio %s 0 %s\n", br->name,
		lydx_get_cattr(lydx_get_child(stp, "cist"), "priority"));

	fclose(mstpctl);
	return 0;

}

/* STP */

/* BR */

static void gen_phys_address(struct ixif_br *br)
{
	const char *chassis;
	int err;

	err = link_gen_address(br->cif, br->ip);
	if (!err)
		return;

	chassis = get_chassis_addr();
	if (!chassis)
		return;

	fprintf(br->ip, " address %s", chassis);
}

static int init_snippets(struct ixif_br *br, struct lyd_node *dif, struct lyd_node *cif,
			 FILE *ip)
{
	int err = 0;

	memset(br, 0, sizeof(*br));

	br->name = lydx_get_cattr(cif, "name");
	br->dif = dif;
	br->cif = cif;

	br->ip = ip;

	err = snippet_open(&br->bropts);
	err = err ? : snippet_open(&br->init.vlan);
	err = err ? : snippet_open(&br->init.mcast);
	err = err ? : snippet_open(&br->init.mdb);
	err = err ? : snippet_open(&br->exit.mdb);
	err = err ? : snippet_open(&br->exit.mcast);
	err = err ? : snippet_open(&br->exit.vlan);
	if (err) {
		snippet_close(&br->exit.vlan, NULL);
		snippet_close(&br->exit.mcast, NULL);
		snippet_close(&br->exit.mdb, NULL);
		snippet_close(&br->init.mdb, NULL);
		snippet_close(&br->init.mcast, NULL);
		snippet_close(&br->init.vlan, NULL);
		snippet_close(&br->bropts, NULL);
		return err;
	}

	return 0;
}

static int collect_snippets(struct ixif_br *br)
{
	FILE *init, *exit = NULL;
	int err;

	err = snippet_close(&br->bropts, br->ip);
	fputc('\n', br->ip);

	init = dagger_fopen_net_init(&confd.netdag, br->name,
				 NETDAG_INIT_PROTO, "init.bridge");

	if (!dagger_is_bootstrap(&confd.netdag))
		exit = dagger_fopen_net_exit(&confd.netdag, br->name,
					    NETDAG_EXIT_PROTO, "exit.bridge");

	err = err ? : snippet_close(&br->init.vlan, init);
	err = err ? : snippet_close(&br->init.mcast, init);
	err = err ? : snippet_close(&br->init.mdb, init);

	err = err ? : snippet_close(&br->exit.mdb, exit);
	err = err ? : snippet_close(&br->exit.mcast, exit);
	err = err ? : snippet_close(&br->exit.vlan, exit);

	if (exit)
		fclose(exit);
	else if (!dagger_is_bootstrap(&confd.netdag))
		err = err ? : -EIO;

	if (init)
		fclose(init);
	else
		err = err ? : -EIO;

	return err;
}

int bridge_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip, int add)
{
	const char *op = add ? "add" : "set";
	struct ixif_br br;
	int err;

	err = init_snippets(&br, dif, cif, ip);
	if (err)
		return err;

	fputs(" mcast_flood_always 1", br.bropts.fp);
	fputs(" vlan_default_pvid 0", br.bropts.fp);

	err = gen_stp(&br);
	if (err)
		goto out;

	err = gen_vlan(&br);
	if (err)
		goto out;

	err = gen_mcast(&br);
	if (err)
		goto out;

	err = gen_mdb(&br, lydx_get_child(dif, "bridge"));
	if (err)
		goto out;

	fprintf(br.ip, "link %s dev %s", op, br.name);
	if (add)
		gen_phys_address(&br);

	fprintf(br.ip, " type bridge");

out:
	err = collect_snippets(&br);
	return err;
}

int bridge_add_deps(struct lyd_node *cif)
{
	const char *brname = lydx_get_cattr(cif, "name");
	struct ly_set *brports;
	const char *portname;
	int err = 0;
	uint32_t i;

	brports = lydx_find_xpathf(cif, "../interface[bridge-port/bridge='%s']", brname);
	if (!brports)
		return ERR_IFACE(cif, -ENOENT, "Unable to fetch bridge ports");


	for (i = 0; i < brports->count; i++) {
		portname = lydx_get_cattr(brports->dnodes[i], "name");

		err = dagger_add_dep(&confd.netdag, brname, portname);
		if (err) {
			ERR_IFACE(cif, err, "Unable to depend on \"%s\"", portname);
			break;
		}
	}

	ly_set_free(brports, NULL);
	return err;
}

/* BR */
