/* SPDX-License-Identifier: BSD-3-Clause */
/*
 * WiFi Interface Management
 *
 * This file handles virtual WiFi interface creation/deletion and
 * station mode configuration (wpa_supplicant). Access point mode
 * configuration (hostapd) is handled by hardware.c.
 */

#include <ctype.h>

#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "interfaces.h"
#include "base64.h"

#define WPA_SUPPLICANT_CONF      "/etc/wpa_supplicant-%s.conf"


int wifi_validate_secret(sr_session_ctx_t *session, struct lyd_node *cif)
{
	struct lyd_node *wifi, *station, *security, *secret_node;
	const char *ifname, *secret_name, *security_mode, *b64;
	unsigned char *decoded;
	size_t len;

	ifname = lydx_get_cattr(cif, "name");
	wifi = lydx_get_child(cif, "wifi");
	if (!wifi)
		return SR_ERR_OK;

	station = lydx_get_child(wifi, "station");
	if (!station)
		return SR_ERR_OK;

	security = lydx_get_child(station, "security");
	security_mode = lydx_get_cattr(security, "mode");
	secret_name = lydx_get_cattr(security, "secret");

	if (!secret_name || !strcmp(security_mode, "disabled"))
		return SR_ERR_OK;

	secret_node = lydx_get_xpathf(cif,
		"../../keystore/symmetric-keys/symmetric-key[name='%s']",
		secret_name);
	b64 = lydx_get_cattr(secret_node, "cleartext-symmetric-key");
	if (!b64 || !*b64)
		return SR_ERR_OK;

	decoded = base64_decode((const unsigned char *)b64, strlen(b64), &len);
	if (!decoded)
		return SR_ERR_OK;

	if (len < 8 || len > 63) {
		if (session)
			sr_session_set_error_message(session,
				"%s: WiFi passphrase must be 8-63 characters, got %zu",
				ifname, len);
		free(decoded);
		return SR_ERR_VALIDATION_FAILED;
	}

	for (size_t i = 0; i < len; i++) {
		if (!isprint((unsigned char)decoded[i])) {
			if (session)
				sr_session_set_error_message(session,
					"%s: WiFi passphrase contains non-printable "
					"character at position %zu",
					ifname, i + 1);
			free(decoded);
			return SR_ERR_VALIDATION_FAILED;
		}
	}

	free(decoded);
	return SR_ERR_OK;
}

wifi_mode_t wifi_get_mode(struct lyd_node *iface)
{
	struct lyd_node *ap, *mesh, *wifi;

	wifi = lydx_get_child(iface, "wifi");
	if (!wifi)
		return wifi_unknown;

	ap = lydx_get_child(wifi, "access-point");
	if (ap) {
		if (lydx_get_op(ap) != LYDX_OP_DELETE)
			return wifi_ap;
	}

	mesh = lydx_get_child(wifi, "mesh-point");
	if (mesh) {
		if (lydx_get_op(mesh) != LYDX_OP_DELETE)
			return wifi_mesh;
	}

	/*
	 * Need to return station even if "station" also is false,
	 * because station is the default scanning mode.
	 */
	return wifi_station;
}

int wifi_mode_changed(struct lyd_node *wifi)
{
	enum lydx_op op = LYDX_OP_DELETE;
	struct lyd_node *node;

	if (!wifi)
		return 0;

	node = lydx_get_child(wifi, "access-point");
	if (node)
		op = lydx_get_op(node);
	if (node && (op == LYDX_OP_CREATE || op == LYDX_OP_DELETE))
		return 1;

	node = lydx_get_child(wifi, "mesh-point");
	if (node)
		op = lydx_get_op(node);
	if (node && (op == LYDX_OP_CREATE || op == LYDX_OP_DELETE))
		return 1;

	return 0;
}

/*
 * Generate wpa_supplicant config for station mode
 */
int wifi_gen_station(struct lyd_node *cif)
{
	const char *ifname, *ssid, *secret_name, *security_mode, *radio;
	struct lyd_node *security, *secret_node, *radio_node, *station, *wifi;
	unsigned char *secret = NULL;
	FILE *wpa_supplicant = NULL;
	char *security_str = NULL;
	const char *country;
	int rc = SR_ERR_OK;
	mode_t oldmask;

	ifname = lydx_get_cattr(cif, "name");
	wifi = lydx_get_child(cif, "wifi");
	if (!wifi)
		return SR_ERR_OK;

	radio = lydx_get_cattr(wifi, "radio");
	station = lydx_get_child(wifi, "station");
	if (station) {
		ssid = lydx_get_cattr(station, "ssid");
		security = lydx_get_child(station, "security");
		security_mode = lydx_get_cattr(security, "mode");
		secret_name = lydx_get_cattr(security, "secret");
	} else {
		/* If station is NULL, we're in scan-only mode (no station container) */
		ssid = NULL;
		security = NULL;
		security_mode = "disabled";
		secret_name = NULL;
	}

	radio_node = lydx_get_xpathf(cif, "../../hardware/component[name='%s']/wifi-radio", radio);
	country = lydx_get_cattr(radio_node, "country-code");

	if (secret_name && strcmp(security_mode, "disabled") != 0) {
		const char *b64;

		secret_node = lydx_get_xpathf(cif,
			"../../keystore/symmetric-keys/symmetric-key[name='%s']",
			secret_name);
		b64 = lydx_get_cattr(secret_node, "cleartext-symmetric-key");
		if (b64)
			secret = base64_decode((const unsigned char *)b64, strlen(b64), NULL);
	}

	oldmask = umask(0077);
	wpa_supplicant = fopenf("w", WPA_SUPPLICANT_CONF, ifname);
	if (!wpa_supplicant) {
		rc = SR_ERR_INTERNAL;
		goto out;
	}

	/*
	 * Background scanning every 10 seconds while not associated, when we
	 * have an SSID (below), bgscan assumes this task.
	 */
	fprintf(wpa_supplicant,
		"ctrl_interface=/run/wpa_supplicant\n"
		"autoscan=periodic:10\n"
		"ap_scan=1\n");

	if (country)
		fprintf(wpa_supplicant, "country=%s\n", country);

	/* If SSID is present, create network block. Otherwise, scan-only mode */
	if (ssid) {
		/* Station mode with network configured */
		if (!strcmp(security_mode, "disabled"))
			asprintf(&security_str, "key_mgmt=NONE");
		else if (secret)
			asprintf(&security_str,
				 "key_mgmt=FT-SAE FT-PSK SAE WPA-PSK\n"
				 "  psk=\"%s\"", secret);

		fprintf(wpa_supplicant,
			"network={\n"
			"  bgscan=\"simple: 30:-45:300\"\n"
			"  ssid=\"%s\"\n"
			"  %s\n"
			"}\n", ssid, security_str);
		free(security_str);
	} else {
		/* Scan-only mode - no station container configured */
		fprintf(wpa_supplicant, "# Scan-only - need dummy network for state machine\n");
		/*
		 * This prevents the daemon from actually trying to associate and fail,
		 * 'bssid' with a reserved/impossible MAC address prevents it from ever
		 * actually \"finding\" and joining a random open AP
		 */
		fprintf(wpa_supplicant,
			"network={\n"
			"  ssid=\"\"\n"
			"  key_mgmt=NONE\n"
			"  disabled=0\n"
			"  bssid=00:00:00:00:00:01\n"
			"  scan_ssid=1\n"
			"}\n");
	}

out:
	free(secret);
	if (wpa_supplicant)
		fclose(wpa_supplicant);
	umask(oldmask);

	return rc;
}



/*
 * Center channel for 80MHz VHT/HE operation.
 * 5GHz 80MHz channel groups and their center channels:
 *   36-48(42), 52-64(58), 100-112(106),
 *                    116-128(122), 132-144(138), 149-161(155)
 */
static int wifi_center_chan_80(int ch)
{
	static const int grp[][2] = {
		{36, 42}, {52, 58}, {100, 106}, {116, 122}, {132, 138}, {149, 155}
	};
	int i;

	for (i = 0; i < 6; i++)
		if (ch >= grp[i][0] && ch < grp[i][0] + 16)
			return grp[i][1];
	return 0;
}

/*
 * Center channel for 160MHz VHT/HE operation.
 * 5GHz 160MHz groups: 36-64(50), 100-128(114)
 */
static int wifi_center_chan_160(int ch)
{
	if (ch >= 36 && ch <= 64)
		return 50;
	if (ch >= 100 && ch <= 128)
		return 114;
	return 0;
}

/* HT40 secondary channel direction for mesh: returns "+" or "-" */
static const char *wifi_mesh_ht40_dir(int ch)
{
	return ((ch / 4) % 2) ? "+" : "-";
}
/*
 * Convert WiFi channel number to frequency in MHz.
 * Band is determined from channel range:
 *   2.4GHz: channels 1-14
 *   5GHz:   channels 32-177
 *   6GHz:   channels 1-233 (identified by band string)
 */
static int wifi_chan_to_freq(int channel, const char *band)
{
	if (!strcmp(band, "6GHz"))
		return 5950 + channel * 5;

	if (channel >= 1 && channel <= 13)
		return 2407 + channel * 5;
	if (channel == 14)
		return 2484;

	/* 5GHz */
	return 5000 + channel * 5;
}

/*
 * Generate wpa_supplicant config for 802.11s mesh mode
 */
int wifi_gen_mesh(struct lyd_node *cif)
{
	const char *ifname, *mesh_id, *secret_name, *radio;
	struct lyd_node *mesh, *security, *secret_node, *radio_node, *wifi;
	unsigned char *secret = NULL;
	FILE *wpa_supplicant = NULL;
	const char *country, *band, *width;
	int rc = SR_ERR_OK;
	int channel, freq;
	mode_t oldmask;
	bool forwarding;

	ifname = lydx_get_cattr(cif, "name");
	wifi = lydx_get_child(cif, "wifi");
	if (!wifi)
		return SR_ERR_OK;

	radio = lydx_get_cattr(wifi, "radio");
	mesh = lydx_get_child(wifi, "mesh-point");
	if (!mesh)
		return SR_ERR_OK;

	mesh_id = lydx_get_cattr(mesh, "mesh-id");
	forwarding = lydx_is_enabled(mesh, "forwarding");

	security = lydx_get_child(mesh, "security");
	secret_name = lydx_get_cattr(security, "secret");

	radio_node = lydx_get_xpathf(cif, "../../hardware/component[name='%s']/wifi-radio", radio);
	country = lydx_get_cattr(radio_node, "country-code");
	band = lydx_get_cattr(radio_node, "band");
	width = lydx_get_cattr(radio_node, "channel-width");
	channel = atoi(lydx_get_cattr(radio_node, "channel") ? : "0");

	if (!band || !channel) {
		ERROR("%s: mesh requires radio band and channel", ifname);
		return SR_ERR_INVAL_ARG;
	}

	freq = wifi_chan_to_freq(channel, band);

	if (secret_name) {
		const char *b64;

		secret_node = lydx_get_xpathf(cif,
			"../../keystore/symmetric-keys/symmetric-key[name='%s']",
			secret_name);
		b64 = lydx_get_cattr(secret_node, "cleartext-symmetric-key");
		if (b64)
			secret = base64_decode((const unsigned char *)b64, strlen(b64), NULL);
	}

	oldmask = umask(0077);
	wpa_supplicant = fopenf("w", WPA_SUPPLICANT_CONF, ifname);
	if (!wpa_supplicant) {
		rc = SR_ERR_INTERNAL;
		goto out;
	}

	fprintf(wpa_supplicant, "ctrl_interface=/run/wpa_supplicant\n");
	if (country)
		fprintf(wpa_supplicant, "country=%s\n", country);

	fprintf(wpa_supplicant, "\nnetwork={\n");
	fprintf(wpa_supplicant, "  mode=5\n");
	fprintf(wpa_supplicant, "  mesh_id=\"%s\"\n", mesh_id);
	fprintf(wpa_supplicant, "  frequency=%d\n", freq);

	/*
	 * Channel width configuration for mesh.
	 * wpa_supplicant uses mesh_ht_mode instead of hostapd's ht_capab/vht_oper_chwidth.
	 * For 6GHz, use HE modes; for 5GHz, use VHT modes.
	 * vht_center_freq1 takes frequency in MHz (not channel number).
	 */
	if (width && strcmp(width, "auto")) {
		if (!strcmp(width, "20MHz")) {
			fprintf(wpa_supplicant, "  mesh_ht_mode=HT20\n");
		} else if (!strcmp(width, "40MHz")) {
			fprintf(wpa_supplicant, "  mesh_ht_mode=HT40%s\n", wifi_mesh_ht40_dir(channel));
		} else if (!strcmp(width, "80MHz")) {
			int center = wifi_center_chan_80(channel);

			if (!strcmp(band, "6GHz")) {
				fprintf(wpa_supplicant, "  mesh_ht_mode=HE80\n");
				fprintf(wpa_supplicant, "  he=1\n");
			} else {
				fprintf(wpa_supplicant, "  mesh_ht_mode=VHT\n");
			}
			fprintf(wpa_supplicant, "  max_oper_chwidth=1\n");
			if (center)
				fprintf(wpa_supplicant, "  vht_center_freq1=%d\n", wifi_chan_to_freq(center, band));
		} else if (!strcmp(width, "160MHz")) {
			int center = wifi_center_chan_160(channel);

			if (!strcmp(band, "6GHz")) {
				fprintf(wpa_supplicant, "  mesh_ht_mode=HE160\n");
				fprintf(wpa_supplicant, "  he=1\n");
			} else {
				fprintf(wpa_supplicant, "  mesh_ht_mode=VHT\n");
			}
			fprintf(wpa_supplicant, "  max_oper_chwidth=2\n");
			if (center)
				fprintf(wpa_supplicant, "  vht_center_freq1=%d\n", wifi_chan_to_freq(center, band));
		}
	}
	fprintf(wpa_supplicant, "  mesh_fwding=%d\n", forwarding ? 1 : 0);

	fprintf(wpa_supplicant, "  key_mgmt=SAE\n");
	fprintf(wpa_supplicant, "  ieee80211w=2\n");
	if (secret)
		fprintf(wpa_supplicant, "  sae_password=\"%s\"\n", secret);

	fprintf(wpa_supplicant, "}\n");

out:
	free(secret);
	if (wpa_supplicant)
		fclose(wpa_supplicant);
	umask(oldmask);

	return rc;
}
/*
 * Get probe-timeout for a radio from sysrepo config.
 * Returns 0 if not set.
 */
static int wifi_get_probe_timeout(sr_session_ctx_t *session, const char *radio)
{
	char *timeout_str;
	int timeout = 0;

	timeout_str = srx_get_str(session,
		"/ietf-hardware:hardware/component[name='%s']/infix-hardware:wifi-radio/probe-timeout",
		radio);
	if (timeout_str) {
		timeout = atoi(timeout_str);
		free(timeout_str);
	}
	return timeout;
}

/*
 * Add WiFi virtual interface using iw
 */
int wifi_add_iface(struct lyd_node *cif, struct dagger *net)
{
	const char *ifname, *radio;
	struct lyd_node *wifi;
	wifi_mode_t mode;
	int probe_timeout;
	FILE *iw;
	int rc = SR_ERR_OK;

	ifname = lydx_get_cattr(cif, "name");
	wifi = lydx_get_child(cif, "wifi");

	if (!wifi) {
		ERROR("WiFi interface %s: no wifi container", ifname);
		return SR_ERR_INVAL_ARG;
	}

	radio = lydx_get_cattr(wifi, "radio");
	if (!radio) {
		ERROR("WiFi interface %s: missing radio reference", ifname);
		return SR_ERR_INVAL_ARG;
	}

	iw = dagger_fopen_net_init(net, ifname, NETDAG_INIT_PRE, "wifi-iface.sh");
	if (!iw) {
		ERRNO("Failed to open dagger file for WiFi interface creation");
		return SR_ERR_INTERNAL;
	}

	mode = wifi_get_mode(cif);
	probe_timeout = wifi_get_probe_timeout(net->session, radio);

	fprintf(iw, "# Generated by Infix confd - WiFi Interface Creation\n");
	fprintf(iw, "# Create %s interface %s on radio %s\n",
		mode == wifi_station ? "station" : (mode == wifi_mesh ? "mesh" : "access point"), ifname, radio);

	/* Wait for PHY if probe-timeout is set (slow USB dongles) */
	if (probe_timeout > 0) {
		fprintf(iw, "\n# Wait for PHY (USB dongle may be slow to initialize)\n");
		fprintf(iw, "timeout=%d\n", probe_timeout);
		fprintf(iw, "while [ $timeout -gt 0 ]; do\n");
		fprintf(iw, "    iw phy %s info >/dev/null 2>&1 && break\n", radio);
		fprintf(iw, "    sleep 1\n");
		fprintf(iw, "    timeout=$((timeout - 1))\n");
		fprintf(iw, "done\n");
	}

	/*
	 * If radio doesn't exist, create a dummy interface as placeholder.  This allows all
	 * downstream config (IP addresses, etc.) to work.  User must reboot when radio becomes
	 * available.
	 */
	fprintf(iw, "if ! iw phy %s info >/dev/null 2>&1; then\n", radio);
	fprintf(iw, "    logger -t wifi \"%s: radio %s not available, creating dummy placeholder\"\n", ifname, radio);
	fprintf(iw, "    ip link add %s type dummy\n", ifname);
	fprintf(iw, "    exit 0\n");
	fprintf(iw, "fi\n\n");

	switch(mode) {
	case wifi_station:
		fprintf(iw, "iw phy %s interface add %s type managed\n", radio, ifname);
		wifi_gen_station(cif);
		fprintf(iw, "initctl -bfq enable wifi@%s\n", ifname);
		fprintf(iw, "initctl -bfq touch wifi@%s\n", ifname);
		break;
	case wifi_ap:
		fprintf(iw, "iw phy %s interface add %s type __ap\n", radio, ifname);
		break;
	case wifi_mesh:
		fprintf(iw, "iw phy %s interface add %s type mesh\n", radio, ifname);
		wifi_gen_mesh(cif);
		fprintf(iw, "initctl -bfq enable wifi@%s\n", ifname);
		fprintf(iw, "initctl -bfq touch wifi@%s\n", ifname);
		break;
	default:
		ERROR("WiFi mode %d unknown", mode);
		rc = SR_ERR_INVAL_ARG;
		goto out;
	}
out:
	fclose(iw);
	return rc;
}

/*
 * Delete WiFi virtual interface using iw
 */
int wifi_del_iface(struct lyd_node *dif, struct dagger *net)
{
	struct lyd_node *wifi;
	const char *ifname;
	FILE *iw;

	ifname = lydx_get_cattr(dif, "name");

	iw = dagger_fopen_net_exit(net, ifname, NETDAG_EXIT_POST, "wifi-iface.sh");
	if (!iw) {
		ERRNO("Failed to open dagger file for WiFi interface deletion");
		return SR_ERR_INTERNAL;
	}

	fprintf(iw, "# Generated by Infix confd - WiFi Interface Deletion\n");
	fprintf(iw, "ip link set %s down\n", ifname);
	fprintf(iw, "iw dev %s disconnect 2>/dev/null\n", ifname);
	fprintf(iw, "iw dev %s del 2>/dev/null || ip link del %s 2>/dev/null || true\n", ifname, ifname);

	wifi = lydx_get_child(dif, "wifi");
	if (wifi && wifi_get_mode(wifi) != wifi_ap) {
		erasef(WPA_SUPPLICANT_CONF, ifname);
		fprintf(iw, "initctl -bfq disable wifi@%s\n", ifname);
	}
	fclose(iw);

	return SR_ERR_OK;
}
