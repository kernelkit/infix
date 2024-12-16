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

static bool ixif_br_vlan_has_mcast_snooping(struct ixif_br *br);


/* MDB */

static int ixif_br_mdb_gen_filter(struct ixif_br *br, struct lyd_node *filter,
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

static int ixif_br_mdb_gen(struct ixif_br *br, struct lyd_node *ctx)
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
		err = ixif_br_mdb_gen_filter(br, filter, vidstr);
		if (err)
			break;
	}

	free(vidstr);

	return err;
}

/* MDB */

/* MCAST */

static int ixif_br_mcast_gen_vlan(struct ixif_br *br, struct lyd_node *vlan)
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

static int ixif_br_mcast_gen_vlans(struct ixif_br *br)
{
	struct lyd_node *vlans, *vlan;
	int err;

	vlans = lydx_get_descendant(lyd_child(br->cif), "bridge", "vlans", NULL);
	if (!vlans)
		return 0;

	LYX_LIST_FOR_EACH(lyd_child(vlans), vlan, "vlan") {
		err = ixif_br_mcast_gen_vlan(br, vlan);
		if (err)
			return err;
	}

	return 0;
}

static int ixif_br_mcast_gen_ieee_forward(struct ixif_br *br)
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

static int ixif_br_mcast_gen(struct ixif_br *br)
{
	bool vlan_snooping = ixif_br_vlan_has_mcast_snooping(br);
	struct lyd_node *mcast;
	bool snooping = false;
	int err, interval = 0;

	err = ixif_br_mcast_gen_ieee_forward(br);
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
		err = ixif_br_mcast_gen_vlans(br);

	return err;
}

/* MCAST */

/* VLAN */

static bool ixif_br_vlan_has_mcast_snooping(struct ixif_br *br)
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

static int ixif_br_vlan_gen_membership(struct ixif_br *br,
				       struct lyd_node *vlan, const char *mode)
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

static int ixif_br_vlan_gen(struct ixif_br *br)
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
			err = ixif_br_vlan_gen_membership(br, vlan, *mode);
			if (err)
				return err;
		}

		err = ixif_br_mdb_gen(br, vlan);
		if (err)
			return err;
	}

	return 0;
}

/* VLAN */

/* BR */

static void ixif_br_gen_phys_address(struct ixif_br *br)
{
	struct json_t *j;
	const char *mac;

	mac = get_phys_addr(br->cif, NULL);
	if (!mac) {
		j = json_object_get(confd.root, "mac-address");
		if (j)
			mac = json_string_value(j);
	}

	if (!mac)
		/* Nothing configured, and no chassis mac available,
		 * let the kernel generate a random one.
		 */
		return;

	fprintf(br->ip, " address %s", mac);
}

static int ixif_br_init(struct ixif_br *br, struct lyd_node *dif, struct lyd_node *cif,
			FILE *ip)
{
	int err = 0;

	memset(br, 0, sizeof(*br));

	br->name = lydx_get_cattr(cif, "name");
	br->dif = dif;
	br->cif = cif;

	br->ip = ip;
	err = snippet_open(&br->bropts);
	if (err)
		goto err;

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
		goto err_close_bropts;
	}

	return 0;

err_close_bropts:
	snippet_close(&br->bropts, NULL);
err:
	return err;
}

static int ixif_br_fini(struct ixif_br *br)
{
	FILE *init, *exit = NULL;
	int err;

	err = snippet_close(&br->bropts, br->ip);
	fputc('\n', br->ip);

	init = dagger_fopen_next(&confd.netdag, "init", br->name,
				 60, "init.bridge");

	if (!dagger_is_bootstrap(&confd.netdag))
		exit = dagger_fopen_current(&confd.netdag, "exit", br->name,
					    60, "exit.bridge");

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

int ixif_br_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip, int add)
{
	const char *op = add ? "add" : "set";
	struct ixif_br br;
	int err;

	err = ixif_br_init(&br, dif, cif, ip);
	if (err)
		return err;

	fputs(" mcast_flood_always 1", br.bropts.fp);
	fputs(" vlan_default_pvid 0", br.bropts.fp);

	err = ixif_br_vlan_gen(&br);
	if (err)
		goto out;

	err = ixif_br_mcast_gen(&br);
	if (err)
		goto out;

	err = ixif_br_mdb_gen(&br, lydx_get_child(dif, "bridge"));
	if (err)
		goto out;

	fprintf(br.ip, "link %s dev %s", op, br.name);
	if (add)
		ixif_br_gen_phys_address(&br);

	fprintf(br.ip, " type bridge");

out:
	ixif_br_fini(&br);
	return err;
}

/* BR */
