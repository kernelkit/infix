/* SPDX-License-Identifier: BSD-3-Clause */
#include <libyang/tree_data.h>
#include <srx/lyx.h>

#include "ietf-interfaces.h"

#define WIREGUARD_KEYS      "/etc/wireguard"

int wireguard_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip, struct lyd_node *tree, struct dagger *net)
{
	struct lyd_node *wireguard, *local_private_key_node, *remote_public_key_node;
	const char *local_key_name, *remote_key_name, /**psk_name, */*name, *remote_public_key;
	const char *remote, *remote_port;
	FILE *wg;

	name = lydx_get_cattr(cif, "name");
	wg = dagger_fopen_net_init(net, name, NETDAG_INIT, "wg.sh");

	wireguard = lydx_get_child(cif, "wireguard");

	//local = lydx_get_cattr(wireguard, "local");
	remote = lydx_get_cattr(wireguard, "remote");
	//local_port = lydx_get_cattr(wireguard, "local-port");
	remote_port = lydx_get_cattr(wireguard, "remote-port");
	local_key_name = lydx_get_cattr(wireguard, "key-pair");
	remote_key_name = lydx_get_cattr(wireguard, "remote-public-key");
//	psk_name = lydx_get_cattr(wireguard, "preshared-key");
	local_private_key_node = lydx_get_xpathf(tree, "/ietf-keystore:keystore/asymmetric-keys/asymmetric-key[name='%s']/cleartext-private-key", local_key_name);
	remote_public_key_node = lydx_get_xpathf(tree, "/ietf-keystore:keystore/asymmetric-keys/asymmetric-key[name='%s']/public-key", remote_key_name);
	remote_public_key = lydx_get_cattr(remote_public_key_node, "public-key");

	fprintf(ip, "link add name %s type wireguard\n", name);
	fprintf(wg, "echo %s > /etc/wireguard/%s-local.priv\n", lydx_get_cattr(local_private_key_node, "cleartext-private-key"), name);
//	fprintf(wg, "echo %s > /etc/wireguard/%s-remote.pub\n", lydx_get_cattr(remote_public_key, "cleartext-private-key"), name);
	fprintf(wg, "wg set %s private-key /etc/wireguard/%s-local.priv\n", name, name);
	fprintf(wg, "wg set %s peer /etc/wireguard/%s-remote.pub\n", name, name);
	if (remote) {
		fprintf(wg, "wg set %s peer %s endpoint %s:%s\n", name,  remote_public_key, remote, remote_port);
	}

	return 0;
}
