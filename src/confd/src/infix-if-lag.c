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

/*
 * The lag-port noode is only in the dif at initial creation, or removal.
 * When a lag is recreated, e.g., when changing mode, the lag-port is only
 * in the cif.
 *
 * To simplify confd dependency handling we always call this function, so
 * a lag member always has its lag master reset.  This is cruicial in the
 * case described above when the the lag is removed to change mode.
 */
int lag_port_gen(struct lyd_node *dif, struct lyd_node *cif)
{
	const char *ifname = lydx_get_cattr(cif, "name");
	struct lyd_node *node;
	const char *lagname;
	int err = 0;
	FILE *fp;

	node = lydx_get_descendant(lyd_child(dif), "lag-port", NULL);
	if (node) {
		struct lydx_diff lagdiff = { 0 };

		if (!lydx_get_diff(lydx_get_child(node, "lag"), &lagdiff))
			goto fail;

		if (lagdiff.old) {
			fp = dagger_fopen_net_exit(&confd.netdag, lagdiff.old,
						   NETDAG_EXIT_LOWERS, "delete-ports.ip");
			if (!fp) {
				err = -EIO;
				goto fail;
			}

			fprintf(fp, "link set %s nomaster\n", ifname);
			fclose(fp);

			return 0;
		}

		lagname = lagdiff.new;
	} else {
		node = lydx_get_descendant(lyd_child(cif), "lag-port", NULL);
		lagname = lydx_get_cattr(node, "lag");
		if (!node || !lagname)
			goto fail; /* done, nothing to do */
	}

	fp = dagger_fopen_net_init(&confd.netdag, lagname, NETDAG_INIT_LOWERS,
				   "add-ports.ip");
	if (!fp)
		return -EIO;

	fprintf(fp, "link set %s down master %s\n", ifname, lagname);
	if (lydx_is_enabled(cif, "enabled"))
		fprintf(fp, "link set dev %s up state up\n", ifname);
	fclose(fp);

	err = dagger_add_dep(&confd.netdag, lagname, ifname);
	if (err)
		ERROR("%s: unable to add dep to %s", ifname, lagname);
fail:
	return err;
}

int lag_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip, int add)
{
	const char *lagname = lydx_get_cattr(cif, "name");
	const char *op = add ? "add" : "set";
	struct lyd_node *lag, *mon;
	const char *mode;
	int err = 0;

	lag = lydx_get_descendant(lyd_child(cif), "lag", NULL);
	if (!lag)
		return -EINVAL;

	/* Must take lag down if changing mode */
	if (!add)
		fprintf(ip, "link set %s down\n", lagname);

	fprintf(ip, "link %s dev %s", op, lagname);

	if (add)
		link_gen_address(cif, ip);

	fprintf(ip, " type bond min_links 1");

	mode = lydx_get_cattr(lag, "mode");
	if (!strcmp(mode, "lacp")) {
		struct lyd_node *lacp = lydx_get_child(lag, "lacp");

		if (add)
			fprintf(ip, " mode 802.3ad ad_select bandwidth");

		mode = lydx_get_cattr(lacp, "mode");
		if (!strcmp(mode, "active"))
			mode = "on";
		else
			mode = "off";

		fprintf(ip, " lacp_rate %s lacp_active %s",
			lydx_get_cattr(lacp, "rate"), mode);
	} else {
		/* XXX: mode hard-coded for now for mv88e6xxxx operation */
		if (add)
			fprintf(ip, " mode balance-xor");
	}

	/*
	 * Required in lacp mode, we rely on it also in static mode.
	 * A previous attempt supported arp-monitor, but it does not
	 * work with mv88e6xxx link aggregates, unfortunately so was
	 * dropped for the final version.
	 */
	fprintf(ip, " miimon 100 use_carrier 1");

	mon = lydx_get_descendant(lyd_child(lag), "link-monitor", NULL);
	if (mon) {
		const struct lyd_node *debounce;

		debounce = lydx_get_descendant(lyd_child(mon), "debounce", NULL);
		if (debounce) {
			const char *msec;

			msec = lydx_get_cattr(mon, "up");
			if (msec)
				fprintf(ip, " updelay %s", msec);
			msec = lydx_get_cattr(mon, "down");
			if (msec)
				fprintf(ip, " downdelay %s", msec);
		}
	}

	fputs("\n", ip);

	/*
	 * When netdag_must_del() is triggered, this is when we reattach
	 * unmodified ports when recreating the lag.
	 */
	if (add) {
		struct lyd_node *node, *cifs;

		cifs = lydx_get_descendant(lyd_parent(cif), "interfaces", "interface", NULL);
		LYX_LIST_FOR_EACH(cifs, node, "interface")
			err += lag_port_gen(NULL, node);
	}

	return err;
}

int lag_add_deps(struct lyd_node *cif)
{
	const char *lagname = lydx_get_cattr(cif, "name");
	struct ly_set *ports;
	const char *portname;
	int err = 0;
	uint32_t i;

	ports = lydx_find_xpathf(cif, "../interface[lag-port/lag='%s']", lagname);
	if (!ports)
		return ERR_IFACE(cif, -ENOENT, "Unable to fetch lag ports");


	for (i = 0; i < ports->count; i++) {
		portname = lydx_get_cattr(ports->dnodes[i], "name");

		err = dagger_add_dep(&confd.netdag, lagname, portname);
		if (err) {
			ERR_IFACE(cif, err, "Unable to depend on \"%s\"", portname);
			break;
		}
	}

	ly_set_free(ports, NULL);
	return err;
}
