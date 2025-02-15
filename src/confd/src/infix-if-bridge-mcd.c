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

static int gen_vlan(struct lyd_node *cif, struct lyd_node *vlan, FILE *conf)
{
	const char *iface, *querier, *upper;
	struct lyd_node *mcast;
	int interval, vid;

	mcast = lydx_get_child(vlan, "multicast");
	if (!mcast)
		return 0;

	querier = lydx_get_cattr(mcast, "querier");
	if (!strcmp(querier, "off"))
		return 0;

	interval = atoi(lydx_get_cattr(mcast, "query-interval"));

	iface = lydx_get_cattr(cif, "name");
	vid = atoi(lydx_get_cattr(vlan, "vid"));

	upper = srx_get_str(confd.netdag.session,
			    "/interfaces/interface/vlan[id=%d and lower-layer-if='%s']/../name",
			    vid, iface);
	if (upper)
		/* This bridge VLAN is locally terminated.  Therefore,
		 * run mcd on the upper interface, so that we can
		 * inject queries with proper source IPs and not just
		 * proxy queries.
		 */
		fprintf(conf, "iface %s", upper);
	else
		fprintf(conf, "iface %s vlan %d", iface, vid);

	fprintf(conf, " enable %s igmpv3 query-interval %d\n",
		!strcmp(querier, "proxy") ? "proxy-queries" : "", interval);

	return 0;
}

static int gen_bridge(struct lyd_node *cif, FILE *conf)
{
	struct lyd_node *vlans, *vlan, *mcast;
	const char *iface, *querier;
	int interval, err;

	iface = lydx_get_cattr(cif, "name");
	mcast = lydx_get_descendant(lyd_child(cif), "bridge", "multicast", NULL);

	if (mcast) {
		querier = lydx_get_cattr(mcast, "querier");
		if (!querier || !strcmp(querier, "off"))
			return 0;

		interval = atoi(lydx_get_cattr(mcast, "query-interval"));

		fprintf(conf, "iface %s enable %s igmpv3 query-interval %d\n",
			iface, !strcmp(querier, "proxy") ? "proxy-queries" : "",
			interval);
		return 0;
	}

	vlans = lydx_get_descendant(lyd_child(cif), "bridge", "vlans", NULL);
	if (!vlans)
		return 0;

	LYX_LIST_FOR_EACH(lyd_child(vlans), vlan, "vlan") {
		err = gen_vlan(cif, vlan, conf);
		if (err)
			return err;
	}

	return 0;
}

int bridge_mcd_gen(struct lyd_node *cifs)
{
	FILE *conf, *stop, *start;
	struct lyd_node *cif;
	int err = 0;
	bool empty;

	conf = fopen("/etc/mc.d/bridges.conf.next", "w");
	if (!conf)
		return -EIO;

	LYX_LIST_FOR_EACH(cifs, cif, "interface") {
		if (iftype_from_iface(cif) != IFT_BRIDGE)
			continue;

		err = gen_bridge(cif, conf);
		if (err)
			break;
	}

	empty = ftell(conf) == 0;
	fclose(conf);

	if (err)
		goto out_remove;

	if (!empty) {
		start = dagger_fopen_next(&confd.netdag, "init", "@post", 50, "mcd-start.sh");
		if (!start) {
			err = -EIO;
			goto out_remove;
		}

		fputs("mv /etc/mc.d/bridges.conf.next /etc/mc.d/bridges.conf\n", start);
		fputs("initctl -bnq enable mcd\n", start);
		fputs("initctl -bnq touch mcd\n", start);
		fclose(start);
		return 0;
	} else if (!dagger_is_bootstrap(&confd.netdag)) {
		stop = dagger_fopen_current(&confd.netdag, "exit", "@pre", 50, "mcd-stop.sh");
		if (!stop) {
			err = -EIO;
			goto out_remove;
		}

		fputs("initctl -bnq stop mcd\n", stop);
		fputs("initctl -bnq disable mcd\n", stop);
		fclose(stop);
	}

out_remove:
	if (remove("/etc/mc.d/bridges.conf.next"))
		ERRNO("Failed removing /etc/mc.d/bridges.conf.next");

	return err;
}
