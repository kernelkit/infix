/* SPDX-License-Identifier: BSD-3-Clause */
#include <srx/lyx.h>

#include "ietf-interfaces.h"

int gre_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip)
{
	const char *local, *remote, *ttl, *tos, *pmtudisc;
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

	fprintf(ip, " local %s remote %s", local, remote);

	ttl = lydx_get_cattr(gre, "ttl");
	if (ttl)
		fprintf(ip, " ttl %s", ttl);

	tos = lydx_get_cattr(gre, "tos");
	if (tos)
		fprintf(ip, " tos %s", tos);

	pmtudisc = lydx_get_cattr(gre, "pmtu-discovery");
	if (pmtudisc && !strcmp(pmtudisc, "false"))
		fprintf(ip, " nopmtudisc");

	fputc('\n', ip);
	return 0;
}
