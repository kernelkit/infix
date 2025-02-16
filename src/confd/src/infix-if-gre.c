/* SPDX-License-Identifier: BSD-3-Clause */
#include <srx/lyx.h>

#include "ietf-interfaces.h"

int gre_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip)
{
	const char *local, *remote;
	struct lyd_node *gre;
	int ipv6;

	gre = lydx_get_child(cif, "gre");
	local = lydx_get_cattr(gre, "local");
	remote = lydx_get_cattr(gre, "remote");

	ipv6 = !!strstr(local, ":");

	fprintf(ip, "link add name %s",
		lydx_get_cattr(cif, "name"));

	switch (iftype_from_iface(cif)) {
	case IFT_GRE:
		fprintf(ip, " type %sgre", ipv6 ? "ip6": "");
		break;
	case IFT_GRETAP:
		link_gen_address(cif, ip);
		fprintf(ip, " type %sgretap", ipv6 ? "ip6": "");
		break;
	default:
		return -EINVAL;
	}

	fprintf(ip, " local %s remote %s\n", local, remote);
	return 0;
}
