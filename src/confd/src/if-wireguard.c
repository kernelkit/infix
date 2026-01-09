/* SPDX-License-Identifier: BSD-3-Clause */
#include <srx/lyx.h>

#include "interfaces.h"

#define WIREGUARD_CONFIG "/run/wireguard-%s.conf"

/* Helper to get a peer setting with override logic:
 * 1. Check peer-specific override
 * 2. Fall back to key-bag level default
 */
static const char *get_peer_setting(struct lyd_node *peer_override, struct lyd_node *bag_peer,
				    const char *setting_name)
{
	const char *value = NULL;

	if (peer_override)
		value = lydx_get_cattr(peer_override, setting_name);
	if (!value)
		value = lydx_get_cattr(bag_peer, setting_name);

	return value;
}

static void write_allowed_ips(FILE *wg_fp, struct lyd_node *peer_override, struct lyd_node *bag_peer)
{
	struct lyd_node *settings_node, *allowed_ip, *check_ip;
	int first = 1;

	/* Determine which node has the allowed-ips to use:
	 * If peer override exists and has any allowed-ips, use those.
	 * Otherwise use bag-level allowed-ips.
	 */
	settings_node = bag_peer;  /* Default to bag level */
	if (peer_override && lyd_child(peer_override)) {
		LYX_LIST_FOR_EACH(lyd_child(peer_override), check_ip, "allowed-ips") {
			settings_node = peer_override;  /* Override has IPs, use them */
			break;
		}
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

static void write_peer(FILE *wg_fp, struct lyd_node *cif, struct lyd_node *bag_peer,
		       struct lyd_node *peer_override, const char *public_key_data)
{
	const char *preshared_key_ref, *preshared_key_data;
	const char *endpoint, *endpoint_port, *keepalive;
	struct lyd_node *psk_node;

	fprintf(wg_fp, "\n[Peer]\n");
	fprintf(wg_fp, "PublicKey = %s\n", public_key_data);

	preshared_key_ref = get_peer_setting(peer_override, bag_peer, "preshared-key");
	endpoint = get_peer_setting(peer_override, bag_peer, "endpoint");
	endpoint_port = get_peer_setting(peer_override, bag_peer, "endpoint-port");
	keepalive = get_peer_setting(peer_override, bag_peer, "persistent-keepalive");

	if (preshared_key_ref) {
		psk_node = lydx_get_xpathf(cif, "../../keystore/symmetric-keys/symmetric-key[name='%s']",
					   preshared_key_ref);
		if (psk_node) {
			preshared_key_data = lydx_get_cattr(psk_node, "cleartext-symmetric-key");
			if (preshared_key_data)
				fprintf(wg_fp, "PresharedKey = %s\n", preshared_key_data);
		}
	}

	if (endpoint) {
		if (!endpoint_port)
			endpoint_port = "51820";  /* Default port */
		fprintf(wg_fp, "Endpoint = %s:%s\n", endpoint, endpoint_port);
	}

	write_allowed_ips(wg_fp, peer_override, bag_peer);

	if (keepalive)
		fprintf(wg_fp, "PersistentKeepalive = %s\n", keepalive);
}


int wireguard_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip, struct dagger *net)
{
	const char *ifname, *listen_port, *private_key_ref, *private_key_data;
	struct lyd_node *wg, *key_node, *bag_peer, *pub_key_bag;
	FILE *wg_fp = NULL;
	FILE *wg_sh = NULL;
	mode_t old_umask;

	ifname = lydx_get_cattr(cif, "name");
	wg = lydx_get_child(cif, "wireguard");
	if (!wg)
		return -EINVAL;

	listen_port = lydx_get_cattr(wg, "listen-port");
	private_key_ref = lydx_get_cattr(wg, "private-key");

	key_node = lydx_get_xpathf(cif, "../../keystore/asymmetric-keys/asymmetric-key[name='%s']",
				   private_key_ref);
	private_key_data = lydx_get_cattr(key_node, "cleartext-private-key");

	fprintf(ip, "link add dev %s type wireguard\n", ifname);

	/* Set umask to create config file with limited permissions (0600) */
	old_umask = umask(0177);
	wg_fp = fopenf("w", WIREGUARD_CONFIG, ifname);
	umask(old_umask);
	if (!wg_fp)
		return -errno;

	fprintf(wg_fp, "[Interface]\n");
	fprintf(wg_fp, "PrivateKey = %s\n", private_key_data);
	fprintf(wg_fp, "ListenPort = %s\n", listen_port);

	LYX_LIST_FOR_EACH(lyd_child(wg), bag_peer, "peers") {
		const char *public_key_bag_ref = lydx_get_cattr(bag_peer, "public-key-bag");

		pub_key_bag = lydx_get_xpathf(cif, "../../truststore/public-key-bags/public-key-bag[name='%s']",
					      public_key_bag_ref);
		if (pub_key_bag) {
			struct lyd_node *pub_key, *peer_override;
			const char *public_key_name, *public_key_data;

			LYX_LIST_FOR_EACH(lyd_child(pub_key_bag), pub_key, "public-key") {
				public_key_name = lydx_get_cattr(pub_key, "name");
				public_key_data = lydx_get_cattr(pub_key, "public-key");

				if (!public_key_data)
					continue;

				/* Check if there's a peer-specific override for this key */
				peer_override = lydx_get_xpathf(bag_peer, "peer[public-key='%s']", public_key_name);

				write_peer(wg_fp, cif, bag_peer, peer_override, public_key_data);
			}
		}
	}

	fclose(wg_fp);

	/* Create activation script */
	wg_sh = dagger_fopen_net_init(net, ifname, NETDAG_INIT_POST, "enable-wireguard.sh");

	fprintf(wg_sh, "wg setconf %s ", ifname);
	fprintf(wg_sh, WIREGUARD_CONFIG, ifname);
	fprintf(wg_sh, "\n");

	fclose(wg_sh);

	return 0;
}
