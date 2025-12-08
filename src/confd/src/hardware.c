/* SPDX-License-Identifier: BSD-3-Clause */
#include <jansson.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>
#include <jansson.h>
#include <ftw.h>
#include <libgen.h>
#include <limits.h>

#include "core.h"
#include "interfaces.h"
#include "dagger.h"

#define XPATH_BASE_              "/ietf-hardware:hardware"
#define HOSTAPD_CONF             "/etc/hostapd-%s.conf"
#define HOSTAPD_CONF_NEXT        HOSTAPD_CONF"+"
#define WPA_SUPPLICANT_CONF      "/etc/wpa_supplicant-%s.conf"
#define WPA_SUPPLICANT_CONF_NEXT WPA_SUPPLICANT_CONF"+"

static int dir_cb(const char *fpath, const struct stat *sb,
		  int typeflag, struct FTW *ftwbuf)
{
	char *filename;
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
	json_t *usb_port, *usb_ports;
	int index;

	usb_ports = json_object_get(root, "usb-ports");
	if (!usb_ports) /* No Infix controlled USB ports is ok */
		return 0;

	json_array_foreach(usb_ports, index, usb_port) {
		struct json_t *jname;

		jname = json_object_get(usb_port, "name");
		if (!jname || !json_is_string(jname)) {
			ERROR("Did not find USB hardware port (name) for %s", name);
			continue;
		}
		if (!strcmp(name, json_string_value(jname))) {
			struct json_t *jpath = json_object_get(usb_port, "path");
			char authorized_default_path[PATH_MAX];
			const char *path;

			if (!jpath || !json_is_string(jpath)) {
				ERROR("Did not find USB hardware port (path) for %s", name);
				continue;
			}

			/* Path now points to USB device directory, not the attribute file */
			path = json_string_value(jpath);
			snprintf(authorized_default_path, sizeof(authorized_default_path),
				 "%s/authorized_default", path);

			if (!enabled) {
				if (fexist(authorized_default_path)) {
					if (writedf(0, "w", "%s", authorized_default_path)) {
						ERROR("Failed to unauthorize %s", authorized_default_path);
						return 1;
					}
				}
			} else {
				char *rpath;

				rpath = realpath(path, NULL);
				if (rpath) {
					nftw(rpath, dir_cb, 0, FTW_DEPTH | FTW_PHYS);
					free(rpath);
				}
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
			break;
		}
	}
out_free_name:
	free(name);
out_free_xpath:
	free(xpath);
	return err;
}

/*
 * WiFi Radio Helper Functions
 */

/* Helper: Find all WiFi interfaces using a specific radio (phy) */
static int wifi_find_interfaces_on_radio(struct lyd_node *cifs, const char *radio_name,
					  struct lyd_node ***iface_list, int *count)
{
	struct lyd_node *cif, *wifi;
	const char *radio;
	struct lyd_node **list = NULL;
	int n = 0, cap = 0;

	if (!cifs)
		return 0;

	LYX_LIST_FOR_EACH(cifs, cif, "interface") {
		wifi = lydx_get_child(cif, "wifi");
		if (!wifi)
			continue;

		radio = lydx_get_cattr(wifi, "radio");
		if (!radio || strcmp(radio, radio_name))
			continue;

		/* Expand array if needed */
		if (n >= cap) {
			cap = cap ? cap * 2 : 4;
			list = realloc(list, sizeof(struct lyd_node *) * cap);
		}

		list[n++] = cif;
	}

	*iface_list = list;
	*count = n;
	return 0;
}

/* Generate wpa_supplicant config for a station interface */
static int wifi_gen_station(const char *ifname, struct lyd_node *station,
			     const char *radio, struct dagger *net)
{
	const char *ssid, *secret_name, *secret, *encryption_type;
	struct lyd_node *encryption, *secret_node, *radio_node;
	FILE *wpa_supplicant = NULL, *wpa = NULL;
	char *encryption_str = NULL;
	const char *country;
	int rc = SR_ERR_OK;

	ssid = lydx_get_cattr(station, "ssid");
	encryption = lydx_get_child(station, "encryption");
	encryption_type = lydx_get_cattr(encryption, "type");
	secret_name = lydx_get_cattr(encryption, "secret");

	/* Get country-code from radio configuration */
	radio_node = lydx_get_xpathf(station,
		"/ietf-hardware:hardware/component[name='%s']/infix-hardware:wifi-radio", radio);
	country = radio_node ? lydx_get_cattr(radio_node, "country-code") : NULL;

	if (!ssid || !encryption_type) {
		ERROR("WiFi station %s: missing required configuration", ifname);
		return SR_ERR_INVAL_ARG;
	}

	/* Get secret from keystore if encryption is enabled */
	if (secret_name && strcmp(encryption_type, "disabled") != 0) {
		secret_node = lydx_get_xpathf(station,
			"/ietf-keystore:keystore/symmetric-keys/symmetric-key[name='%s']",
			secret_name);
		secret = secret_node ? lydx_get_cattr(secret_node, "cleartext-key") : NULL;
	} else {
		secret = NULL;
	}

	/* Generate finit service enable script */
	wpa = dagger_fopen_net_init(net, ifname, NETDAG_INIT_POST, "wpa_supplicant.sh");
	if (!wpa) {
		rc = SR_ERR_INTERNAL;
		goto out;
	}

	fprintf(wpa, "# Generated by Infix confd - WiFi Station %s\n", ifname);
	fprintf(wpa, "if [ -f '/etc/finit.d/enabled/wifi@%s.conf' ];then\n", ifname);
	fprintf(wpa, "initctl -bfqn touch wifi@%s\n", ifname);
	fprintf(wpa, "else\n");
	fprintf(wpa, "initctl -bfqn enable wifi@%s\n", ifname);
	fprintf(wpa, "fi\n");
	fclose(wpa);

	/* Generate wpa_supplicant configuration */
	wpa_supplicant = fopenf("w", WPA_SUPPLICANT_CONF_NEXT, ifname);
	if (!wpa_supplicant) {
		rc = SR_ERR_INTERNAL;
		goto out;
	}

	fprintf(wpa_supplicant,
		"ctrl_interface=/run/wpa_supplicant\n"
		"autoscan=periodic:10\n"
		"ap_scan=1\n");

	if (country)
		fprintf(wpa_supplicant, "country=%s\n", country);

	/* Only generate network block if we have all required info */
	if (ssid && encryption_type) {
		if (!strcmp(encryption_type, "disabled")) {
			asprintf(&encryption_str, "key_mgmt=NONE");
		} else if (secret) {
			asprintf(&encryption_str, "key_mgmt=SAE WPA-PSK\npsk=\"%s\"", secret);
		} else {
			/* Encryption enabled but no secret yet - skip network block */
			goto out;
		}

		fprintf(wpa_supplicant,
			"network={\n"
			"bgscan=\"simple: 30:-45:300\"\n"
			"ssid=\"%s\"\n"
			"%s\n"
			"}\n", ssid, encryption_str);
		free(encryption_str);
	}

out:
	if (wpa_supplicant)
		fclose(wpa_supplicant);
	return rc;
}

/* Delete station configuration */
__attribute__((unused))
static int wifi_del_station(const char *ifname, struct dagger *net)
{
	FILE *cleanup = dagger_fopen_net_exit(net, ifname, NETDAG_EXIT_DAEMON, "wifi.sh");

	fprintf(cleanup, "# Generated by Infix confd - WiFi Station cleanup\n");
	fprintf(cleanup, "initctl -bfqn disable wifi@%s\n", ifname);
	fclose(cleanup);

	erasef("/etc/wpa_supplicant-%s.conf", ifname);
	return SR_ERR_OK;
}

/* Helper: Find all AP interfaces on a specific radio */
static int wifi_find_radio_aps(struct lyd_node *cifs, const char *radio_name,
				char ***ap_list, int *count)
{
	struct lyd_node *cif, *wifi, *ap;
	const char *ifname, *radio;
	char **list = NULL;
	int n = 0, cap = 0;

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

		/* Expand array if needed */
		if (n >= cap) {
			cap = cap ? cap * 2 : 4;
			list = realloc(list, sizeof(char *) * cap);
		}

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

/* Generate BSS section for secondary AP (multi-SSID) */
static int wifi_gen_bss_section(FILE *hostapd, struct lyd_node *cifs, const char *ifname)
{
	struct lyd_node *cif, *wifi, *ap, *security, *secret_node;
	const char *ssid, *hidden, *security_mode, *secret_name, *secret;

	/* Find the interface node for this BSS */
	LYX_LIST_FOR_EACH(cifs, cif, "interface") {
		const char *name = lydx_get_cattr(cif, "name");
		if (strcmp(name, ifname) == 0)
			break;
	}

	if (!cif) {
		ERROR("Failed to find interface %s for BSS section", ifname);
		return SR_ERR_INVAL_ARG;
	}

	wifi = lydx_get_child(cif, "wifi");
	ap = lydx_get_child(wifi, "access-point");

	fprintf(hostapd, "\n# BSS %s\n", ifname);
	fprintf(hostapd, "bss=%s\n", ifname);

	/* SSID configuration */
	ssid = lydx_get_cattr(ap, "ssid");
	hidden = lydx_get_cattr(ap, "hidden");

	if (ssid)
		fprintf(hostapd, "ssid=%s\n", ssid);
	if (hidden && !strcmp(hidden, "true"))
		fprintf(hostapd, "ignore_broadcast_ssid=1\n");

	/* Security configuration */
	security = lydx_get_child(ap, "security");
	security_mode = lydx_get_cattr(security, "mode");

	if (!security_mode)
		security_mode = "open";

	/* Get secret from keystore if needed */
	secret = NULL;
	if (strcmp(security_mode, "open") != 0) {
		secret_name = lydx_get_cattr(security, "secret");
		if (secret_name) {
			secret_node = lydx_get_xpathf(ap,
				"/ietf-keystore:keystore/symmetric-keys/symmetric-key[name='%s']",
				secret_name);
			if (secret_node)
				secret = lydx_get_cattr(secret_node, "cleartext-key");
		}
	}

	if (!strcmp(security_mode, "open")) {
		fprintf(hostapd, "# Open network\n");
		fprintf(hostapd, "auth_algs=1\n");
	} else if (!strcmp(security_mode, "wpa2-personal")) {
		fprintf(hostapd, "# WPA2-Personal\n");
		fprintf(hostapd, "wpa=2\n");
		fprintf(hostapd, "wpa_key_mgmt=WPA-PSK\n");
		fprintf(hostapd, "wpa_pairwise=CCMP\n");
		if (secret)
			fprintf(hostapd, "wpa_passphrase=%s\n", secret);
	} else if (!strcmp(security_mode, "wpa3-personal")) {
		fprintf(hostapd, "# WPA3-Personal\n");
		fprintf(hostapd, "wpa=2\n");
		fprintf(hostapd, "wpa_key_mgmt=SAE\n");
		fprintf(hostapd, "rsn_pairwise=CCMP\n");
		if (secret)
			fprintf(hostapd, "sae_password=%s\n", secret);
		fprintf(hostapd, "ieee80211w=2\n");
	} else if (!strcmp(security_mode, "wpa2-wpa3-personal")) {
		fprintf(hostapd, "# WPA2/WPA3 Mixed\n");
		fprintf(hostapd, "wpa=2\n");
		fprintf(hostapd, "wpa_key_mgmt=WPA-PSK SAE\n");
		fprintf(hostapd, "rsn_pairwise=CCMP\n");
		if (secret) {
			fprintf(hostapd, "wpa_passphrase=%s\n", secret);
			fprintf(hostapd, "sae_password=%s\n", secret);
		}
		fprintf(hostapd, "ieee80211w=1\n");
	}

	return 0;
}

/* Generate hostapd config for all APs on a radio (multi-SSID support) */
static int wifi_gen_aps_on_radio(const char *radio_name, struct lyd_node *cifs,
				  struct lyd_node *radio_node, struct dagger *net)
{
	const char *country, *channel, *channel_width;
	char hostapd_conf[256];
	char **ap_list = NULL;
	int ap_count = 0;
	FILE *hostapd = NULL;
	int rc = SR_ERR_OK;

	/* Find all APs on this radio */
	wifi_find_radio_aps(cifs, radio_name, &ap_list, &ap_count);

	if (ap_count == 0) {
		DEBUG("No APs found on radio %s", radio_name);
		return SR_ERR_OK;
	}

	DEBUG("Generating hostapd config for radio %s (%d APs)", radio_name, ap_count);

	/* Get primary AP interface node */
	const char *primary_ifname = ap_list[0];
	struct lyd_node *primary_cif = NULL, *cif;
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

	struct lyd_node *primary_wifi = lydx_get_child(primary_cif, "wifi");
	struct lyd_node *primary_ap = lydx_get_child(primary_wifi, "access-point");

	/* Get AP configuration */
	const char *ssid = lydx_get_cattr(primary_ap, "ssid");
	const char *hidden = lydx_get_cattr(primary_ap, "hidden");
	struct lyd_node *security = lydx_get_child(primary_ap, "security");
	const char *security_mode = lydx_get_cattr(security, "mode");
	const char *secret_name = lydx_get_cattr(security, "secret");
	const char *secret = NULL;

	if (!ssid || !security_mode) {
		ERROR("WiFi AP %s: missing required configuration", primary_ifname);
		rc = SR_ERR_INVAL_ARG;
		goto cleanup;
	}

	/* Get radio configuration */
	country = lydx_get_cattr(radio_node, "country-code");
	channel = lydx_get_cattr(radio_node, "channel");
	channel_width = lydx_get_cattr(radio_node, "channel-width");

	/* Get secret from keystore if not open network */
	if (secret_name && strcmp(security_mode, "open") != 0) {
		struct lyd_node *secret_node = lydx_get_xpathf(primary_ap,
			"/ietf-keystore:keystore/symmetric-keys/symmetric-key[name='%s']",
			secret_name);
		secret = secret_node ? lydx_get_cattr(secret_node, "cleartext-key") : NULL;
		if (!secret) {
			ERROR("WiFi AP %s: secret '%s' not found in keystore",
			      primary_ifname, secret_name);
			rc = SR_ERR_INVAL_ARG;
			goto cleanup;
		}
	}

	/* Generate hostapd configuration file (per-radio) */
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

	/* Interface configuration */
	fprintf(hostapd, "interface=%s\n", primary_ifname);
	fprintf(hostapd, "driver=nl80211\n");
	fprintf(hostapd, "ctrl_interface=/run/hostapd\n\n");

	/* SSID configuration */
	fprintf(hostapd, "ssid=%s\n", ssid);
	if (hidden && !strcmp(hidden, "true"))
		fprintf(hostapd, "ignore_broadcast_ssid=1\n");
	fprintf(hostapd, "\n");

	/* Radio configuration */
	if (country)
		fprintf(hostapd, "country_code=%s\n", country);

	/* Channel and band configuration */
	if (channel && strcmp(channel, "auto") != 0) {
		int ch = atoi(channel);

		/* Determine band from channel number */
		if (ch >= 1 && ch <= 14) {
			fprintf(hostapd, "hw_mode=g\n");  /* 2.4 GHz */
			fprintf(hostapd, "channel=%d\n", ch);
		} else if (ch >= 36 && ch <= 196) {
			fprintf(hostapd, "hw_mode=a\n");  /* 5 GHz */
			fprintf(hostapd, "channel=%d\n", ch);
		}

		/* Channel width / HT/VHT configuration */
		if (channel_width) {
			if (!strcmp(channel_width, "40")) {
				fprintf(hostapd, "ieee80211n=1\n");
				fprintf(hostapd, "ht_capab=[HT40+]\n");
			} else if (!strcmp(channel_width, "80")) {
				fprintf(hostapd, "ieee80211n=1\n");
				fprintf(hostapd, "ieee80211ac=1\n");
				fprintf(hostapd, "vht_oper_chwidth=1\n");
				fprintf(hostapd, "vht_oper_centr_freq_seg0_idx=%d\n", ch + 6);
			} else if (!strcmp(channel_width, "160")) {
				fprintf(hostapd, "ieee80211n=1\n");
				fprintf(hostapd, "ieee80211ac=1\n");
				fprintf(hostapd, "vht_oper_chwidth=2\n");
				fprintf(hostapd, "vht_oper_centr_freq_seg0_idx=%d\n", ch + 14);
			}
		}
	}
	fprintf(hostapd, "\n");

	/* Security configuration */
	if (!strcmp(security_mode, "open")) {
		fprintf(hostapd, "# Open network (no encryption)\n");
		fprintf(hostapd, "auth_algs=1\n");
	} else if (!strcmp(security_mode, "wpa2-personal")) {
		fprintf(hostapd, "# WPA2-Personal\n");
		fprintf(hostapd, "wpa=2\n");
		fprintf(hostapd, "wpa_key_mgmt=WPA-PSK\n");
		fprintf(hostapd, "wpa_pairwise=CCMP\n");
		fprintf(hostapd, "wpa_passphrase=%s\n", secret);
	} else if (!strcmp(security_mode, "wpa3-personal")) {
		fprintf(hostapd, "# WPA3-Personal\n");
		fprintf(hostapd, "wpa=2\n");
		fprintf(hostapd, "wpa_key_mgmt=SAE\n");
		fprintf(hostapd, "rsn_pairwise=CCMP\n");
		fprintf(hostapd, "sae_password=%s\n", secret);
		fprintf(hostapd, "ieee80211w=2\n");
	} else if (!strcmp(security_mode, "wpa2-wpa3-personal")) {
		fprintf(hostapd, "# WPA2/WPA3 Mixed Mode\n");
		fprintf(hostapd, "wpa=2\n");
		fprintf(hostapd, "wpa_key_mgmt=WPA-PSK SAE\n");
		fprintf(hostapd, "rsn_pairwise=CCMP\n");
		fprintf(hostapd, "wpa_passphrase=%s\n", secret);
		fprintf(hostapd, "sae_password=%s\n", secret);
		fprintf(hostapd, "ieee80211w=1\n");
	}

	/* Add BSS sections for secondary APs (multi-SSID) */
	for (int i = 1; i < ap_count; i++) {
		DEBUG("Adding BSS section for secondary AP %s", ap_list[i]);
		rc = wifi_gen_bss_section(hostapd, cifs, ap_list[i]);
		if (rc != SR_ERR_OK) {
			ERROR("Failed to generate BSS section for %s", ap_list[i]);
			fclose(hostapd);
			goto cleanup;
		}
	}

	fclose(hostapd);

cleanup:
	for (int i = 0; i < ap_count; i++)
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

int hardware_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
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
			state = lydx_get_child(dif, "state");
			admin_state = lydx_get_cattr(state, "admin-state");
			if (usb_authorize(confd->root, name, !strcmp(admin_state, "unlocked"))) {
				rc = SR_ERR_INTERNAL;
				goto err;
			}
		}

		/* Handle WiFi radio components */
		if (!strcmp(class, "infix-hardware:wifi")) {
			struct lyd_node *interfaces_config, *interfaces_diff;
			struct lyd_node **wifi_iface_list = NULL;
			struct lyd_node *station;
			int wifi_iface_count = 0;

			switch (event) {
			case SR_EV_ABORT:
				continue;
			case SR_EV_CHANGE:
				break;
			case SR_EV_DONE:
				char src[40], dst[40];

				/* Get interfaces configuration */
				interfaces_diff = lydx_get_descendant(diff, "interfaces", "interface", NULL);

				wifi_find_interfaces_on_radio(interfaces_diff, name,
							      &wifi_iface_list, &wifi_iface_count);
				ERROR("DONE: Found %d WiFi interfaces on radio %s", wifi_iface_count, name);
				if (wifi_iface_count > 0) {
					bool running, enabled;
					station = lydx_get_descendant(wifi_iface_list[0], "interface", "wifi", "station", NULL);
					if (station) {
						const char *ifname = lydx_get_cattr(wifi_iface_list[0], "name");
						snprintf(src, sizeof(src), WPA_SUPPLICANT_CONF_NEXT, ifname);
						snprintf(dst, sizeof(dst), WPA_SUPPLICANT_CONF, ifname);
						running = !systemf("initctl -bfq status wifi@%s", ifname);
						enabled = fexistf(WPA_SUPPLICANT_CONF_NEXT, ifname);

						if (enabled && lydx_get_xpathf(config, "/ietf-interfaces:interfaces/interface[name='%s']", ifname)) { /* if enabled and not removed */
							rename(src, dst);

							ERROR("WPA SUPPLICANT RUNNING: %s", running ? "YES" : "NO");
							if (running)
								systemf("initctl -bfq touch wifi@%s", ifname);
							else
								systemf("initctl -bfq enable wifi@%s", ifname);
						} else {
							erase(dst);
							systemf("initctl -bfq disable wifi@%s", ifname);
						}
					} else if (wifi_iface_count > 0) {
						/* AP mode - activate hostapd for radio */
						snprintf(src, sizeof(src), HOSTAPD_CONF_NEXT, name);
						snprintf(dst, sizeof(dst), HOSTAPD_CONF, name);

						running = !systemf("initctl -bfq status hostapd@%s", name);
						enabled = fexistf(HOSTAPD_CONF_NEXT, name);

						if (enabled) {
							rename(src, dst);
							ERROR("HOSTAPD for radio %s: config activated", name);

							if (running)
								systemf("initctl -bfq touch hostapd@%s", name);
							else
								systemf("initctl -bfq enable hostapd@%s", name);
						} else {
							erase(dst);
							systemf("initctl -bfq disable hostapd@%s", name);
						}
					}
				}
				continue;
			default:
				continue;

			}
			struct lyd_node *cwifi_radio = lydx_get_child(cif, "wifi-radio");

			if (!cwifi_radio) {
				DEBUG("WiFi component %s has no wifi-radio config", name);
				continue;
			}

			ERROR("WiFi radio %s changed, processing interfaces", name);

			/* Get interfaces configuration for WiFi radio processing */
			interfaces_config = lydx_get_descendant(config, "interfaces", "interface", NULL);

			/* Find all interfaces using this radio */
			wifi_find_interfaces_on_radio(interfaces_config, name,
						       &wifi_iface_list, &wifi_iface_count);

			ERROR("CHANGE: Found %d WiFi interfaces on radio %s", wifi_iface_count, name);

			if (!wifi_iface_count)
				continue;
			/*
			 * A radio operates in one of two mutually exclusive modes:
			 * 1. Station mode: One station interface (client mode)
			 * 2. AP mode: One or more AP interfaces (hostapd multi-SSID)
			 *
			 * Check for station first - there can be at most one per radio.
			 */
			station = lydx_get_descendant(wifi_iface_list[0], "interface", "wifi", "station", NULL);
			if (wifi_iface_count == 1 && station) {
				struct lyd_node *iface = wifi_iface_list[0];
				if (station && lydx_is_enabled(iface, "enabled")) {
					const char *ifname = lydx_get_cattr(iface, "name");
					ERROR("Generating wpa_supplicant for station %s", ifname);
					rc = wifi_gen_station(ifname, station, name, &confd->netdag);
					if (rc != SR_ERR_OK) {
						ERROR("Failed to generate station config for %s", ifname);
						goto next;
					}
				}
			} else {
				rc = wifi_gen_aps_on_radio(name, interfaces_config, cwifi_radio, &confd->netdag);
				if (rc != SR_ERR_OK) {
					ERROR("Failed to generate AP config for radio %s", name);
					goto next;
				}
			}
		next:
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
