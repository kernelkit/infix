/* SPDX-License-Identifier: BSD-3-Clause */
#include <unistd.h>
#include <openssl/evp.h>

#include <srx/common.h>
#include <srx/lyx.h>

#include "core.h"
#include "interfaces.h"
#include "base64.h"
#include "hardware.h"

#define WIFI_RXKH_DIR            "/run/wifi-neighbors"
#define WIFI_RXKH_FILE           WIFI_RXKH_DIR "/%s.rxkh"

/* Resolve mobility domain - if "hash", derive from SSID using MD5 (OpenWrt-compatible) */
static const char *resolve_mobility_domain(const char *mobility_domain, const char *ssid)
{
	static char hash_result[5]; /* 4 hex chars + null */
	unsigned char md5_digest[EVP_MAX_MD_SIZE];
	unsigned int md5_len;
	EVP_MD_CTX *ctx;

	if (strcmp(mobility_domain, "hash"))
		return mobility_domain;

	if (!ssid) {
		ERROR("Cannot derive mobility domain from NULL SSID");
		return "4f57"; /* Fallback to default */
	}

	/* Compute MD5 hash using EVP API (OpenSSL 3.0+) */
	ctx = EVP_MD_CTX_new();
	if (!ctx) {
		ERROR("Failed to create EVP context");
		return "4f57";
	}

	if (EVP_DigestInit_ex(ctx, EVP_md5(), NULL) != 1 ||
	    EVP_DigestUpdate(ctx, ssid, strlen(ssid)) != 1 ||
	    EVP_DigestFinal_ex(ctx, md5_digest, &md5_len) != 1) {
		ERROR("Failed to compute MD5 hash");
		EVP_MD_CTX_free(ctx);
		return "4f57";
	}

	EVP_MD_CTX_free(ctx);

	/* Extract first 4 hex characters (first 2 bytes) */
	snprintf(hash_result, sizeof(hash_result), "%02x%02x", md5_digest[0], md5_digest[1]);

	DEBUG("Derived mobility domain '%s' from SSID '%s'", hash_result, ssid);
	return hash_result;
}

/*
 * Cleartext of a keystore symmetric key, NULL when unset.  Caller frees.
 */
static unsigned char *wifi_keystore_secret(struct lyd_node *config, const char *name)
{
	struct lyd_node *node;
	const char *b64;

	if (!name)
		return NULL;

	node = lydx_get_xpathf(config, "/keystore/symmetric-keys/symmetric-key[name='%s']/cleartext-symmetric-key", name);
	if (!node)
		return NULL;

	b64 = lyd_get_value(node);
	if (!b64)
		return NULL;

	return base64_decode((const unsigned char *)b64, strlen(b64), NULL);
}

/*
 * Key for the 802.11r key holder exchange between access points, hex
 * encoded SHA-256 over the mobility domain and the key holder secret.
 * Every AP of the mobility domain derives the same key, so the wildcard
 * R0KH/R1KH entries let any of them fetch a roaming client's PMK-R1 from
 * the AP it came from, which is what fast transition needs for WPA3
 * (SAE) clients: their PMK comes from the SAE handshake and cannot be
 * regenerated locally the way a WPA2 PSK can.
 *
 * The secret is the dot11r key-holder-secret when one is set, else the
 * WiFi password.  A holder of the key can fetch any client's keys and is
 * pushed them, and the key tags the announcements wifi-neighbors.py
 * sends on the APs' network, so with the password as the secret every
 * client can read the others' keys and anyone on the network has an
 * offline oracle for the password.  Hence the opt-in key of its own.
 */
static int wifi_ft_key(const char *mobility_domain, const unsigned char *secret, char *out, size_t len)
{
	unsigned char digest[EVP_MAX_MD_SIZE];
	unsigned int dlen = 0;
	EVP_MD_CTX *ctx;
	size_t i;

	if (len < 2 * 32 + 1)
		return -1;

	ctx = EVP_MD_CTX_new();
	if (!ctx)
		return -1;

	if (EVP_DigestInit_ex(ctx, EVP_sha256(), NULL) != 1 ||
	    EVP_DigestUpdate(ctx, "infix-ft:", 9) != 1 ||
	    EVP_DigestUpdate(ctx, mobility_domain, strlen(mobility_domain)) != 1 ||
	    EVP_DigestUpdate(ctx, ":", 1) != 1 ||
	    EVP_DigestUpdate(ctx, secret, strlen((const char *)secret)) != 1 ||
	    EVP_DigestFinal_ex(ctx, digest, &dlen) != 1 || dlen < 32) {
		EVP_MD_CTX_free(ctx);
		return -1;
	}
	EVP_MD_CTX_free(ctx);

	for (i = 0; i < 32; i++)
		snprintf(out + 2 * i, 3, "%02x", digest[i]);

	return 0;
}

/*
 * Find an AP interface on a higher-band radio (5/6 GHz) advertising the
 * same SSID as the caller's 2.4 GHz BSS, for no_probe_resp_if_seen_on=.
 * hostapd suppresses a 2.4 GHz probe response only once it has actually
 * seen that MAC on the listed interface, so 2.4-only clients always get a
 * response while dual-band clients are nudged to the higher band. Note:
 * sta_track spans bands only when both radios run in one hostapd process.
 *
 * Returns a pointer into the config tree (do not free) or NULL.
 */
static const char *wifi_find_higher_band_twin(struct lyd_node *config,
					      const char *current_band,
					      const char *current_ssid)
{
	struct lyd_node *cifs, *cif;

	if (!current_band || !current_ssid || strcmp(current_band, "2.4GHz"))
		return NULL;

	cifs = lydx_get_descendant(config, "interfaces", "interface", NULL);
	LYX_LIST_FOR_EACH(cifs, cif, "interface") {
		struct lyd_node *wifi, *ap, *radio_node;
		const char *radio, *ssid, *band;

		wifi = lydx_get_child(cif, "wifi");
		if (!wifi)
			continue;
		ap = lydx_get_child(wifi, "access-point");
		if (!ap || !lydx_is_enabled(cif, "enabled"))
			continue;
		ssid = lydx_get_cattr(ap, "ssid");
		if (!ssid || strcmp(ssid, current_ssid))
			continue;
		radio = lydx_get_cattr(wifi, "radio");
		radio_node = lydx_get_xpathf(config,
			"/hardware/component[name='%s']/wifi-radio", radio);
		band = lydx_get_cattr(radio_node, "band");
		if (band && (!strcmp(band, "5GHz") || !strcmp(band, "6GHz")))
			return lydx_get_cattr(cif, "name");
	}

	return NULL;
}

/*
 * The 802.11r key holder list of a BSS, read by hostapd at start and on
 * RELOAD_RXKHS.  wifi-neighbors.py rewrites it with the other nodes'
 * access points as R1KHs, keeping these wildcard entries.
 */
static int wifi_write_rxkh(const char *ifname, const char *ft_key)
{
	FILE *fp;

	if (fmkpath(0755, WIFI_RXKH_DIR))
		return -1;

	fp = fopenf("w", WIFI_RXKH_FILE, ifname);
	if (!fp)
		return -1;

	fprintf(fp, "r0kh=ff:ff:ff:ff:ff:ff * %s\n", ft_key);
	fprintf(fp, "r1kh=00:00:00:00:00:00 00:00:00:00:00:00 %s\n", ft_key);

	return fclose(fp);
}

/* Emit the 4-address WDS ports (wds-link interfaces) of an AP */
static void wifi_gen_wds_ports(FILE *hostapd, struct lyd_node *config, const char *ap_ifname)
{
	struct lyd_node *cifs, *cif;
	bool first = true;

	cifs = lydx_get_descendant(config, "interfaces", "interface", NULL);
	LYX_LIST_FOR_EACH(cifs, cif, "interface") {
		struct lyd_node *wds;
		const char *ap;

		wds = lydx_get_descendant(lyd_child(cif), "wifi", "wds-link", NULL);
		if (!wds)
			continue;

		ap = lydx_get_cattr(wds, "access-point");
		if (!ap || strcmp(ap, ap_ifname))
			continue;

		if (first) {
			fprintf(hostapd, "# 4-address WDS ports\n");
			fprintf(hostapd, "wds_sta=1\n");
			/* A deauth from a vanishing peer is easily lost, and the
			 * default 300 s leaves a dead backhaul port up that long. */
			fprintf(hostapd, "ap_max_inactivity=30\n");
			first = false;
		}
		fprintf(hostapd, "wds_sta_ifname=%s %s\n",
			lydx_get_cattr(wds, "peer-address"), lydx_get_cattr(cif, "name"));
	}
}

/* At boot gethostname() is still the default, the hostname is set later */
static void wifi_nas_hostname(struct confd *confd, struct lyd_node *config, char *hostname, size_t len)
{
	struct lyd_node *node;

	node = lydx_get_xpathf(config, "/ietf-system:system/hostname");
	if (node && !hostnamefmt(confd, lyd_get_value(node), hostname, len, NULL, 0) && hostname[0])
		return;

	if (gethostname(hostname, len) != 0)
		strlcpy(hostname, "localhost", len);
	hostname[len - 1] = '\0';
}

/* Helper: Write SSID and security configuration (shared between primary and BSS) */
void wifi_gen_ssid_config(FILE *hostapd, struct lyd_node *cif, struct lyd_node *config, bool is_bss,
			  const char *band, struct confd *confd)
{
	struct lyd_node *wifi, *ap, *security, *roaming;
	struct lyd_node *dot11k, *dot11r, *dot11v;
	const char *ssid, *hidden, *security_mode;
	const char *mobility_domain, *mobility_domain_raw;
	const char *nas_identifier_cfg;
	bool enable_80211k, enable_80211r, enable_80211v, enable_mbo;
	unsigned char *secret = NULL;
	const char *ifname;
	char nas_identifier_default[300];
	char hostname[64] = "localhost";
	char bssid[18];

	ifname = lydx_get_cattr(cif, "name");
	wifi = lydx_get_child(cif, "wifi");
	ap = lydx_get_child(wifi, "access-point");

	if (is_bss) {
		fprintf(hostapd, "\n# BSS %s\n", ifname);
		fprintf(hostapd, "bss=%s\n", ifname);
		fprintf(hostapd, "ctrl_interface=/run/hostapd\n");
	}

	/* Check 802.11k/r/v configuration */
	roaming = lydx_get_child(ap, "roaming");
	dot11k = roaming ? lydx_get_child(roaming, "dot11k") : NULL;
	dot11r = roaming ? lydx_get_child(roaming, "dot11r") : NULL;
	dot11v = roaming ? lydx_get_child(roaming, "dot11v") : NULL;
	enable_80211k = dot11k != NULL;
	enable_80211r = dot11r != NULL;
	enable_80211v = dot11v != NULL;
	if (dot11v) {
		const char *band_steering = lydx_get_cattr(dot11v, "band-steering");

		enable_mbo = !band_steering || !strcmp(band_steering, "true");
	} else {
		enable_mbo = false;
	}



	/* Set BSSID if custom MAC is configured */
	if (!interface_get_phys_addr(cif, bssid))
		fprintf(hostapd, "bssid=%s\n", bssid);

	/* SSID configuration */
	ssid = lydx_get_cattr(ap, "ssid");
	hidden = lydx_get_cattr(ap, "hidden");

	if (enable_80211r) {
		mobility_domain_raw = lydx_get_cattr(dot11r, "mobility-domain");
		mobility_domain = resolve_mobility_domain(mobility_domain_raw, ssid);
		nas_identifier_cfg = lydx_get_cattr(dot11r, "nas-identifier");
		if (!nas_identifier_cfg || !strcmp(nas_identifier_cfg, "auto")) {
			wifi_nas_hostname(confd, config, hostname, sizeof(hostname));
			snprintf(nas_identifier_default, sizeof(nas_identifier_default),
				 "%s-%s.%s", ifname, hostname, mobility_domain);
			nas_identifier_cfg = nas_identifier_default;
		}
	}
	if (ssid)
		fprintf(hostapd, "ssid=%s\n", ssid);
	if (hidden && !strcmp(hidden, "true"))
		fprintf(hostapd, "ignore_broadcast_ssid=1\n");

	/*
	 * Sane defaults for AP operation
	 */

	/* WMM (Wi-Fi Multimedia) required for 802.11n/ac/ax and proper QoS */
	fprintf(hostapd, "wmm_enabled=1\n");

	/* Allow UTF-8 characters in SSID for international network names */
	fprintf(hostapd, "utf8_ssid=1\n");

	/* Poll inactive clients before disconnecting to detect sleepy devices */
	fprintf(hostapd, "skip_inactivity_poll=0\n");

	security = lydx_get_child(ap, "security");
	security_mode = lydx_get_cattr(security, "mode");

	if (!security_mode)
		security_mode = "open";

	/* Get secret from keystore if needed */
	if (strcmp(security_mode, "open") != 0)
		secret = wifi_keystore_secret(config, lydx_get_cattr(security, "secret"));

	/* Resolve "auto" to concrete mode based on band */
	if (!strcmp(security_mode, "auto"))
		security_mode = (band && !strcmp(band, "6GHz")) ? "wpa3-personal" : "wpa2-wpa3-personal";

	if (!strcmp(security_mode, "open")) {
		/* auth_algs=1: Open System authentication (unencrypted) */
		fprintf(hostapd, "auth_algs=1\n");
	} else if (!strcmp(security_mode, "wpa2-personal")) {
		fprintf(hostapd, "wpa=2\n");
		/* WPA-PSK: Pre-shared key authentication */
		fprintf(hostapd, "wpa_key_mgmt=%sWPA-PSK\n", enable_80211r ? "FT-PSK " : "");
		/* CCMP: AES-based encryption, mandatory for WPA2 */
		fprintf(hostapd, "wpa_pairwise=CCMP\n");
		if (secret)
			fprintf(hostapd, "wpa_passphrase=%s\n", secret);
	} else if (!strcmp(security_mode, "wpa3-personal")) {
		fprintf(hostapd, "wpa=2\n");
		/* SAE: Simultaneous Authentication of Equals, resistant to offline dictionary attacks */
		fprintf(hostapd, "wpa_key_mgmt=%sSAE\n", enable_80211r ? "FT-SAE " : "");
		fprintf(hostapd, "rsn_pairwise=CCMP\n");
		if (secret)
			fprintf(hostapd, "sae_password=%s\n", secret);
		fprintf(hostapd, "ieee80211w=2\n");
	} else if (!strcmp(security_mode, "wpa2-wpa3-personal")) {
		fprintf(hostapd, "wpa=2\n");
		/* Allow both PSK (WPA2) and SAE (WPA3) authentication */
		fprintf(hostapd, "wpa_key_mgmt=%sWPA-PSK SAE\n",
			enable_80211r ? "FT-PSK FT-SAE " : "");
		fprintf(hostapd, "rsn_pairwise=CCMP\n");
		if (secret) {
			fprintf(hostapd, "wpa_passphrase=%s\n", secret);
			fprintf(hostapd, "sae_password=%s\n", secret);
		}
		/* ieee80211w=1: MFP capable but optional, for WPA2 client compatibility */
		fprintf(hostapd, "ieee80211w=1\n");
	}

	/* 802.11r: Fast BSS Transition */
	if (enable_80211r) {
		const char *bridge = lydx_get_cattr(lydx_get_child(cif, "bridge-port"), "bridge");
		unsigned char *kh_secret;
		const char *kh_name;
		char ft_key[65];

		fprintf(hostapd, "# Fast BSS Transition (802.11r)\n");
		fprintf(hostapd, "mobility_domain=%s\n", mobility_domain);
		/* Over-the-air FT: the client authenticates directly with the
		 * target AP. Over-DS would relay the exchange through the wireless
		 * mesh backhaul, adding latency and packet loss, so it is not used. */
		fprintf(hostapd, "ft_over_ds=0\n");
		fprintf(hostapd, "ft_psk_generate_local=1\n");
		fprintf(hostapd, "nas_identifier=%s\n", nas_identifier_cfg);
		/* The key holders of all APs on the SSID reach each other over
		 * the network the APs are bridged to, see wifi_ft_key().  On a
		 * VLAN filtering bridge that is the APs' VLAN: use its VLAN
		 * interface when there is one, the bridge device itself has
		 * no say in which VLAN its frames end up in. */
		kh_name = lydx_get_cattr(dot11r, "key-holder-secret");
		if (!kh_name)
			kh_name = lydx_get_cattr(security, "secret");
		kh_secret = wifi_keystore_secret(config, kh_name);
		if (!kh_secret)
			ERROR("%s: 802.11r key holder secret not found in keystore, no key exchange", ifname);
		if (kh_secret && !wifi_ft_key(mobility_domain, kh_secret, ft_key, sizeof(ft_key))) {
			if (bridge) {
				const char *pvid = lydx_get_cattr(lydx_get_child(cif, "bridge-port"), "pvid");
				const char *ft_iface = bridge;

				if (pvid) {
					struct lyd_node *vif;

					vif = lydx_get_xpathf(config, "/interfaces/interface[vlan/id='%s' and vlan/lower-layer-if='%s']/name",
							      pvid, bridge);
					if (vif)
						ft_iface = lyd_get_value(vif);
				}
				fprintf(hostapd, "ft_iface=%s\n", ft_iface);
			}
			/* The key holders live in a file so that wifi-neighbors.py
			 * can add the other nodes' access points as it hears of
			 * them and have hostapd reload the list.  Start with the
			 * wildcards, which accept any node holding the key. */
			if (!wifi_write_rxkh(ifname, ft_key))
				fprintf(hostapd, "rxkh_file=" WIFI_RXKH_FILE "\n", ifname);
			/* Hand every R1KH the PMK-R1 as soon as a client
			 * authenticates: a roam must not depend on this node,
			 * the R0KH, still being up to pull the key from. */
			fprintf(hostapd, "pmk_r1_push=1\n");
			/* A client roaming away from a node that is going down
			 * asks for a key that node can no longer hand out.
			 * Give up on the fetch quickly and reject, the client
			 * then logs in the normal way; waiting in silence makes
			 * it blacklist the target instead. */
			fprintf(hostapd, "rkh_pull_timeout=300\n");
			fprintf(hostapd, "rkh_pull_retries=1\n");
		}
		free(kh_secret);
	}

	/* 802.11k: Radio Resource Management */
	if (enable_80211k) {
		fprintf(hostapd, "# Radio Resource Management (802.11k)\n");
		fprintf(hostapd, "rrm_neighbor_report=1\n");
		fprintf(hostapd, "rrm_beacon_report=1\n");
	}

	/* 802.11v: BSS Transition Management */
	if (enable_80211v) {
		fprintf(hostapd, "# BSS Transition Management (802.11v)\n");
		fprintf(hostapd, "bss_transition=1\n");
	}

	wifi_gen_wds_ports(hostapd, config, ifname);

	/* OKC: Opportunistic Key Caching */
	if (roaming) {
		const char *okc = lydx_get_cattr(roaming, "okc");

		if (!okc || !strcmp(okc, "true"))
			fprintf(hostapd, "okc=1\n");
	}

	/* MBO: Multi-Band Operation. Advertises multi-band capability so
	 * MBO-aware clients make their own band-steering decisions; pairs
	 * with 802.11v BSS Transition Management above. */
	if (enable_mbo) {
		const char *twin;

		fprintf(hostapd, "mbo=1\n");

		/* Band steering on a 2.4 GHz BSS with a same-SSID twin on 5/6
		 * GHz: a client the twin has seen lately gets no probe response
		 * here, so it tends to join the twin instead.  The seen-on list
		 * is kept per radio, see the radio section.  Clients already
		 * connected are moved by a wifi-steer.sh for every pair with
		 * this directive.  Refusing authentication as well would
		 * lock a client out when the twin cannot take it, e.g. during
		 * its radar check, so that is deliberately not done. */
		twin = wifi_find_higher_band_twin(config, band, ssid);
		if (twin)
			fprintf(hostapd, "no_probe_resp_if_seen_on=%s\n", twin);
	}

	free(secret);
}
