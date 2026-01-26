/* SPDX-License-Identifier: BSD-3-Clause */
#include <fnmatch.h>
#include <ftw.h>
#include <jansson.h>
#include <libgen.h>
#include <limits.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "core.h"
#include "interfaces.h"
#include "dagger.h"

#define XPATH_BASE_              "/ietf-hardware:hardware"
#define HOSTAPD_CONF             "/etc/hostapd-%s.conf"
#define HOSTAPD_CONF_NEXT        HOSTAPD_CONF"+"

static int dir_cb(const char *fpath, const struct stat *sb, int typeflag, struct FTW *ftwbuf)
{
	const char *filename;

	if (typeflag == FTW_DP)
		return 0;

	filename = basename((char *)fpath);
	if (!strcmp(filename, "authorized_default") || !strcmp(filename, "authorized")) {
		if (writedf(1, "w", "%s", fpath)) {
			ERROR("Failed to authorize %s", fpath);
			return FTW_STOP;
		}
	}

	return 0;
}

static bool usb_authorize(struct json_t *root, const char *name, int enabled)
{
	json_t *port, *ports;
	int index;

	ports = json_object_get(root, "usb-ports");
	if (!ports)
		return 0;

	json_array_foreach(ports, index, port) {
		struct json_t *obj;

		obj = json_object_get(port, "name");
		if (!obj || !json_is_string(obj)) {
			ERROR("Did not find USB hardware port (name) for %s", name);
			continue;
		}

		if (!strcmp(name, json_string_value(obj))) {
			char apath[PATH_MAX];
			const char *path;

			obj = json_object_get(port, "path");
			if (!obj || !json_is_string(obj)) {
				ERROR("Did not find USB hardware port (path) for %s", name);
				continue;
			}

			path = json_string_value(obj);
			snprintf(apath, sizeof(apath), "%s/authorized_default", path);

			if (!enabled) {
				if (fexist(apath)) {
					if (writedf(0, "w", "%s", apath)) {
						ERROR("Failed to unauthorize %s", apath);
						return 1;
					}
				}
			} else {
				if (realpath(path, apath))
					nftw(apath, dir_cb, 20, FTW_DEPTH | FTW_PHYS);
			}
		}
	}

	return 0;
}

static char *component_xpath(const char *xpath)
{
	char *path, *ptr;

	if (!xpath)
		return NULL;

	path = strdup(xpath);
	if (!path)
		return NULL;

	if (!(ptr = strstr(path, "]/"))) {
		free(path);
		return NULL;
	}
	ptr[1] = 0;

	return path;
}

static int hardware_cand_infer_class(json_t *root, sr_session_ctx_t *session, const char *path)
{
	sr_val_t inferred = { .type = SR_STRING_T };
	struct json_t *usb_ports, *usb_port;
	sr_error_t err = SR_ERR_OK;
	char *name, *class;
	char *xpath;
	int index;

	xpath = component_xpath(path);
	if (!xpath)
		return SR_ERR_SYS;

	class = srx_get_str(session, "%s/class", xpath);
	if (class) {
		free(class);
		goto out_free_xpath;
	}

	name = srx_get_str(session, "%s/name", xpath);
	if (!name) {
		err = SR_ERR_INTERNAL;
		goto out_free_xpath;
	}
	usb_ports = json_object_get(root, "usb-ports");
	if (!usb_ports)
		goto out_free_name; /* No USB-ports is OK */

	json_array_foreach(usb_ports, index, usb_port) {
		struct json_t *n = json_object_get(usb_port, "name");
		if (!n || !json_is_string(n)) {
			ERROR("Did not find hardware port for %s", name);
			continue;
		}
		if (!strcmp(name, json_string_value(n))) {
			inferred.data.string_val = "infix-hardware:usb";
			err = srx_set_item(session, &inferred, 0,
					   "%s/class", xpath);
			goto out_free_name;
		}
	}

	if (!fnmatch("radio+([0-9])", name, FNM_EXTMATCH)) {
		inferred.data.string_val = "infix-hardware:wifi";
		err = srx_set_item(session, &inferred, 0, "%s/class", xpath);
	}
out_free_name:
	free(name);
out_free_xpath:
	free(xpath);
	return err;
}


static int wifi_find_interfaces_on_radio(struct lyd_node *ifs, const char *radio_name,
					  struct lyd_node ***iface_list, int *count)
{
	struct lyd_node *iface, *wifi;
	const char *radio;
	struct lyd_node **list = NULL;
	int n = 0;

	if (!ifs)
		return 0;

	LYX_LIST_FOR_EACH(ifs, iface, "interface") {
		wifi = lydx_get_child(iface, "wifi");
		if (!wifi)
			continue;

		radio = lydx_get_cattr(wifi, "radio");
		if (!radio || strcmp(radio, radio_name))
			continue;

		if (lydx_get_op(iface) == LYDX_OP_DELETE)
			continue;
		list = realloc(list, sizeof(struct lyd_node *) * n + 1);
		list[n++] = iface;
	}

	*iface_list = list;
	*count = n;
	return 0;
}

/* Helper: Find all AP interfaces on a specific radio */
static int wifi_find_radio_aps(struct lyd_node *cifs, const char *radio_name,
				char ***ap_list, int *count)
{
	struct lyd_node *cif, *wifi, *ap;
	const char *ifname, *radio;
	char **list = NULL;
	int n = 0;

	LYX_LIST_FOR_EACH(cifs, cif, "interface") {
		wifi = lydx_get_child(cif, "wifi");
		if (!wifi)
			continue;

		radio = lydx_get_cattr(wifi, "radio");
		if (!radio || strcmp(radio, radio_name))
			continue;

		ap = lydx_get_child(wifi, "access-point");
		if (!ap)
			continue;
		list = realloc(list, sizeof(char *) *n+1);


		ifname = lydx_get_cattr(cif, "name");
		list[n++] = strdup(ifname);
	}

	/* Sort alphabetically for consistent primary selection */
	for (int i = 0; i < n - 1; i++) {
		for (int j = i + 1; j < n; j++) {
			if (strcmp(list[i], list[j]) > 0) {
				char *tmp = list[i];
				list[i] = list[j];
				list[j] = tmp;
			}
		}
	}

	*ap_list = list;
	*count = n;
	return 0;
}

/* Helper: Write SSID and security configuration (shared between primary and BSS) */
static void wifi_gen_ssid_config(FILE *hostapd, struct lyd_node *cif, struct lyd_node *config, bool is_bss)
{
	const char *ssid, *hidden, *security_mode, *secret_name, *secret;
	struct lyd_node *wifi, *ap, *security, *secret_node;
	const char *ifname;
	char bssid[18];

	ifname = lydx_get_cattr(cif, "name");
	wifi = lydx_get_child(cif, "wifi");
	ap = lydx_get_child(wifi, "access-point");

	if (is_bss) {
		fprintf(hostapd, "\n# BSS %s\n", ifname);
		fprintf(hostapd, "bss=%s\n", ifname);
	}

	/* Set BSSID if custom MAC is configured */
	if (!interface_get_phys_addr(cif, bssid))
		fprintf(hostapd, "bssid=%s\n", bssid);

	/* SSID configuration */
	ssid = lydx_get_cattr(ap, "ssid");
	hidden = lydx_get_cattr(ap, "hidden");

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
	secret = NULL;
	if (strcmp(security_mode, "open") != 0) {
		secret_name = lydx_get_cattr(security, "secret");
		if (secret_name) {
			secret_node = lydx_get_xpathf(config,
				"/keystore/symmetric-keys/symmetric-key[name='%s']/symmetric-key",
				secret_name);
			if (secret_node)
				secret = lyd_get_value(secret_node);
		}
	}

	if (!strcmp(security_mode, "open")) {
		/* auth_algs=1: Open System authentication (unencrypted) */
		fprintf(hostapd, "auth_algs=1\n");
	} else if (!strcmp(security_mode, "wpa2-personal")) {
		/* wpa=2: WPA2 only (RSN), no legacy WPA1 */
		fprintf(hostapd, "wpa=2\n");
		/* WPA-PSK: Pre-shared key authentication */
		fprintf(hostapd, "wpa_key_mgmt=WPA-PSK\n");
		/* CCMP: AES-based encryption, mandatory for WPA2 */
		fprintf(hostapd, "wpa_pairwise=CCMP\n");
		if (secret)
			fprintf(hostapd, "wpa_passphrase=%s\n", secret);
	} else if (!strcmp(security_mode, "wpa3-personal")) {
		/* wpa=2: Uses RSN (WPA2) frame format for WPA3 */
		fprintf(hostapd, "wpa=2\n");
		/* SAE: Simultaneous Authentication of Equals, resistant to offline dictionary attacks */
		fprintf(hostapd, "wpa_key_mgmt=SAE\n");
		fprintf(hostapd, "rsn_pairwise=CCMP\n");
		if (secret)
			fprintf(hostapd, "sae_password=%s\n", secret);
		/* ieee80211w=2: Management Frame Protection required for WPA3 */
		fprintf(hostapd, "ieee80211w=2\n");
	} else if (!strcmp(security_mode, "wpa2-wpa3-personal")) {
		/* Transition mode: supports both WPA2 and WPA3 clients */
		fprintf(hostapd, "wpa=2\n");
		/* Allow both PSK (WPA2) and SAE (WPA3) authentication */
		fprintf(hostapd, "wpa_key_mgmt=WPA-PSK SAE\n");
		fprintf(hostapd, "rsn_pairwise=CCMP\n");
		if (secret) {
			fprintf(hostapd, "wpa_passphrase=%s\n", secret);
			fprintf(hostapd, "sae_password=%s\n", secret);
		}
		/* ieee80211w=1: MFP capable but optional, for WPA2 client compatibility */
		fprintf(hostapd, "ieee80211w=1\n");
	}
}

/* Helper: Write radio-specific configuration */
static void wifi_gen_radio_config(FILE *hostapd, struct lyd_node *radio_node)
{
	const char *country, *channel, *band;

	country = lydx_get_cattr(radio_node, "country-code");
	band = lydx_get_cattr(radio_node, "band");
	channel = lydx_get_cattr(radio_node, "channel");

	if (country)
		fprintf(hostapd, "country_code=%s\n", country);

	/*
	 * 802.11d broadcasts country info in beacons, required for clients
	 * to know which channels/power levels are legal in this region.
	 */
	fprintf(hostapd, "ieee80211d=1\n");

	/*
	 * 802.11h enables DFS (radar detection) and TPC (power control),
	 * required for 5GHz operation in most regulatory domains.
	 */
	fprintf(hostapd, "ieee80211h=1\n");

	fprintf(hostapd, "beacon_int=100\n");

	/* Use short preamble for better throughput on modern clients */
	fprintf(hostapd, "preamble=1\n");

	if (band) {
		if (!strcmp(band, "2.4GHz")) {
			/* hw_mode=g: 2.4GHz with 802.11g (OFDM) as baseline */
			fprintf(hostapd, "hw_mode=g\n");

			/*
			 * Disable legacy 802.11b rates (1, 2, 5.5, 11 Mbps).
			 * Slow clients using these rates consume excessive
			 * airtime, degrading performance for all clients.
			 * Rates in 0.5 Mbps units: 60=6M, 90=9M, etc.
			 */
			fprintf(hostapd, "supported_rates=60 90 120 180 240 360 480 540\n");
			fprintf(hostapd, "basic_rates=60 120 240\n");
		} else if (!strcmp(band, "5GHz") || !strcmp(band, "6GHz")) {
			/* hw_mode=a: 5GHz/6GHz with 802.11a (OFDM) as baseline */
			fprintf(hostapd, "hw_mode=a\n");
		}

		if (channel) {
			if (strcmp(channel, "auto") == 0) {
				/*
				 * Default channels when "auto" selected:
				 * - Ch 6: Center of 2.4GHz, least overlap with 1 and 11
				 * - Ch 36: First UNII-1 channel, indoor use, no DFS
				 * - Ch 109: 6GHz PSC channel, preferred for discovery
				 * TODO: Replace with ACS (channel=acs_survey) when driver support is verified.
				 */
				/* set to channel=acs_survey, if succeed to find a survey:
				   iw dev wlan0 survey dump
				   good:
				      Survey data from wlan0
 				       frequency:                      2412 MHz
				       noise:                          -95 dBm
				       channel active time:            154 ms
				       channel busy time:              0 ms
				       channel receive time:           0 ms
				       channel transmit time:          0 ms
				   bad:
				      Survey data from wlan0
  				       frequency:                      2412 MHz
				       [in use]

				 */
				if (!strcmp(band, "2.4GHz")) {
					fprintf(hostapd, "channel=6\n");
				} else if (!strcmp(band, "5GHz")) {
					fprintf(hostapd, "channel=36\n");
				} else if (!strcmp(band, "6GHz")) {
					fprintf(hostapd, "channel=109\n");
				} else {
					fprintf(hostapd, "channel=0\n");
				}
			} else {
				fprintf(hostapd, "channel=%s\n", channel);
			}
		}

		/*
		 * Enable high-throughput modes per band:
		 * - 802.11n (HT): Required for speeds >54Mbps, all bands
		 * - 802.11ac (VHT): 5GHz only, enables 80/160MHz and MU-MIMO
		 * - 802.11ax (HE): Always enabled, improves dense deployments with OFDMA
		 */
		if (!strcmp(band, "2.4GHz")) {
			fprintf(hostapd, "ieee80211n=1\n");
		} else if (!strcmp(band, "5GHz")) {
			fprintf(hostapd, "ieee80211n=1\n");
			fprintf(hostapd, "ieee80211ac=1\n");
		}
		/* 802.11ax (WiFi 6) always enabled for better performance */
		fprintf(hostapd, "ieee80211ax=1\n");

		/* Beamforming improves signal quality and range */
		fprintf(hostapd, "he_su_beamformer=1\n");
		fprintf(hostapd, "he_su_beamformee=1\n");
	}
	fprintf(hostapd, "\n");
}

/* Generate hostapd config for all APs on a radio (multi-SSID support) */
static int wifi_gen_aps_on_radio(const char *radio_name, struct lyd_node *cifs,
				  struct lyd_node *radio_node, struct lyd_node *config)
{
	struct lyd_node *primary_cif, *cif;
	const char *primary_ifname;
	char hostapd_conf[256];
	char **ap_list = NULL;
	FILE *hostapd = NULL;
	int rc = SR_ERR_OK;
	int ap_count = 0;
	mode_t oldmask;
	int i;

	oldmask = umask(0077);

	wifi_find_radio_aps(cifs, radio_name, &ap_list, &ap_count);
	if (ap_count == 0) {
		DEBUG("No APs found on radio %s", radio_name);
		goto cleanup;
	}

	DEBUG("Generating hostapd config for radio %s (%d APs)", radio_name, ap_count);

	primary_ifname = ap_list[0];
	primary_cif = NULL;
	LYX_LIST_FOR_EACH(cifs, cif, "interface") {
		if (!strcmp(lydx_get_cattr(cif, "name"), primary_ifname)) {
			primary_cif = cif;
			break;
		}
	}

	if (!primary_cif) {
		ERROR("Failed to find primary AP interface %s", primary_ifname);
		rc = SR_ERR_INVAL_ARG;
		goto cleanup;
	}

	snprintf(hostapd_conf, sizeof(hostapd_conf), HOSTAPD_CONF_NEXT, radio_name);

	hostapd = fopen(hostapd_conf, "w");
	if (!hostapd) {
		ERROR("Failed to create hostapd config: %s", hostapd_conf);
		rc = SR_ERR_INTERNAL;
		goto cleanup;
	}

	fprintf(hostapd, "# Generated by Infix confd - WiFi Radio %s\n", radio_name);
	fprintf(hostapd, "# Primary BSS: %s", primary_ifname);
	if (ap_count > 1)
		fprintf(hostapd, " (%d total APs)\n\n", ap_count);
	else
		fprintf(hostapd, "\n\n");

	fprintf(hostapd, "interface=%s\n", primary_ifname);
	fprintf(hostapd, "driver=nl80211\n");
	fprintf(hostapd, "ctrl_interface=/run/hostapd\n\n");

	/* Primary AP SSID and security configuration */
	wifi_gen_ssid_config(hostapd, primary_cif, config, false);
	fprintf(hostapd, "\n");

	/* Radio-specific configuration */
	wifi_gen_radio_config(hostapd, radio_node);

	/* Add BSS sections for secondary APs (multi-SSID) */
	for (i = 1; i < ap_count; i++) {
		struct lyd_node *bss_cif = NULL;

		LYX_LIST_FOR_EACH(cifs, cif, "interface") {
			if (!strcmp(lydx_get_cattr(cif, "name"), ap_list[i])) {
				bss_cif = cif;
				break;
			}
		}

		if (!bss_cif) {
			ERROR("Failed to find interface %s for BSS section", ap_list[i]);
			fclose(hostapd);
			rc = SR_ERR_INVAL_ARG;
			goto cleanup;
		}

		DEBUG("Adding BSS section for secondary AP %s", ap_list[i]);
		wifi_gen_ssid_config(hostapd, bss_cif, config, true);
	}

	fclose(hostapd);

cleanup:
	umask(oldmask);

	for (i = 0; i < ap_count; i++)
		free(ap_list[i]);
	free(ap_list);

	return rc;
}

static int hardware_cand(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
			 const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	sr_change_iter_t *iter;
	sr_change_oper_t op;
	sr_val_t *old, *new;
	sr_error_t err = SR_ERR_OK;
	struct confd *confd = (struct confd *)priv;

	switch (event) {
	case SR_EV_UPDATE:
	case SR_EV_CHANGE:
		break;
	default:
		return SR_ERR_OK;
	}
	err = sr_dup_changes_iter(session, "/ietf-hardware:hardware/component//*", &iter);
	if (err)
		return err;
	while (sr_get_change_next(session, iter, &op, &old, &new) == SR_ERR_OK) {
		switch (op) {
		case SR_OP_CREATED:
		case SR_OP_MODIFIED:
			break;
		default:
			continue;
		}
		err = hardware_cand_infer_class(confd->root, session, new->xpath);
		if (err)
			break;
	}
	sr_free_change_iter(iter);
	return err;
}

int hardware_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff,
		    sr_event_t event, struct confd *confd)
{
	struct lyd_node  *difs = NULL, *dif = NULL;
	int rc = SR_ERR_OK;

	if (!lydx_find_xpathf(diff, XPATH_BASE_))
		return SR_ERR_OK;

	difs = lydx_get_descendant(diff, "hardware", "component", NULL);

	LYX_LIST_FOR_EACH(difs, dif, "component") {
		enum lydx_op op;
		struct lyd_node *state, *cif;
		const char *admin_state;
		const char *class, *name;

		op = lydx_get_op(dif);
		name = lydx_get_cattr(dif, "name");

		/* Get the current config node for this component */
		cif = lydx_get_xpathf(config, "/hardware/component[name='%s']", name);
		if (!cif)
			continue;

		class = lydx_get_cattr(cif, "class");

		/* Handle USB components */
		if (!strcmp(class, "infix-hardware:usb")) {
			if (event != SR_EV_DONE)
				continue;

			if (op == LYDX_OP_DELETE) {
				/* Handle USB deletion */
				if (usb_authorize(confd->root, name, 0)) {
					rc = SR_ERR_INTERNAL;
					goto err;
				}
				continue;
			}
			state = lydx_get_child(cif, "state");
			admin_state = lydx_get_cattr(state, "admin-state");
			if (usb_authorize(confd->root, name, !strcmp(admin_state, "unlocked"))) {
				rc = SR_ERR_INTERNAL;
				goto err;
			}
		} else if (!strcmp(class, "infix-hardware:wifi")) {
			struct lyd_node *interfaces_config, *interfaces_diff;
			struct lyd_node **wifi_iface_list = NULL;
			struct lyd_node *ap;
			struct lyd_node *cwifi_radio;
			int wifi_iface_count = 0;
			char src[40], dst[40];
			int ap_interfaces = 0;

			switch (event) {
			case SR_EV_ABORT:
				continue;
			case SR_EV_CHANGE:
				break;
			case SR_EV_DONE:
				interfaces_diff = lydx_get_descendant(diff, "interfaces", "interface", NULL);

				wifi_find_interfaces_on_radio(interfaces_diff, name,
							      &wifi_iface_list, &wifi_iface_count);
				if (wifi_iface_count > 0) {
					bool running, enabled;

					ap = lydx_get_descendant(wifi_iface_list[0], "interface", "wifi", "access-point", NULL);
					if (ap && lydx_get_op(ap) != LYDX_OP_DELETE) {
						/* AP mode - activate hostapd for radio */
						snprintf(src, sizeof(src), HOSTAPD_CONF_NEXT, name);
						snprintf(dst, sizeof(dst), HOSTAPD_CONF, name);

						running = !systemf("initctl -bfq status hostapd:%s", name);
						enabled = fexistf(HOSTAPD_CONF_NEXT, name);

						if (enabled) {
							(void)rename(src, dst);
							ap_interfaces++;

							if (running)
								systemf("initctl -bfq touch hostapd@%s", name);
							else
								systemf("initctl -bfq enable hostapd@%s", name);
						}
					}
				}
				if (!ap_interfaces) {
					systemf("initctl -bfq disable hostapd@%s", name);
					erasef(HOSTAPD_CONF, name);
					erasef(HOSTAPD_CONF_NEXT, name);
				}
				free(wifi_iface_list);
				continue;
			default:
				continue;

			}

			cwifi_radio = lydx_get_child(cif, "wifi-radio");

			interfaces_config = lydx_get_descendant(config, "interfaces", "interface", NULL);

			wifi_find_interfaces_on_radio(interfaces_config, name,
						       &wifi_iface_list, &wifi_iface_count);


			if (!wifi_iface_count)
				continue;

			/* Generate AP config (hostapd) for all APs on this radio */
			rc = wifi_gen_aps_on_radio(name, interfaces_config, cwifi_radio, config);
			if (rc != SR_ERR_OK)
				ERROR("Failed to generate AP config for radio %s", name);
			/* Free the interface list */
			free(wifi_iface_list);
			wifi_iface_list = NULL;
			wifi_iface_count = 0;
		}
	}

err:

	return rc;
}
int hardware_candidate_init(struct confd *confd)
{
	int rc = 0;

	REGISTER_CHANGE(confd->cand, "ietf-hardware", XPATH_BASE_,
			SR_SUBSCR_UPDATE, hardware_cand, confd, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("Init hardware failed: %s", sr_strerror(rc));
	return rc;
}
