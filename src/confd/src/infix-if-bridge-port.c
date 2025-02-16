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

static const char *mstpd_cost_str(struct lyd_node *cost)
{
	const char *val = lyd_get_value(cost);

	if (!strcmp(val, "auto"))
		return "0";

	return val;
}

static int gen_stp_tree(FILE *mstpctl, const char *iface,
			const char *brname, struct lyd_node *tree)
{
	int mstid = 0;

	fprintf(mstpctl, "settreeportcost %s %s %d %s\n", brname, iface, mstid,
		mstpd_cost_str(lydx_get_child(tree, "internal-path-cost")));

	fprintf(mstpctl, "settreeportprio %s %s %d %s\n", brname, iface, mstid,
		lydx_get_cattr(tree, "priority"));

	return 0;
}

static int gen_stp(struct lyd_node *cif)
{
	const char *brname, *edge, *iface;
	struct lyd_node *stp, *cist;
	FILE *mstpctl;
	int err;

	brname = lydx_get_cattr(lydx_get_child(cif, "bridge-port"), "bridge");
	if (!brname)
		return 0;

	if (!lydx_get_xpathf(cif, "../interface[name='%s']/bridge/stp", brname))
		return 0;

	iface = lydx_get_cattr(cif, "name");
	stp = lydx_get_descendant(lyd_child(cif), "bridge-port", "stp", NULL);

	mstpctl = dagger_fopen_net_init(&confd.netdag, brname,
					NETDAG_INIT_DAEMON_LOWERS, "init-ports.mstpctl");
	if (!mstpctl)
		return -EIO;

	fputs("#!/sbin/mstpctl -b\n", mstpctl);

	edge = lydx_get_cattr(stp, "edge");
	if (!strcmp(edge, "true")) {
		fprintf(mstpctl, "setportautoedge %s %s no\n", brname, iface);
		fprintf(mstpctl, "setportadminedge %s %s yes\n", brname, iface);
	} else if (!strcmp(edge, "false")) {
		fprintf(mstpctl, "setportautoedge %s %s no\n", brname, iface);
		fprintf(mstpctl, "setportadminedge %s %s no\n", brname, iface);
	} else if (!strcmp(edge, "auto")) {
		fprintf(mstpctl, "setportautoedge %s %s yes\n", brname, iface);
	}

	cist = lydx_get_child(stp, "cist");
	err = gen_stp_tree(mstpctl, iface, brname, cist);
	if (err)
		goto out_close;

	fprintf(mstpctl, "setportpathcost %s %s %s\n", brname, iface,
		mstpd_cost_str(lydx_get_child(cist, "external-path-cost")));

out_close:
	fclose(mstpctl);
	return err;
}

static const char *get_port_egress_mode(const char *iface, int vid, const char *brname)
{
	static const char *modes[] = { "tagged", "untagged", NULL };
	const char **mode;

	for (mode = modes; *mode; mode++) {
		if (srx_get_str(confd.netdag.session,
				"/interfaces/interface[name='%s']"
				"/bridge/vlans/vlan[vid=%d]/%s[.='%s']",
				brname, vid, *mode, iface))
			return *mode;
	}

	return NULL;
}

static int gen_pvid_del(struct lyd_node *cif, const char *brname, int vid)
{
	const char *iface, *mode;
	FILE *exit;

	iface = lydx_get_cattr(cif, "name");

	mode = get_port_egress_mode(iface, vid, brname);
	if (!mode)
		/* Port is not a member of the VLAN anymore, so the
		 * PVID is already removed.
		 */
		return 0;

	exit = dagger_fopen_net_exit(&confd.netdag, brname,
				     NETDAG_EXIT_LOWERS_PROTO, "delete-pvids.bridge");
	if (!exit)
		return -EIO;

	/* Since PVID is a flag on the VLAN rather than a separate
	 * setting, the delete option becomes add-but-omit-pvid.
	 */
	fprintf(exit, "vlan add vid %d dev %s %s %s\n",
		vid, iface, mode, !strcmp(brname, iface) ? "self" : "");

	fclose(exit);
	return 0;
}

static int gen_pvid_add(struct lyd_node *cif, const char *brname, int vid)
{
	const char *iface, *mode;
	FILE *init;

	iface = lydx_get_cattr(cif, "name");

	mode = get_port_egress_mode(iface, vid, brname);
	if (!mode) {
		WARN("%s is not a member of VLAN %d: Ignoring PVID", iface, vid);
		return 0;
	}

	init = dagger_fopen_net_init(&confd.netdag, brname,
				     NETDAG_INIT_LOWERS_PROTO, "add-pvids.bridge");
	if (!init)
		return -EIO;

	fprintf(init, "vlan add vid %d dev %s pvid %s %s\n",
		vid, iface, mode, !strcmp(brname, iface) ? "self" : "");

	fclose(init);
	return 0;
}

static int gen_pvid(struct lyd_node *dif, struct lyd_node *cif)
{
	struct lyd_node *bridge, *pvid;
	struct lydx_diff pvdiff;
	const char *brname;
	int err;

	pvid = lydx_get_descendant(lyd_child(dif), "bridge-port", "pvid", NULL);
	if (!pvid || !lydx_get_diff(pvid, &pvdiff))
		return 0;

	brname = lydx_get_cattr(lydx_get_child(cif, "bridge-port"), "bridge");
	if (!brname)
		/* The interface is itself a bridge. */
		brname = lydx_get_cattr(cif, "name");

	bridge = lydx_get_descendant(lyd_child(dif), "bridge-port", "bridge", NULL);

	/* We only need to remove our old PVID in the case when this
	 * port has _not_ switched bridge.  Otherwise, all old VLAN
	 * config will be removed as a result of detaching from the
	 * old bridge.
	 */
	if (!bridge && pvdiff.old) {
		err = gen_pvid_del(cif, brname, atoi(pvdiff.old));
		if (err)
			return err;
	}

	if (pvdiff.new) {
		err = gen_pvid_add(cif, brname, atoi(pvdiff.new));
		if (err)
			return err;
	}

	return 0;
}

static int gen_link(struct lyd_node *dif, struct lyd_node *cif)
{
	struct lyd_node *bp, *flood, *mcast;
	const char *brname, *iface;
	int mrouter;
	FILE *next;

	if (!lydx_get_child(dif, "bridge-port"))
		return 0;

	bp = lydx_get_child(cif, "bridge-port");
	if (!bp)
		return 0;

	brname = lydx_get_cattr(bp, "bridge");
	if (!brname)
		/* The interface is itself a bridge. */
		return 0;

	iface = lydx_get_cattr(cif, "name");

	next = dagger_fopen_net_init(&confd.netdag, brname,
				     NETDAG_INIT_LOWERS, "add-ports.ip");
	if (!next)
		return -EIO;

	fprintf(next, "link set %s type bridge_slave", iface);

	flood = lydx_get_child(bp, "flood");
	fprintf(next, " bcast_flood %s", ONOFF(lydx_is_enabled(flood, "broadcast")));
	fprintf(next,       " flood %s", ONOFF(lydx_is_enabled(flood, "unicast")));
	fprintf(next, " mcast_flood %s", ONOFF(lydx_is_enabled(flood, "multicast")));

	if (lydx_is_enabled(flood, "unicast")) {
		/* proxy arp must be disabled while flood on, see man
		 * page.
		 */
		fprintf(next, " proxy_arp off");
		fprintf(next, " proxy_arp_wifi off");
	} else {
		/* XXX: proxy arp/wifi settings here */
	}

	mcast = lydx_get_child(bp, "multicast");

	mrouter = 1;
	if (!strcmp(lydx_get_cattr(mcast, "router"), "off"))
		mrouter = 0;
	else if (!strcmp(lydx_get_cattr(mcast, "router"), "permanent"))
		mrouter = 2;

	fprintf(next, " mcast_fast_leave %s mcast_router %d",
		ONOFF(lydx_is_enabled(mcast, "fast-leave")), mrouter);

	fprintf(next, "\n");
	fclose(next);
	return 0;
}

static int gen_join_leave(struct lyd_node *dif)
{
	struct lyd_node *bridge;
	struct lydx_diff brdiff;
	const char *iface;
	FILE *prev, *next;
	int err = 0;

	bridge = lydx_get_descendant(lyd_child(dif), "bridge-port", "bridge", NULL);
	if (!bridge || !lydx_get_diff(bridge, &brdiff))
		return 0;

	iface = lydx_get_cattr(dif, "name");

	if (brdiff.old) {
		prev = dagger_fopen_net_exit(&confd.netdag, brdiff.old,
					     NETDAG_EXIT_LOWERS, "delete-ports.ip");
		if (!prev)
			return -EIO;

		fprintf(prev, "link set %s nomaster\n", iface);
		fclose(prev);
	}

	if (brdiff.new) {
		next = dagger_fopen_net_init(&confd.netdag, brdiff.new,
					     NETDAG_INIT_LOWERS, "add-ports.ip");
		if (!next)
			return -EIO;

		fprintf(next, "link set %s master %s\n", iface, brdiff.new);
		fclose(next);
	}

	return err;
}

int bridge_port_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip)
{
	int err = 0;

	err = gen_join_leave(dif);
	if (err)
		return err;

	err = gen_link(dif, cif);
	if (err)
		return err;

	err = gen_pvid(dif, cif);
	if (err)
		return err;

	err = gen_stp(cif);
	if (err)
		return err;

	return 0;
}
