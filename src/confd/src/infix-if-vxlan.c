/* SPDX-License-Identifier: BSD-3-Clause */
#include <srx/lyx.h>

#include "ietf-interfaces.h"

int vxlan_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip)
{
	struct lyd_node *vxlan = NULL;

	vxlan = lydx_get_descendant(lyd_child(cif), "vxlan", NULL);
	if (!vxlan)
		return -EINVAL;

	fprintf(ip, "link add name %s type vxlan id %s local %s remote %s dstport %s",
		lydx_get_cattr(cif, "name"),
		lydx_get_cattr(vxlan, "vni"),
		lydx_get_cattr(vxlan, "local"),
		lydx_get_cattr(vxlan, "remote"),
		lydx_get_cattr(vxlan, "remote-port"));

	link_gen_address(cif, ip);

	fputc('\n', ip);
	return 0;
}
