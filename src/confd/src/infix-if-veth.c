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
 * While the kernel atomically creates/destroys the pair, in `running`
 * the two sides are distinct interfaces. So we need to figure out
 * which one is going to create/delete the other - i.e. which side is
 * the "primary"
 */
bool veth_is_primary(struct lyd_node *cif)
{
	struct lyd_node *peer, *veth;
	const char *peername;

	veth = lydx_get_child(cif, "veth");
	peername = lydx_get_cattr(veth, "peer");
	peer = lydx_find_by_name(lyd_parent(cif), "interface", peername);

	/* At the moment, CNI code relies on one side of the pair
	 * remaining in the host namespace, and that that interface
	 * takes care of creating the pair.
	 */
	if (lydx_get_child(cif, "container-network"))
		return false;
	if (lydx_get_child(peer, "container-network"))
		return true;

	return strcmp(lydx_get_cattr(cif, "name"),
		      lydx_get_cattr(veth, "peer")) < 0;
}

int ifchange_cand_infer_veth(sr_session_ctx_t *session, const char *path)
{
	char *ifname, *type, *peer, *xpath, *val;
	sr_error_t err = SR_ERR_OK;
	size_t cnt = 0;

	xpath = xpath_base(path);
	if (!xpath)
		return SR_ERR_SYS;

	type = srx_get_str(session, "%s/type", xpath);
	if (!type)
		goto out;

	if (strcmp(type, "infix-if-type:veth"))
		goto out_free_type;

	ifname = srx_get_str(session, "%s/name", xpath);
	if (!ifname)
		goto out_free_type;

	peer = srx_get_str(session, "%s/veth/peer", xpath);
	if (!peer)
		goto out_free_ifname;

	err = srx_nitems(session, &cnt, "/interfaces/interface[name='%s']/name", peer);
	if (err || cnt)
		goto out_free_peer;

	val = "infix-if-type:veth";
	err = srx_set_str(session, val, 0, IF_XPATH "[name='%s']/type", peer);
	if (err) {
		ERROR("failed setting iface %s type %s, err %d", peer, val, err);
		goto out_free_peer;
	}

	err = srx_set_str(session, ifname, 0, IF_XPATH "[name='%s']/infix-interfaces:veth/peer", peer);
	if (err)
		ERROR("failed setting iface %s peer %s, err %d", peer, ifname, err);

out_free_peer:
	free(peer);
out_free_ifname:
	free(ifname);
out_free_type:
	free(type);
out:
	free(xpath);
	return err;
}

int netdag_gen_veth(struct dagger *net, struct lyd_node *dif,
		    struct lyd_node *cif, FILE *ip)
{
	const char *ifname = lydx_get_cattr(cif, "name");
	char ifname_args[64] = "", peer_args[64] = "";
	const char *mac, *peer;
	struct lyd_node *node;

	if (!veth_is_primary(cif))
		return 0;

	node = lydx_get_descendant(lyd_child(cif), "veth", NULL);
	if (!node)
		return -EINVAL;

	peer = lydx_get_cattr(node, "peer");

	mac = get_phys_addr(dif, NULL);
	if (mac)
		snprintf(ifname_args, sizeof(ifname_args), "address %s", mac);

	node = lydx_find_by_name(lyd_parent(cif), "interface", peer);
	if (node && (mac = get_phys_addr(node, NULL)))
		snprintf(peer_args, sizeof(peer_args), "address %s", mac);

	fprintf(ip, "link add dev %s %s type veth peer %s %s\n",
		ifname, ifname_args, peer, peer_args);

	return 0;
}

int veth_add_deps(struct lyd_node *cif)
{
	struct lyd_node *veth = lydx_get_child(cif, "veth");
	const char *peer;
	int err;

	if (veth_is_primary(cif))
		return 0;

	peer = lydx_get_cattr(veth, "peer");

	err = dagger_add_dep(&confd.netdag, lydx_get_cattr(cif, "name"), peer);
	if (err)
		return ERR_IFACE(cif, err, "Unable to depend on \"%s\"", peer);

	return 0;
}
