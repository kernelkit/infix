/* SPDX-License-Identifier: BSD-3-Clause */
#include <srx/lyx.h>

#include "ietf-interfaces.h"

int vxlan_gen(struct dagger *net, struct lyd_node *dif,
	      struct lyd_node *cif, FILE *ip)
{
	const char *ifname, *local, *remote, *mac = NULL;
	const char *vni, *remote_port;
	struct lyd_node *node = NULL;

	ifname = lydx_get_cattr(cif, "name");
	node = lydx_get_descendant(lyd_child(cif), "vxlan", NULL);
	if (!node)
		return -EINVAL;

	local = lydx_get_cattr(node, "local");
	remote = lydx_get_cattr(node, "remote");
	vni = lydx_get_cattr(node, "vni");
	remote_port = lydx_get_cattr(node, "remote-port");
	fprintf(ip, "link add name %s type vxlan id %s local %s remote %s dstport %s", ifname, vni, local, remote, remote_port);
	if (mac)
		fprintf(ip, "address %s\n", mac);
	else
		fprintf(ip, "\n");

	return 0;
}
