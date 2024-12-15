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
int lag_gen_ports(struct dagger *net, struct lyd_node *dif, struct lyd_node *cif, FILE *ip)
{
	const char *ifname = lydx_get_cattr(cif, "name");
	struct lyd_node *node;
	const char *lagname;
	int err = 0;

	node = lydx_get_descendant(lyd_child(dif), "lag-port", NULL);
	if (node) {
		struct lydx_diff lagdiff = { 0 };

		if (!lydx_get_diff(lydx_get_child(node, "lag"), &lagdiff))
			goto fail;

		if (lagdiff.old) {
			FILE *fp;

			fp = dagger_fopen_current(net, "exit", lagdiff.old, 55, "exit.ip");
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

	fprintf(ip, "link set %s down master %s\n", ifname, lagname);

	/* We depend on lag to exist before we can set master ... */
	err = dagger_add_dep(net, ifname, lagname);
	if (err)
		ERROR("%s: unable to add dep to %s", ifname, lagname);
fail:
	return err;
}

int netdag_gen_lag(sr_session_ctx_t *session, struct dagger *net, struct lyd_node *dif,
		   struct lyd_node *cif, FILE *ip, int add)
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

	fprintf(ip, " type bond");

	mode = lydx_get_cattr(lag, "mode");
	if (!strcmp(mode, "lacp")) {
		struct lyd_node *lacp = lydx_get_child(lag, "lacp");

		if (add)
			fprintf(ip, " mode 802.3ad");

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
	 * No ARP Monitor support yet, and miimon is required for lacp mode.
	 * The kernel default interval is 100, which is needlessly often for
	 * us since we always follow the carrier.
	 *
	 * Note: debounce only supported by miimon, not the ARP Monitor.
	 */
	fprintf(ip, " miimon 1000 use_carrier 1");

	mon = lydx_get_descendant(lyd_child(cif), "link-monitor", "debounce", NULL);
	if (mon) {
		const char *msec;

		msec = lydx_get_cattr(mon, "up");
		if (msec)
			fprintf(ip, " updelay %s", msec);

		msec = lydx_get_cattr(mon, "down");
		if (msec)
			fprintf(ip, " downdelay %s", msec);
	}

	fputs("\n", ip);

	if (add) {
		struct lyd_node *node, *cifs;

		cifs = lydx_get_descendant(lyd_parent(cif), "interfaces", "interface", NULL);
		LYX_LIST_FOR_EACH(cifs, node, "interface")
			err += lag_gen_ports(net, NULL, node, ip);
	}

	return err;
}
