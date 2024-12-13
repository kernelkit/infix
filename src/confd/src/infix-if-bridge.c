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

static void brport_pvid_adjust(FILE *br, struct lyd_node *vlan, int vid, const char *brport,
			       struct lydx_diff *pvidiff, int tagged)
{
	const char *type = tagged ? "tagged" : "untagged";
	struct lyd_node *port;

	LYX_LIST_FOR_EACH(lyd_child(vlan), port, type) {
		if (strcmp(brport, lyd_get_value(port)))
			continue;

		if (pvidiff->old && atoi(pvidiff->old) == vid)
			fprintf(br, "vlan add vid %d dev %s %s\n", vid, brport, type);
		if (pvidiff->new && atoi(pvidiff->new) == vid)
			fprintf(br, "vlan add vid %d dev %s pvid %s\n", vid, brport, type);
	}
}

/*
 * Called when only pvid is changed for a bridge-port.  Then we use the
 * cif data to iterate over all known VLANS for the given port.
 */
static int bridge_port_vlans(struct dagger *net, struct lyd_node *cif, const char *brname,
			     const char *brport, struct lydx_diff *pvidiff)
{
	struct lyd_node *bridge = lydx_find_by_name(lyd_parent(cif), "interface", brname);
	struct lyd_node *vlan, *vlans;
	int err = 0;
	FILE *br;

	vlans  = lydx_get_descendant(lyd_child(bridge), "bridge", "vlans", NULL);
	if (!vlans)
		goto done;

	br = dagger_fopen_next(net, "init", brname, 60, "init.bridge");
	if (!br) {
		err = -EIO;
		goto done;
	}

	LYX_LIST_FOR_EACH(lyd_child(vlans), vlan, "vlan") {
		int vid = atoi(lydx_get_cattr(vlan, "vid"));

		brport_pvid_adjust(br, vlan, vid, brport, pvidiff, 0);
		brport_pvid_adjust(br, vlan, vid, brport, pvidiff, 1);
	}

	fclose(br);
done:
	return err;
}

static void bridge_remove_vlan_ports(struct dagger *net, FILE *br, const char *brname,
				     int vid, struct lyd_node *ports, int tagged)
{
	struct lyd_node *port;

	LYX_LIST_FOR_EACH(lyd_child(ports), port, tagged ? "tagged" : "untagged") {
		enum lydx_op op = lydx_get_op(port);
		const char *brport = lyd_get_value(port);

		if (op != LYDX_OP_CREATE) {
			fprintf(br, "vlan del vid %d dev %s\n", vid, brport);
		}

	}
}

static void bridge_add_vlan_ports(struct dagger *net, FILE *br, const char *brname,
				  int vid, struct lyd_node *ports, int tagged)
{
	struct lyd_node *port;

	LYX_LIST_FOR_EACH(lyd_child(ports), port, tagged ? "tagged" : "untagged") {
		enum lydx_op op = lydx_get_op(port);
		const char *brport = lyd_get_value(port);

		if (op != LYDX_OP_DELETE) {
			int pvid = 0;
			srx_get_int(net->session, &pvid, SR_UINT16_T, IF_XPATH "[name='%s']/bridge-port/pvid", brport);

			fprintf(br, "vlan add vid %d dev %s %s %s %s\n", vid, brport, vid == pvid ? "pvid" : "",
				tagged ? "" : "untagged", strcmp(brname, brport) ? "" : "self");

		}
	}
}

static int bridge_diff_vlan_ports(struct dagger *net, FILE *br, const char *brname,
				  int vid, struct lyd_node *ports)
{
	/* First remove all VLANs that should that should be removed, see #676 */
	bridge_remove_vlan_ports(net, br, brname, vid, ports, 0);
	bridge_remove_vlan_ports(net, br, brname, vid, ports, 1);

	bridge_add_vlan_ports(net, br, brname, vid, ports, 0);
	bridge_add_vlan_ports(net, br, brname, vid, ports, 1);

	return 0;
}

static int bridge_vlan_settings(struct lyd_node *cif, const char **proto, int *vlan_mcast)
{
	struct lyd_node *vlans, *vlan;

	vlans = lydx_get_descendant(lyd_child(cif), "bridge", "vlans", NULL);
	if (vlans) {
		const char *type = lydx_get_cattr(vlans, "proto");
		int num = 0;

		*proto = bridge_tagtype2str(type);
		LYX_LIST_FOR_EACH(lyd_child(vlans), vlan, "vlan") {
			struct lyd_node *mcast;

			mcast = lydx_get_descendant(lyd_child(vlan), "multicast", NULL);
			if (mcast)
				*vlan_mcast += lydx_is_enabled(mcast, "snooping");

			num++;
		}

		return num;
	}

	return 0;
}

static void bridge_port_settings(FILE *next, const char *ifname, struct lyd_node *cif)
{
	struct lyd_node *bp, *flood, *mcast;
	int ucflood = 1;	/* default: flood unknown unicast */

	bp = lydx_get_descendant(lyd_child(cif), "bridge-port", NULL);
	if (!bp)
		return;

	fprintf(next, "link set %s type bridge_slave", ifname);
	flood = lydx_get_child(bp, "flood");
	if (flood) {
		ucflood = lydx_is_enabled(flood, "unicast");

		fprintf(next, " bcast_flood %s", ONOFF(lydx_is_enabled(flood, "broadcast")));
		fprintf(next, " flood %s",       ONOFF(ucflood));
		fprintf(next, " mcast_flood %s", ONOFF(lydx_is_enabled(flood, "multicast")));
	}

	if (ucflood) {
		/* proxy arp must be disabled while flood on, see man page */
		fprintf(next, " proxy_arp off");
		fprintf(next, " proxy_arp_wifi off");
	} else {
		/* XXX: proxy arp/wifi settings here */
	}

	mcast = lydx_get_child(bp, "multicast");
	if (mcast) {
		const char *router = lydx_get_cattr(mcast, "router");
		struct { const char *str; int val; } xlate[] = {
			{ "off",       0 },
			{ "auto",      1 },
			{ "permanent", 2 },
		};
		int mrouter = 1;

		for (size_t i = 0; i < NELEMS(xlate); i++) {
			if (strcmp(xlate[i].str, router))
				continue;

			mrouter = xlate[i].val;
			break;
		}

		fprintf(next, " mcast_fast_leave %s mcast_router %d",
			ONOFF(lydx_is_enabled(mcast, "fast-leave")),
			mrouter);
	}
	fprintf(next, "\n");
}

int bridge_gen_ports(struct dagger *net, struct lyd_node *dif, struct lyd_node *cif, FILE *ip)
{
	const char *ifname = lydx_get_cattr(cif, "name");
	struct lyd_node *node, *bridge;
	struct lydx_diff brdiff;
	int err = 0;

	node = lydx_get_descendant(lyd_child(dif), "bridge-port", NULL);
	if (!node)
		goto fail;

	/*
	 * If bridge is not in dif, then we only have bridge-port
	 * settings and can use cif instead for any new settings
	 * since we always set *all* port settings anyway.
	 */
	bridge = lydx_get_child(node, "bridge");
	if (!bridge) {
		struct lyd_node *pvid = lydx_get_child(node, "pvid");
		struct lydx_diff pvidiff;
		const char *brname;
		FILE *next;

		node = lydx_get_descendant(lyd_child(cif), "bridge-port", NULL);
		brname = lydx_get_cattr(node, "bridge");
		if (!node || !brname)
			goto fail;

		next = dagger_fopen_next(net, "init", ifname, 56, "init.ip");
		if (!next) {
			err = -EIO;
			goto fail;
		}
		bridge_port_settings(next, ifname, cif);
		fclose(next);

		/* Change in bridge port's PVID => change in VLAN port memberships */
		if (lydx_get_diff(pvid, &pvidiff))
			bridge_port_vlans(net, cif, brname, ifname, &pvidiff);

		err = dagger_add_dep(net, brname, ifname);
		if (err)
			return ERR_IFACE(cif, err, "Unable to add dep \"%s\" to %s", ifname, brname);
		goto fail;
	}

	if (lydx_get_diff(bridge, &brdiff) && brdiff.old) {
		FILE *prev;

		prev = dagger_fopen_current(net, "exit", brdiff.old, 55, "exit.ip");
		if (!prev) {
			err = -EIO;
			goto fail;
		}
		fprintf(prev, "link set %s nomaster\n", ifname);
		fclose(prev);
	}

	if (brdiff.new) {
		FILE *next;

		next = dagger_fopen_next(net, "init", brdiff.new, 55, "init.ip");
		if (!next) {
			err = -EIO;
			goto fail;
		}
		fprintf(next, "link set %s master %s\n", ifname, brdiff.new);
		bridge_port_settings(next, ifname, cif);
		fclose(next);

		err = dagger_add_dep(net, brdiff.new, ifname);
		if (err)
			return ERR_IFACE(cif, err, "Unable to add dep \"%s\" to %s", ifname, brdiff.new);
	}
fail:
	return err;
}

static int bridge_fwd_mask(struct lyd_node *cif)
{
	struct lyd_node *node, *proto;
	int fwd_mask = 0;

	node = lydx_get_descendant(lyd_child(cif), "bridge", NULL);
	if (!node)
		goto fail;

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

fail:
	return fwd_mask;
}

static int querier_mode(const char *mode)
{
	struct { const char *mode; int val; } table[] = {
		{ "off",   0 },
		{ "proxy", 1 },
		{ "auto",  2 },
	};

	for (size_t i = 0; i < NELEMS(table); i++) {
		if (strcmp(table[i].mode, mode))
			continue;

		return table[i].val;
	}

	return 0;		/* unknown: off */
}

void mcast_querier(const char *ifname, int vid, int mode, int interval)
{
	FILE *fp;

	DEBUG("mcast querier %s mode %d interval %d", ifname, mode, interval);
	if (!mode) {
		systemf("rm -f /etc/mc.d/%s-*.conf", ifname);
		systemf("initctl -bnq disable mcd");
		return;
	}

	fp = fopenf("w", "/etc/mc.d/%s-%d.conf", ifname, vid);
	if (!fp) {
		ERRNO("Failed creating querier configuration for %s", ifname);
		return;
	}

	fprintf(fp, "iface %s", ifname);
	if (vid > 0)
		fprintf(fp, " vlan %d", vid);
	fprintf(fp, " enable %sigmpv3 query-interval %d\n",
		mode == 1 ? "proxy-queries " : "", interval);
	fclose(fp);

	systemf("initctl -bnq enable mcd");
	systemf("initctl -bnq touch mcd");
}

static char *find_vlan_interface(sr_session_ctx_t *session, const char *brname, int vid)
{
	const char *fmt = "/interfaces/interface/vlan[id=%d and lower-layer-if='%s']";
	static char xpath[128];
       	struct lyd_node *iface;
	sr_data_t *data;
	int rc;

	snprintf(xpath, sizeof(xpath), fmt, vid, brname);
	rc = sr_get_data(session, xpath, 0, 0, 0, &data);
	if (rc || !data) {
		DEBUG("Skpping VLAN %d interface for %s", vid, brname);
		return NULL;
	}

	/* On match we should not need the if(iface) checks */
	iface = lydx_get_descendant(data->tree, "interfaces", "interface", NULL);
	if (iface)
		strlcpy(xpath, lydx_get_cattr(iface, "name"), sizeof(xpath));

	sr_release_data(data);
	if (iface)
		return xpath;

	return NULL;
}

static int vlan_mcast_settings(sr_session_ctx_t *session, FILE *br, const char *brname,
			       struct lyd_node *vlan, int vid)
{
	int interval, querier, snooping;
	struct lyd_node *mcast;
	const char *ifname;

	mcast = lydx_get_descendant(lyd_child(vlan), "multicast", NULL);
	if (!mcast)
		return 0;

	snooping = lydx_is_enabled(mcast, "snooping");
	querier = querier_mode(lydx_get_cattr(mcast, "querier"));

	fprintf(br, "vlan global set vid %d dev %s mcast_snooping %d",
		vid, brname, snooping);
	fprintf(br, " mcast_igmp_version 3 mcast_mld_version 2\n");

	interval = atoi(lydx_get_cattr(mcast, "query-interval"));
	ifname = find_vlan_interface(session, brname, vid);
	if (ifname)
		mcast_querier(ifname, 0, querier, interval);
	else
		mcast_querier(brname, vid, querier, interval);

	return 0;
}

static int bridge_mcast_settings(FILE *ip, const char *brname, struct lyd_node *cif, int vlan_mcast)
{
	int interval, querier, snooping;
	struct lyd_node *mcast;

	mcast = lydx_get_descendant(lyd_child(cif), "bridge", "multicast", NULL);
	if (!mcast) {
		mcast_querier(brname, 0, 0, 0);
		interval = snooping = querier = 0;
	} else {
		snooping = lydx_is_enabled(mcast, "snooping");
		querier  = querier_mode(lydx_get_cattr(mcast, "querier"));
		interval = atoi(lydx_get_cattr(mcast, "query-interval"));
	}

	fprintf(ip, " mcast_vlan_snooping %d", vlan_mcast ? 1 : 0);
	fprintf(ip, " mcast_snooping %d mcast_querier 0", vlan_mcast ? 1 : snooping);
	if (snooping)
		fprintf(ip, " mcast_igmp_version 3 mcast_mld_version 2");
	if (interval)
		fprintf(ip, " mcast_query_interval %d", interval * 100);

	if (!vlan_mcast)
		mcast_querier(brname, 0, querier, interval);
	else
		mcast_querier(brname, 0, 0, 0);

	return 0;
}

static int netdag_gen_multicast_filter(FILE *current, FILE *prev, const char *brname,
			  struct lyd_node *multicast_filter, int vid)
{
	const char *group = lydx_get_cattr(multicast_filter, "group");
	enum lydx_op op = lydx_get_op(multicast_filter);
	struct lyd_node * port;

	LYX_LIST_FOR_EACH(lyd_child(multicast_filter), port, "ports") {
		enum lydx_op port_op = lydx_get_op(port);
		if (op == LYDX_OP_DELETE) {
			fprintf(prev, "mdb del dev %s port %s ", brname, lydx_get_cattr(port, "port"));
			fprintf(prev, " grp %s ", group);
			if (vid)
				fprintf(prev, " vid %d ", vid);
			fputs("\n", prev);
		} else {
			fprintf(current, "mdb replace dev %s ", brname);
			if (port_op != LYDX_OP_DELETE)
				fprintf(current, " port %s ", lydx_get_cattr(port, "port"));
			fprintf(current, " grp %s ", group);
			if (vid)
				fprintf(current, " vid %d ", vid);
			fprintf(current, " %s\n", lydx_get_cattr(port, "state"));
		}
	}

	return 0;
}

static int netdag_gen_multicast_filters(struct dagger *net, FILE *current, const char *brname,
					struct lyd_node *multicast_filters, int vid) {
	struct lyd_node *multicast_filter;
	FILE *prev = NULL;
	int err = 0;

	prev = dagger_fopen_current(net, "exit", brname, 50, "exit.bridge");
	if (!prev) {
		/* check if in bootstrap (pre gen 0) */
		if (errno != EUNATCH) {
			err = -EIO;
			goto err;
		}
	}

	LYX_LIST_FOR_EACH(lyd_child(multicast_filters), multicast_filter, "multicast-filter") {
		netdag_gen_multicast_filter(current, prev, brname, multicast_filter, vid);
	}

	if(prev)
		fclose(prev);
err:
	return err;
}

int netdag_gen_bridge(sr_session_ctx_t *session, struct dagger *net, struct lyd_node *dif,
		      struct lyd_node *cif, FILE *ip, int add)
{
	struct lyd_node *vlans, *vlan, *multicast_filters;
	const char *brname = lydx_get_cattr(cif, "name");
	int vlan_filtering, fwd_mask, vlan_mcast = 0;
	const char *op = add ? "add" : "set";
	const char *proto;
	FILE *br = NULL;
	int err = 0;

	vlan_filtering = bridge_vlan_settings(cif, &proto, &vlan_mcast);
	fwd_mask = bridge_fwd_mask(cif);

	fprintf(ip, "link %s dev %s", op, brname);
	/*
	 * Must set base mac on add to prevent kernel from seeding ipv6
	 * addrgenmode eui64 with random mac, issue #357.
	 */
	if (add) {
		const char *mac = get_phys_addr(cif, NULL);

		if (!mac) {
			struct json_t *j;

			j = json_object_get(confd.root, "mac-address");
			if (j)
				mac = json_string_value(j);
		}
		if (mac)
			fprintf(ip, " address %s", mac);

		/* on failure, fall back to kernel's random mac */
	}

	/*
	 * Issue #198: we require explicit VLAN assignment for ports
	 *             when VLAN filtering is enabled.  We strongly
	 *             believe this is the only sane way of doing it.
	 * Issue #310: malplaced 'vlan_default_pvid 0'
	 */
	fprintf(ip, " type bridge group_fwd_mask %d mcast_flood_always 1"
		" vlan_filtering %d vlan_default_pvid 0",
		fwd_mask, vlan_filtering ? 1 : 0);

	if ((err = bridge_mcast_settings(ip, brname, cif, vlan_mcast)))
		goto out;

	br = dagger_fopen_next(net, "init", brname, 60, "init.bridge");
	if (!br) {
		err = -EIO;
		goto out;
	}

	if (!vlan_filtering) {
		fputc('\n', ip);

		multicast_filters = lydx_get_descendant(lyd_child(dif), "bridge", "multicast-filters", NULL);
		if (multicast_filters)
			err = netdag_gen_multicast_filters(net, br, brname, multicast_filters, 0);
		goto out_close_br;
	} else if (!proto) {
		fputc('\n', ip);
		ERROR("%s: unsupported bridge proto", brname);
		err = -ENOSYS;
		goto out_close_br;
	}
	fprintf(ip, " vlan_protocol %s\n", proto);

	vlans = lydx_get_descendant(lyd_child(dif), "bridge", "vlans", NULL);
	if (!vlans)
		goto out_close_br;

	LYX_LIST_FOR_EACH(lyd_child(vlans), vlan, "vlan") {
		int vid = atoi(lydx_get_cattr(vlan, "vid"));

		err = bridge_diff_vlan_ports(net, br, brname, vid, vlan);
		if (err)
			break;

		/* MDB static groups */
		multicast_filters = lydx_get_child(vlan, "multicast-filters");
		if (multicast_filters) {
			if ((err = netdag_gen_multicast_filters(net, br, brname, multicast_filters, vid)))
				break;
		}
	}

	/* need the vlans created before we can set features on them */
	vlans = lydx_get_descendant(lyd_child(cif), "bridge", "vlans", NULL);
	if (vlans) {
		LYX_LIST_FOR_EACH(lyd_child(vlans), vlan, "vlan") {
			int vid = atoi(lydx_get_cattr(vlan, "vid"));

			err = vlan_mcast_settings(session, br, brname, vlan, vid);
			if (err)
				break;
		}
	}
out_close_br:
	fclose(br);
out:
	return err;
}
