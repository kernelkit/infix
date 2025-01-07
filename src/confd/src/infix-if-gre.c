/* SPDX-License-Identifier: BSD-3-Clause */
#include <srx/lyx.h>

#include "ietf-interfaces.h"

int gre_gen(struct dagger *net, struct lyd_node *dif,
		    struct lyd_node *cif, FILE *ip)
{
	const char *ifname, *iftype, *local, *remote, *mac = NULL;
	struct lyd_node *node = NULL;
	char gretype[10] = "";
	int  ipv6;

	ifname = lydx_get_cattr(cif, "name");
	iftype = lydx_get_cattr(cif, "type");

	node = lydx_get_descendant(lyd_child(cif), "gre", NULL);
	if (!node)
		return -EINVAL;

	local = lydx_get_cattr(node, "local");
	remote = lydx_get_cattr(node, "remote");
	ipv6 = !!strstr(local, ":");

	if (!strcmp(iftype, "infix-if-type:gre")) {
		snprintf(gretype, sizeof(gretype), "%sgre", ipv6 ? "ip6" : "");

	}
	else if (!strcmp(iftype, "infix-if-type:gretap")) {
		snprintf(gretype, sizeof(gretype), "%sgretap", ipv6 ? "ip6" : "");
		mac = get_phys_addr(cif, NULL);
	}

	fprintf(ip, "link add name %s type %s local %s remote %s", ifname, gretype, local, remote);
	if (mac)
		fprintf(ip, "address %s\n", mac);
	else
		fprintf(ip, "\n");

	return 0;
}
