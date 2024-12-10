/* SPDX-License-Identifier: BSD-3-Clause */
#include <libite/lite.h>
#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "ietf-interfaces.h"

int netdag_gen_gre(struct dagger *net, struct lyd_node *dif,
		    struct lyd_node *cif, FILE *ip)
{
	const char *ifname, *iftype, *local, *remote;
	struct lyd_node *node = NULL;
//	char ifname_args[64] = "";
	char gretype[10] = "";
	int  ipv6;

	ifname = lydx_get_cattr(cif, "name");
	iftype = lydx_get_cattr(cif, "type");
//	mac = get_phys_addr(cif, NULL);
	//if (mac)
	//	snprintf(ifname_args, sizeof(ifname_args), "address %s", mac);
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
	}

	fprintf(ip, "link add name %s type %s local %s remote %s\n", ifname, gretype, local, remote);
	ERROR("link add name %s type %s local %s remote %s", ifname, gretype, local, remote);
	return 0;
}
