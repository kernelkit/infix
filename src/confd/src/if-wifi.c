/* SPDX-License-Identifier: BSD-3-Clause */
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "interfaces.h"

#define WPA_SUPPLICANT_CONF      "/etc/wpa_supplicant-%s.conf"

/*
 * WiFi Interface Management
 *
 * This file handles virtual WiFi interface creation/deletion and
 * station mode configuration (wpa_supplicant). Access point mode
 * configuration (hostapd) is handled by hardware.c.
 */

/*
 * Determine WiFi mode from YANG configuration
 */
typedef enum wifi_mode_t {
	wifi_station,
	wifi_ap,
	wifi_unknown
} wifi_mode_t;

static wifi_mode_t wifi_get_mode(struct lyd_node *wifi)
{
	struct lyd_node *ap = lydx_get_child(wifi, "access-point");

	if (ap && (lydx_get_op(ap) != LYDX_OP_DELETE))
		return wifi_ap;
	else
		return wifi_station; /* Need to return station even if "station" also is false, since that is the default scanning mode */
}

int wifi_mode_changed(struct lyd_node *wifi)
{
	struct lyd_node *ap;
	enum lydx_op ap_op;

	if (!wifi)
		return 0;

	ap = lydx_get_child(wifi, "access-point");

	if (ap)
		ap_op = lydx_get_op(ap);

	ERROR("MODE CHANGED: %d", ap && (ap_op == LYDX_OP_CREATE || ap_op == LYDX_OP_DELETE));
	return (ap && (ap_op == LYDX_OP_CREATE || ap_op == LYDX_OP_DELETE));
}

/*
 * Generate wpa_supplicant config for station mode
 */
static int wifi_gen_station(struct lyd_node *cif)
{
	const char *ifname, *ssid, *secret_name, *secret, *security_mode, *radio;
	struct lyd_node *security, *secret_node, *radio_node, *station, *wifi;
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
	/* If station is NULL, we're in scan-only mode (no station container) */
	if (station) {
		ssid = lydx_get_cattr(station, "ssid");
		security = lydx_get_child(station, "security");
		security_mode = lydx_get_cattr(security, "mode");
		secret_name = lydx_get_cattr(security, "secret");
	} else {
		ssid = NULL;
		security = NULL;
		security_mode = "disabled";
		secret_name = NULL;
	}

	radio_node = lydx_get_xpathf(cif,
		"../../hardware/component[name='%s']/wifi-radio", radio);
	country = lydx_get_cattr(radio_node, "country-code");

	if (secret_name && strcmp(security_mode, "disabled") != 0) {
		secret_node = lydx_get_xpathf(cif,
			"../../keystore/symmetric-keys/symmetric-key[name='%s']",
			secret_name);
		secret = lydx_get_cattr(secret_node, "symmetric-key");
	} else {
		secret = NULL;
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
		if (!strcmp(security_mode, "disabled")) {
			asprintf(&security_str, "key_mgmt=NONE");
		} else if (secret) {
			asprintf(&security_str, "key_mgmt=SAE WPA-PSK\npsk=\"%s\"", secret);
		}
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
		ERROR("Failed to open dagger file for WiFi interface creation");
		return SR_ERR_INTERNAL;
	}

	mode = wifi_get_mode(wifi);
	probe_timeout = wifi_get_probe_timeout(net->session, radio);

	fprintf(iw, "# Generated by Infix confd - WiFi Interface Creation\n");
	fprintf(iw, "# Create %s interface %s on radio %s\n",
		mode == wifi_station ? "station" : "access point", ifname, radio);

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
		ERROR("Failed to open dagger file for WiFi interface deletion");
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
