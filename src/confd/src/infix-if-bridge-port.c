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
