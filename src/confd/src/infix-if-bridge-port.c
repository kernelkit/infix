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

static const char *ixif_br_port_get_egress_mode(const char *iface, int vid,
						const char *brname)
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

static int ixif_br_port_gen_pvid_del(struct lyd_node *cif, const char *brname, int vid)
{
	const char *iface, *mode;
	FILE *exit;

	iface = lydx_get_cattr(cif, "name");

	mode = ixif_br_port_get_egress_mode(iface, vid, brname);
	if (!mode)
		/* Port is not a member of the VLAN anymore, so the
		 * PVID is already removed.
		 */
		return 0;

	exit = dagger_fopen_current(&confd.netdag, "exit", brname, 61, "delete-pvids.bridge");
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

static int ixif_br_port_gen_pvid_add(struct lyd_node *cif, const char *brname, int vid)
{
	const char *iface, *mode;
	FILE *init;

	iface = lydx_get_cattr(cif, "name");

	mode = ixif_br_port_get_egress_mode(iface, vid, brname);
	if (!mode) {
		WARN("%s is not a member of VLAN %d: Ignoring PVID", iface, vid);
		return 0;
	}

	init = dagger_fopen_next(&confd.netdag, "init", brname, 61, "add-pvids.bridge");
	if (!init)
		return -EIO;

	fprintf(init, "vlan add vid %d dev %s pvid %s %s\n",
		vid, iface, mode, !strcmp(brname, iface) ? "self" : "");

	fclose(init);
	return 0;
}

static int ixif_br_port_gen_pvid(struct lyd_node *dif, struct lyd_node *cif)
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
		err = ixif_br_port_gen_pvid_del(cif, brname, atoi(pvdiff.old));
		if (err)
			return err;
	}

	if (pvdiff.new) {
		err = ixif_br_port_gen_pvid_add(cif, brname, atoi(pvdiff.new));
		if (err)
			return err;
	}

	return 0;
}

static int ixif_br_port_gen_link(struct lyd_node *dif, struct lyd_node *cif)
{
	struct lyd_node *bp, *flood, *mcast;
	const char *brname, *iface;
	int mrouter;
	FILE *next;
	int err;

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

	err = dagger_add_dep(&confd.netdag, brname, iface);
	if (err)
		return ERR_IFACE(cif, err, "Unable to add dep \"%s\" to %s", iface, brname);

	next = dagger_fopen_next(&confd.netdag, "init", brname, 55, "add-ports.ip");
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

int ixif_br_port_gen_join_leave(struct lyd_node *dif)
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
		prev = dagger_fopen_current(&confd.netdag, "exit", brdiff.old, 55, "delete-ports.ip");
		if (!prev)
			return -EIO;

		fprintf(prev, "link set %s nomaster\n", iface);
		fclose(prev);
	}

	if (brdiff.new) {
		next = dagger_fopen_next(&confd.netdag, "init", brdiff.new, 55, "add-ports.ip");
		if (!next)
			return -EIO;

		fprintf(next, "link set %s master %s\n", iface, brdiff.new);
		fclose(next);
	}

	return err;
}

int ixif_br_port_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip)
{
	int err = 0;

	err = ixif_br_port_gen_join_leave(dif);
	if (err)
		return err;

	err = ixif_br_port_gen_link(dif, cif);
	if (err)
		return err;

	err = ixif_br_port_gen_pvid(dif, cif);
	if (err)
		return err;

	return 0;
}
