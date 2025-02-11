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

static bool iface_uses_autoneg(struct lyd_node *cif)
{
	struct lyd_node *aneg = lydx_get_descendant(lyd_child(cif), "ethernet",
						    "auto-negotiation", NULL);

	/* Because `ieee802-ethernet-interface` declares
	 * `auto-negotiation` as a presence container, the `enabled`
	 * leaf, although `true` by default, is not set if the whole
	 * container is absent. Since auto-negotiation is the expected
	 * default behavior for most Ethernet links, we choose to
	 * enable it in these situations.
	 */
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
	FILE *fp;

	fp = dagger_fopen_net_init(net, ifname, NETDAG_INIT_PHYS, "ethtool-aneg.sh");
	if (!fp)
		return -EIO;

	fprintf(fp, "ethtool --pause %s autoneg %s rx off tx off\n",
		ifname, iface_uses_autoneg(cif) ? "on" : "off");
	fclose(fp);

	return 0;
}

static int netdag_gen_ethtool_autoneg(struct dagger *net, struct lyd_node *cif)
{
	struct lyd_node *eth = lydx_get_child(cif, "ethernet");
	const char *ifname = lydx_get_cattr(cif, "name");
	const char *speed, *duplex;
	int mbps, err = 0;
	FILE *fp;

	fp = dagger_fopen_net_init(net, ifname, NETDAG_INIT_PHYS, "ethtool-aneg.sh");
	if (!fp)
		return -EIO;

	fprintf(fp, "ethtool --change %s autoneg ", ifname);

	if (iface_uses_autoneg(cif)) {
		fputs("on\n", fp);
	} else {
		speed = lydx_get_cattr(eth, "speed");
		if (!speed) {
			sr_session_set_error_message(net->session, "%s: "
						     "\"speed\" must be specified "
						     "when auto-negotiation is disabled", ifname);
			err = -EINVAL;
			goto out;
		}

		mbps = (int)(atof(speed) * 1000.);
		if (!((mbps == 10) || (mbps == 100))) {
				sr_session_set_error_message(net->session, "%s: "
						     "\"speed\" must be either 0.01 or 0.1 "
						     "when auto-negotiation is disabled", ifname);
			err = -EINVAL;
			goto out;
		}

		duplex = lydx_get_cattr(eth, "duplex");
		if (!duplex || (strcmp(duplex, "full") && strcmp(duplex, "half"))) {
			sr_session_set_error_message(net->session, "%s: "
						     "\"duplex\" must be either "
						     "\"full\" or \"half\" "
						     "when auto-negotiation is disabled", ifname);
			err = -EINVAL;
			goto out;
		}

		fprintf(fp,"off speed %d duplex %s\n", mbps, duplex);
	}
out:
	fclose(fp);
	return err;

}

int netdag_gen_ethtool(struct dagger *net, struct lyd_node *cif, struct lyd_node *dif)
{
	struct lyd_node *eth = lydx_get_child(dif, "ethernet");
	int err;

	/*
	 * Story time: when assigning a physical interface to a container, and then
	 *             removing it, even though our type may be 'etherlike' we will
	 *             get the following from sysrepo:
	 *
	 *               "ieee802-ethernet-interface:ethernet": {
	 *                 "@": {
	 *                   "yang:operation": "delete"
	 *                 },
	 *                 "duplex": "full"
	 *               },
	 *
	 *             Hence this "redundant" check.
	 */
	if (iftype_from_iface(cif) != IFT_ETH)
		return 0;

	if (!eth)
		return 0;

	if (dagger_is_bootstrap(net) ||
	    lydx_get_descendant(lyd_child(eth), "auto-negotiation", "enable", NULL)) {
		err = netdag_gen_ethtool_flow_control(net, cif);
		if (err)
			return err;
	}

	if (dagger_is_bootstrap(net) ||
	    lydx_get_descendant(lyd_child(eth), "auto-negotiation", "enable", NULL) ||
	    lydx_get_child(eth, "speed") ||
	    lydx_get_child(eth, "duplex")) {
		err = netdag_gen_ethtool_autoneg(net, cif);
		if (err)
			return err;
	}

	return 0;
}
