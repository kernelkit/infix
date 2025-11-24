/* SPDX-License-Identifier: BSD-3-Clause */
#include <srx/lyx.h>

#include "interfaces.h"

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

	fprintf(ip, "link add dev %s type wireguard\n", ifname);
	wg_fp = fopenf("w", WIREGUARD_CONFIG, ifname);
	if (!wg_fp)
		return -errno;

	fprintf(wg_fp, "[Interface]\n");
	fprintf(wg_fp, "PrivateKey = %s\n", private_key_data);
	fprintf(wg_fp, "ListenPort = %s\n", listen_port);

	LYX_LIST_FOR_EACH(lyd_child(wg), peer, "peers") {
		const char *public_key_bag_ref;
		const char *bag_preshared_key_ref, *bag_endpoint, *bag_endpoint_port, *bag_keepalive;
		struct lyd_node *pub_key_bag, *pub_key, *peer_override;

		public_key_bag_ref = lydx_get_cattr(peer, "public-key-bag");

		/* Get key-bag level settings (defaults for all keys in bag) */
		bag_preshared_key_ref = lydx_get_cattr(peer, "preshared-key");
		bag_endpoint = lydx_get_cattr(peer, "endpoint");
		bag_endpoint_port = lydx_get_cattr(peer, "endpoint-port");
		bag_keepalive = lydx_get_cattr(peer, "persistent-keepalive");

		pub_key_bag = lydx_get_xpathf(cif, "../../truststore/public-key-bags/public-key-bag[name='%s']",
					      public_key_bag_ref);
		if (!pub_key_bag)
			continue;

		/* Iterate through all public keys in the bag */
		LYX_LIST_FOR_EACH(lyd_child(pub_key_bag), pub_key, "public-key") {
			const char *public_key_name, *public_key_data;
			const char *preshared_key_ref, *preshared_key_data;
			const char *endpoint, *endpoint_port, *keepalive;
			struct lyd_node *allowed_ip, *psk_node;

			public_key_name = lydx_get_cattr(pub_key, "name");
			public_key_data = lydx_get_cattr(pub_key, "public-key");

			if (!public_key_data)
				continue;

			/* Check if there's a peer-specific override for this key */
			peer_override = lydx_get_xpathf(peer, "peer[public-key='%s']", public_key_name);

			/* Use peer override if exists, otherwise use key-bag level settings */
			preshared_key_ref = peer_override ? lydx_get_cattr(peer_override, "preshared-key") : NULL;
			if (!preshared_key_ref)
				preshared_key_ref = bag_preshared_key_ref;

			endpoint = peer_override ? lydx_get_cattr(peer_override, "endpoint") : NULL;
			if (!endpoint)
				endpoint = bag_endpoint;

			endpoint_port = peer_override ? lydx_get_cattr(peer_override, "endpoint-port") : NULL;
			if (!endpoint_port)
				endpoint_port = bag_endpoint_port;

			keepalive = peer_override ? lydx_get_cattr(peer_override, "persistent-keepalive") : NULL;
			if (!keepalive)
				keepalive = bag_keepalive;

			fprintf(wg_fp, "\n[Peer]\n");
			fprintf(wg_fp, "PublicKey = %s\n", public_key_data);

			if (preshared_key_ref) {
				psk_node = lydx_get_xpathf(cif, "../../keystore/symmetric-keys/symmetric-key[name='%s']",
							   preshared_key_ref);
				preshared_key_data = lydx_get_cattr(psk_node, "cleartext-symmetric-key");
				if (preshared_key_data)
					fprintf(wg_fp, "PresharedKey = %s\n", preshared_key_data);
			}

			if (endpoint) {
				if (!endpoint_port)
					endpoint_port = "51820";
				fprintf(wg_fp, "Endpoint = %s:%s\n", endpoint, endpoint_port);
			}

			/* Output all allowed IPs on a single line, comma-separated */
			{
				int first = 1;
				struct lyd_node *settings_node, *check_ip;

				/* Check peer override first, fall back to key-bag level */
				if (peer_override && lyd_child(peer_override)) {
					/* Check if peer override has any allowed-ips */
					int has_override_ips = 0;
					LYX_LIST_FOR_EACH(lyd_child(peer_override), check_ip, "allowed-ips") {
						has_override_ips = 1;
						break;
					}
					settings_node = has_override_ips ? peer_override : peer;
				} else {
					settings_node = peer;
				}

				LYX_LIST_FOR_EACH(lyd_child(settings_node), allowed_ip, "allowed-ips") {
					const char *ip_prefix = lyd_get_value(allowed_ip);
					if (ip_prefix) {
						fprintf(wg_fp, "%s%s", first ? "AllowedIPs = " : ", ", ip_prefix);
						first = 0;
					}
				}
				if (!first)
					fprintf(wg_fp, "\n");
			}

			if (keepalive)
				fprintf(wg_fp, "PersistentKeepalive = %s\n", keepalive);
		}
	}

	fclose(wg_fp);

	wg_sh = dagger_fopen_net_init(net, ifname, NETDAG_INIT_POST, "enable-wireguard.sh");

	fprintf(wg_sh, "wg setconf %s ", ifname);
	fprintf(wg_sh, WIREGUARD_CONFIG, ifname);
	fprintf(wg_sh, "\n");

	/* Remove wireguard config after tunnel is configured, to protect keys */
	fprintf(wg_sh, "rm -f ");
	fprintf(wg_sh, WIREGUARD_CONFIG, ifname);
	fprintf(wg_sh, "\n");
	fclose(wg_sh);

	return 0;
}
