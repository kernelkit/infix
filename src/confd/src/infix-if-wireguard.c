/* SPDX-License-Identifier: BSD-3-Clause */
#include <srx/lyx.h>

#include "ietf-interfaces.h"

#define WIREGUARD_CONFIG "/run/wireguard-%s.conf"

int wireguard_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip, struct dagger *net)
{
	const char *ifname = lydx_get_cattr(cif, "name");
	const char *listen_port, *private_key_ref;
	const char *private_key_data;
	struct lyd_node *wg, *peer, *key_node;
	FILE *wg_fp = NULL;
	FILE *wg_sh = NULL;

	wg = lydx_get_child(cif, "wireguard");
	if (!wg)
		return -EINVAL;

	listen_port = lydx_get_cattr(wg, "listen-port");
	if (!listen_port)
		listen_port = "51820";

	private_key_ref = lydx_get_cattr(wg, "private-key");

	key_node = lydx_get_xpathf(cif, "../../keystore/asymmetric-keys/asymmetric-key[name='%s']", private_key_ref);
	private_key_data = lydx_get_cattr(key_node, "cleartext-private-key");

	wg_fp = fopenf("w", WIREGUARD_CONFIG, ifname);
	if (!wg_fp)
		return -errno;

	fprintf(wg_fp, "[Interface]\n");
	fprintf(wg_fp, "PrivateKey = %s\n", private_key_data);
	fprintf(wg_fp, "ListenPort = %s\n", listen_port);

	LYX_LIST_FOR_EACH(lyd_child(wg), peer, "peer") {
		const char *public_key_bag_ref, *public_key_ref, *preshared_key_ref;
		const char *public_key_data, *preshared_key_data;
		const char *endpoint, *endpoint_port;
		const char *keepalive;
		struct lyd_node *allowed_ip, *pub_key_node, *psk_node;

		fprintf(wg_fp, "\n[Peer]\n");

		public_key_bag_ref = lydx_get_cattr(peer, "public-key-bag");
		public_key_ref = lydx_get_cattr(peer, "public-key");

		pub_key_node = lydx_get_xpathf(cif, "../../truststore/public-key-bags/public-key-bag[name='%s']/public-key[name='%s']",
					       public_key_bag_ref, public_key_ref);
		public_key_data = lydx_get_cattr(pub_key_node, "public-key");

		fprintf(wg_fp, "PublicKey = %s\n", public_key_data);

		preshared_key_ref = lydx_get_cattr(peer, "preshared-key");
		if (preshared_key_ref) {
			psk_node = lydx_get_xpathf(cif, "../../keystore/symmetric-keys/symmetric-key[name='%s']",
						   preshared_key_ref);
			preshared_key_data = lydx_get_cattr(psk_node, "cleartext-key");
			if (preshared_key_data)
				fprintf(wg_fp, "PresharedKey = %s\n", preshared_key_data);
		}

		endpoint = lydx_get_cattr(peer, "endpoint");
		if (endpoint) {
			endpoint_port = lydx_get_cattr(peer, "endpoint-port");
			if (!endpoint_port)
				endpoint_port = "51820";
			fprintf(wg_fp, "Endpoint = %s:%s\n", endpoint, endpoint_port);
		}

		LYX_LIST_FOR_EACH(lyd_child(peer), allowed_ip, "allowed-ips") {
			const char *ip_prefix = lydx_get_cattr(allowed_ip, ".");
			if (ip_prefix)
				fprintf(wg_fp, "AllowedIPs = %s\n", ip_prefix);
		}

		keepalive = lydx_get_cattr(peer, "persistent-keepalive");
		if (keepalive)
			fprintf(wg_fp, "PersistentKeepalive = %s\n", keepalive);
	}

	fclose(wg_fp);

	wg_sh = dagger_fopen_net_init(net, ifname, NETDAG_INIT_POST, "enable-wireguard.sh");

	fprintf(wg_sh, "wg setconf %s ", ifname);
	fprintf(wg_sh, WIREGUARD_CONFIG, ifname);
	fprintf(wg_sh, "\n");

	fprintf(wg_sh, "rm -f ");
	fprintf(wg_sh, WIREGUARD_CONFIG, ifname);
	fprintf(wg_sh, "\n");
	fclose(wg_sh);

	return 0;
}
