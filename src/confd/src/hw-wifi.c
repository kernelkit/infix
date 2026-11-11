/* SPDX-License-Identifier: BSD-3-Clause */
#include <glob.h>
#include <jansson.h>
#include <net/if.h>

#include <srx/common.h>
#include <srx/lyx.h>

#include "core.h"
#include "interfaces.h"
#include "hardware.h"

#define HOSTAPD_CONF             "/etc/hostapd-%s.conf"
#define HOSTAPD_CONF_NEXT        HOSTAPD_CONF"+"
#define HOSTAPD_SERVICE          "/etc/finit.d/available/hostapd.conf"

int wifi_find_interfaces_on_radio(struct lyd_node *ifs, const char *radio_name,
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
		list = realloc(list, sizeof(struct lyd_node *) * (n + 1));
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
		/* hostapd brings every BSS it is given up, a disabled one
		 * must not be in its config at all. */
		if (!lydx_is_enabled(cif, "enabled"))
			continue;
		list = realloc(list, sizeof(char *) * (n + 1));

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

/*
 * Center channel for 80MHz VHT/HE operation.
 * 5GHz 80MHz groups: 36-48(42), 52-64(58), 100-112(106),
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

/*
 * 6GHz center channel index for 80MHz HE operation.
 * 6GHz channels are uniformly spaced at 5MHz intervals from 5950MHz.
 * 80MHz groups each span 16 channel numbers: 1-13(7), 17-29(23), etc.
 */
static int wifi_6ghz_center_chan_80(int ch)
{
	/* Round down to group start, add 6 to reach center */
	int group_start = ((ch - 1) / 16) * 16 + 1;

	return group_start + 6;
}

/*
 * 6GHz center channel index for 160MHz HE operation.
 * 160MHz groups each span 32 channel numbers: 1-29(15), 33-61(47), etc.
 */
static int wifi_6ghz_center_chan_160(int ch)
{
	int group_start = ((ch - 1) / 32) * 32 + 1;

	return group_start + 14;
}

/* HT40 secondary channel direction: lower channel in pair uses +, upper uses - */
static const char *wifi_ht40_dir(int ch)
{
	return ((ch / 4) % 2) ? "[HT40+]" : "[HT40-]";
}

/*
 * Read HT/VHT capability bitmasks and HE support from hardware via iw.py.
 * Returns 0 on success, -1 on failure.
 */
static int wifi_read_phy_caps(const char *radio_name, unsigned int *ht_cap,
			      unsigned int *vht_cap, bool *he)
{
	json_error_t jerr;
	json_t *root, *jht, *jvht;
	char buf[256];
	size_t len;
	FILE *pp;

	*ht_cap = 0;
	*vht_cap = 0;
	*he = false;

	pp = popenf("r", "/usr/libexec/infix/iw.py caps %s", radio_name);
	if (!pp)
		return -1;

	len = fread(buf, 1, sizeof(buf) - 1, pp);
	pclose(pp);
	buf[len] = '\0';

	/*
	 * Parse JSON output: {"ht_cap": NNN, "vht_cap": NNN, "he": bool}
	 * Use jansson since hardware.c already includes it.
	 */
	root = json_loads(buf, 0, &jerr);
	if (!root)
		return -1;

	jht = json_object_get(root, "ht_cap");
	jvht = json_object_get(root, "vht_cap");

	if (json_is_integer(jht))
		*ht_cap = (unsigned int)json_integer_value(jht);
	if (json_is_integer(jvht))
		*vht_cap = (unsigned int)json_integer_value(jvht);
	*he = json_is_true(json_object_get(root, "he"));

	json_decref(root);
	return 0;
}

/*
 * Build hostapd ht_capab string from HT capability bitmask.
 * IEEE 802.11-2016 Table 9-153 — HT Capabilities Info field
 *
 * The ht40_dir string ("[HT40+]" or "[HT40-]") is prepended when
 * the configured channel width is 40MHz or wider.
 */
static void wifi_build_ht_capab(char *out, size_t sz, unsigned int ht_cap,
				const char *ht40_dir, int want_ht40)
{
	char *p = out;
	size_t rem = sz;
	int n;

	*p = '\0';

	if (want_ht40 && ht40_dir) {
		n = snprintf(p, rem, "%s", ht40_dir);
		p += n;
		rem -= n;
	}

	if (ht_cap & 0x0001) {
		n = snprintf(p, rem, "[LDPC]");
		p += n;
		rem -= n;
	}
	if (ht_cap & 0x0020) {
		n = snprintf(p, rem, "[SHORT-GI-20]");
		p += n;
		rem -= n;
	}
	if (ht_cap & 0x0040) {
		n = snprintf(p, rem, "[SHORT-GI-40]");
		p += n;
		rem -= n;
	}
	if (ht_cap & 0x0080) {
		n = snprintf(p, rem, "[TX-STBC]");
		p += n;
		rem -= n;
	}

	/* RX-STBC: bits 8-9 */
	switch ((ht_cap >> 8) & 0x3) {
	case 1:
		n = snprintf(p, rem, "[RX-STBC1]");
		p += n;
		rem -= n;
		break;
	case 2:
		n = snprintf(p, rem, "[RX-STBC12]");
		p += n;
		rem -= n;
		break;
	case 3:
		n = snprintf(p, rem, "[RX-STBC123]");
		p += n;
		rem -= n;
		break;
	}

	/* Max A-MSDU Length: bit 11 */
	if (ht_cap & 0x0800) {
		n = snprintf(p, rem, "[MAX-AMSDU-7935]");
		p += n;
		rem -= n;
	}
}

/*
 * Build hostapd vht_capab string from VHT capability bitmask.
 * IEEE 802.11-2016 Table 9-250 — VHT Capabilities Info field
 *
 * When the configured width is less than 160MHz, the VHT160 and
 * SHORT-GI-160 flags are masked out to prevent hostapd from
 * advertising capabilities the configuration does not use.
 */
static void wifi_build_vht_capab(char *out, size_t sz, unsigned int vht_cap, int chwidth)
{
	char *p = out;
	size_t rem = sz;
	int n;

	*p = '\0';

	/* Max MPDU Length: bits 0-1 */
	switch (vht_cap & 0x3) {
	case 1:
		n = snprintf(p, rem, "[MAX-MPDU-7991]");
		p += n;
		rem -= n;
		break;
	case 2:
		n = snprintf(p, rem, "[MAX-MPDU-11454]");
		p += n;
		rem -= n;
		break;
	}

	/* Supported Channel Width: bits 2-3 */
	if (chwidth >= 2) {
		switch ((vht_cap >> 2) & 0x3) {
		case 1:
			n = snprintf(p, rem, "[VHT160]");
			p += n;
			rem -= n;
			break;
		case 2:
			n = snprintf(p, rem, "[VHT160-80PLUS80]");
			p += n;
			rem -= n;
			break;
		}
	}

	/* RXLDPC: bit 4 */
	if (vht_cap & 0x10) {
		n = snprintf(p, rem, "[RXLDPC]");
		p += n;
		rem -= n;
	}

	/* Short GI for 80MHz: bit 5 */
	if (vht_cap & 0x20) {
		n = snprintf(p, rem, "[SHORT-GI-80]");
		p += n;
		rem -= n;
	}

	/* Short GI for 160MHz: bit 6 — only when configured for 160MHz */
	if (chwidth >= 2 && (vht_cap & 0x40)) {
		n = snprintf(p, rem, "[SHORT-GI-160]");
		p += n;
		rem -= n;
	}

	/* TX STBC: bit 7 */
	if (vht_cap & 0x80) {
		n = snprintf(p, rem, "[TX-STBC-2BY1]");
		p += n;
		rem -= n;
	}

	/* RX STBC: bits 8-10 */
	switch ((vht_cap >> 8) & 0x7) {
	case 1:
		n = snprintf(p, rem, "[RX-STBC-1]");
		p += n;
		rem -= n;
		break;
	case 2:
		n = snprintf(p, rem, "[RX-STBC-12]");
		p += n;
		rem -= n;
		break;
	case 3:
		n = snprintf(p, rem, "[RX-STBC-123]");
		p += n;
		rem -= n;
		break;
	case 4:
		n = snprintf(p, rem, "[RX-STBC-1234]");
		p += n;
		rem -= n;
		break;
	}

	/* SU Beamformer: bit 11 */
	if (vht_cap & 0x800) {
		n = snprintf(p, rem, "[SU-BEAMFORMER]");
		p += n;
		rem -= n;
	}

	/* SU Beamformee: bit 12 */
	if (vht_cap & 0x1000) {
		n = snprintf(p, rem, "[SU-BEAMFORMEE]");
		p += n;
		rem -= n;
	}

	/* MU Beamformer: bit 19 */
	if (vht_cap & 0x80000) {
		n = snprintf(p, rem, "[MU-BEAMFORMER]");
		p += n;
		rem -= n;
	}

	/* MU Beamformee: bit 20 */
	if (vht_cap & 0x100000) {
		n = snprintf(p, rem, "[MU-BEAMFORMEE]");
		p += n;
		rem -= n;
	}

	/* VHT TXOP PS: bit 21 */
	if (vht_cap & 0x200000) {
		n = snprintf(p, rem, "[VHT-TXOP-PS]");
		p += n;
		rem -= n;
	}

	/* HTC-VHT: bit 22 */
	if (vht_cap & 0x400000) {
		n = snprintf(p, rem, "[HTC-VHT]");
		p += n;
		rem -= n;
	}

	/* Max A-MPDU Length Exponent: bits 23-25 */
	n = (vht_cap >> 23) & 0x7;
	if (n) {
		int wrote = snprintf(p, rem, "[MAX-A-MPDU-LEN-EXP%d]", n);
		p += wrote;
		rem -= wrote;
	}
}

/* The box-wide WiFi country code, NULL when none is configured */
const char *wifi_country_code(struct lyd_node *config)
{
	struct lyd_node *wifi;

	wifi = lydx_get_descendant(config, "hardware", "wifi", NULL);
	return lydx_get_cattr(wifi, "country-code");
}

/* Helper: Write radio-specific configuration */
static void wifi_gen_radio_config(FILE *hostapd, const char *radio_name,
				  struct lyd_node *radio_node, struct lyd_node *config)
{
	const char *country, *channel, *band, *width;
	unsigned int ht_cap = 0, vht_cap = 0;
	char ht_capab[512], vht_capab[512];
	int chwidth = 0; /* 0=20/40, 1=80, 2=160 */
	int ch = 0;
	bool legacy_rates, he = false;

	country = wifi_country_code(config);
	band = lydx_get_cattr(radio_node, "band");
	channel = lydx_get_cattr(radio_node, "channel");
	width = lydx_get_cattr(radio_node, "channel-width");
	legacy_rates = lydx_is_enabled(radio_node, "legacy-rates");
	if (channel && strcmp(channel, "auto"))
		ch = atoi(channel);

	/* Read HT/VHT/HE hardware capabilities from PHY, hostapd refuses
	 * to start with a mode the driver does not support. */
	wifi_read_phy_caps(radio_name, &ht_cap, &vht_cap, &he);

	if (country)
		fprintf(hostapd, "country_code=%s\n", country);

	/* Keep our address on a second BSS, not the primary's plus one */
	fprintf(hostapd, "use_driver_iface_addr=1\n");

	/*
	 * Every access point is created with its address before hostapd
	 * starts.  Without this hostapd gives a second BSS the primary's
	 * address plus one and changes the interface under us.
	 */
	fprintf(hostapd, "use_driver_iface_addr=1\n");

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

	/* Dozing clients wake on every beacon, bounds buffered reply latency to one beacon interval */
	fprintf(hostapd, "dtim_period=1\n");

	/* Let power-saving clients trigger delivery instead of waiting for a DTIM beacon */
	fprintf(hostapd, "uapsd_advertisement_enabled=1\n");

	/* Use short preamble for better throughput on modern clients */
	fprintf(hostapd, "preamble=1\n");

	/* Remember the clients this radio has seen, for band steering on
	 * the other radios.  A dual-band client is refused on 2.4 GHz
	 * while it is on this list, so keep the list short-lived: that is
	 * the longest a client that cannot get in on 5 GHz has to wait. */
	fprintf(hostapd, "track_sta_max_num=100\n");
	fprintf(hostapd, "track_sta_max_age=60\n");

	if (band) {
		if (!strcmp(band, "2.4GHz")) {
			/* hw_mode=g: 2.4GHz with 802.11g (OFDM) as baseline */
			fprintf(hostapd, "hw_mode=g\n");

			/*
			 * Disable legacy 802.11b rates (1, 2, 5.5, 11 Mbps)
			 * unless explicitly enabled via 'legacy-rates'. Slow
			 * 802.11b clients consume excessive airtime, degrading
			 * performance for all clients. When enabled, hostapd
			 * keeps its default rate set so old 2.4GHz-only IoT
			 * devices can still associate.
			 * Rates in 0.5 Mbps units: 60=6M, 90=9M, etc.
			 */
			if (!legacy_rates) {
				fprintf(hostapd, "supported_rates=60 90 120 180 240 360 480 540\n");
				fprintf(hostapd, "basic_rates=60 120 240\n");
			}
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
				 * - Ch 37: 6GHz PSC channel (6135 MHz), preferred for discovery
				 * TODO: Replace with ACS (channel=acs_survey) when driver support is verified.
				 */
				if (!strcmp(band, "2.4GHz")) {
					fprintf(hostapd, "channel=6\n");
					ch = 6;
				} else if (!strcmp(band, "5GHz")) {
					fprintf(hostapd, "channel=36\n");
					ch = 36;
				} else if (!strcmp(band, "6GHz")) {
					fprintf(hostapd, "channel=37\n");
					ch = 37;
				} else {
					fprintf(hostapd, "channel=0\n");
				}
			} else {
				fprintf(hostapd, "channel=%s\n", channel);
			}
		}

		if (!strcmp(band, "2.4GHz")) {
			fprintf(hostapd, "ieee80211n=1\n");
			if (he)
				fprintf(hostapd, "ieee80211ax=1\n");
		} else if (!strcmp(band, "5GHz")) {
			fprintf(hostapd, "ieee80211n=1\n");
			if (vht_cap)
				fprintf(hostapd, "ieee80211ac=1\n");
			if (he)
				fprintf(hostapd, "ieee80211ax=1\n");
		} else if (!strcmp(band, "6GHz")) {
			/* 6GHz is HE-only, no HT/VHT */
			fprintf(hostapd, "ieee80211ax=1\n");
			fprintf(hostapd, "he_6ghz_reg_pwr_type=0\n"); /* Indoor AP */
		}

		/*
		 * Channel width + HT/VHT capability configuration.
		 *
		 * hostapd requires explicit ht_capab and vht_capab strings
		 * that match hardware capabilities.  Without vht_capab, 160MHz
		 * support is not advertised in beacons, causing clients to
		 * connect at 80MHz.  Read capabilities from hardware via iw.py
		 * and build the capability strings from the bitmasks.
		 */
		if (!strcmp(band, "6GHz")) {
			int op_class = 133; /* default 80MHz for 6GHz */

			if (width && strcmp(width, "auto")) {
				if (!strcmp(width, "20MHz"))
					op_class = 131;
				else if (!strcmp(width, "40MHz"))
					op_class = 132;
				else if (!strcmp(width, "80MHz"))
					op_class = 133;
				else if (!strcmp(width, "160MHz"))
					op_class = 134;
			}
			fprintf(hostapd, "op_class=%d\n", op_class);

			if (ch && op_class >= 133) {
				int center = (op_class == 134)
					? wifi_6ghz_center_chan_160(ch)
					: wifi_6ghz_center_chan_80(ch);

				fprintf(hostapd, "he_oper_centr_freq_seg0_idx=%d\n", center);
			}
		} else if (width && strcmp(width, "auto")) {
			if (!strcmp(width, "20MHz")) {
				chwidth = 0;
				wifi_build_ht_capab(ht_capab, sizeof(ht_capab), ht_cap, NULL, 0);
				fprintf(hostapd, "ht_capab=%s\n", ht_capab);
				if (strcmp(band, "2.4GHz")) {
					if (vht_cap)
						fprintf(hostapd, "vht_oper_chwidth=0\n");
					if (he)
						fprintf(hostapd, "he_oper_chwidth=0\n");
				}
			} else if (!strcmp(width, "40MHz")) {
				chwidth = 0;
				wifi_build_ht_capab(ht_capab, sizeof(ht_capab), ht_cap,
						    ch ? wifi_ht40_dir(ch) : "[HT40+]", 1);
				fprintf(hostapd, "ht_capab=%s\n", ht_capab);
				if (strcmp(band, "2.4GHz")) {
					if (vht_cap)
						fprintf(hostapd, "vht_oper_chwidth=0\n");
					if (he)
						fprintf(hostapd, "he_oper_chwidth=0\n");
				}
			} else if (!strcmp(width, "80MHz") && ch) {
				int center = wifi_center_chan_80(ch);

				chwidth = 1;
				wifi_build_ht_capab(ht_capab, sizeof(ht_capab), ht_cap, wifi_ht40_dir(ch), 1);
				wifi_build_vht_capab(vht_capab, sizeof(vht_capab), vht_cap, chwidth);
				fprintf(hostapd, "ht_capab=%s\n", ht_capab);
				fprintf(hostapd, "vht_capab=%s\n", vht_capab);
				fprintf(hostapd, "vht_oper_chwidth=1\n");
				if (he)
					fprintf(hostapd, "he_oper_chwidth=1\n");
				if (center) {
					fprintf(hostapd, "vht_oper_centr_freq_seg0_idx=%d\n", center);
					if (he)
						fprintf(hostapd, "he_oper_centr_freq_seg0_idx=%d\n", center);
				}
			} else if (!strcmp(width, "160MHz") && ch) {
				int center = wifi_center_chan_160(ch);

				chwidth = 2;
				wifi_build_ht_capab(ht_capab, sizeof(ht_capab), ht_cap, wifi_ht40_dir(ch), 1);
				wifi_build_vht_capab(vht_capab, sizeof(vht_capab), vht_cap, chwidth);
				fprintf(hostapd, "ht_capab=%s\n", ht_capab);
				fprintf(hostapd, "vht_capab=%s\n", vht_capab);
				fprintf(hostapd, "vht_oper_chwidth=2\n");
				if (he)
					fprintf(hostapd, "he_oper_chwidth=2\n");
				if (center) {
					fprintf(hostapd, "vht_oper_centr_freq_seg0_idx=%d\n", center);
					if (he)
						fprintf(hostapd, "he_oper_centr_freq_seg0_idx=%d\n", center);
				}
			}
		}

		if (he) {
			fprintf(hostapd, "he_su_beamformer=1\n");
			fprintf(hostapd, "he_su_beamformee=1\n");
		}
	}
	fprintf(hostapd, "\n");
}

/* Generate hostapd config for all APs on a radio (multi-SSID support) */
int wifi_gen_aps_on_radio(const char *radio_name, struct lyd_node *cifs,
			  struct lyd_node *radio_node, struct lyd_node *config,
			  struct confd *confd)
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
		ERRNO("Failed to create hostapd config: %s", hostapd_conf);
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
	wifi_gen_ssid_config(hostapd, primary_cif, config, false, lydx_get_cattr(radio_node, "band"), confd);
	fprintf(hostapd, "\n");

	/* Radio-specific configuration */
	wifi_gen_radio_config(hostapd, radio_name, radio_node, config);

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
		wifi_gen_ssid_config(hostapd, bss_cif, config, true, lydx_get_cattr(radio_node, "band"), confd);
	}

	fclose(hostapd);

cleanup:
	umask(oldmask);

	for (i = 0; i < ap_count; i++)
		free(ap_list[i]);
	free(ap_list);

	return rc;
}

static void wifi_gen_helpers(FILE *fp, glob_t *gl)
{
	bool neighbors = false;
	size_t i;

	for (i = 0; i < gl->gl_pathc; i++) {
		char line[256], bss[IFNAMSIZ] = "", twin[IFNAMSIZ];
		FILE *conf;

		conf = fopen(gl->gl_pathv[i], "r");
		if (!conf)
			continue;
		while (fgets(line, sizeof(line), conf)) {
			if (sscanf(line, "interface=%15s", bss) == 1 ||
			    sscanf(line, "bss=%15s", bss) == 1)
				continue;
			if (!strncmp(line, "ft_iface=", 9))
				neighbors = true;
			if (bss[0] && sscanf(line, "no_probe_resp_if_seen_on=%15s", twin) == 1) {
				fprintf(fp, "\nservice <!pid/hostapd> name:wifi-steer :%s \\\n", bss);
				fprintf(fp, "\t[2345] /usr/libexec/infix/wifi-steer.sh %s %s \\\n", bss, twin);
				fprintf(fp, "\t-- Wi-Fi band steering %s\n", bss);
			}
		}
		fclose(conf);
	}

	if (!neighbors)
		return;

	fprintf(fp, "\nservice <!pid/hostapd> name:wifi-neighbors \\\n");
	fprintf(fp, "\t[2345] /usr/libexec/infix/wifi-neighbors.py");
	for (i = 0; i < gl->gl_pathc; i++)
		fprintf(fp, " %s", gl->gl_pathv[i]);
	fprintf(fp, " \\\n\t-- Wi-Fi neighbor exchange\n");
}

int hardware_wifi_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff,
			 sr_event_t event, struct confd *confd)
{
	struct lyd_node *difs, *dif, *cif;
	int wifi_changed = 0;
	int rc = SR_ERR_OK;

	if (event != SR_EV_DONE || !lydx_find_xpathf(diff, XPATH_BASE_))
		return SR_ERR_OK;

	if (lydx_get_xpathf(diff, XPATH_BASE_ "/infix-hardware:wifi/country-code"))
		difs = lydx_get_descendant(config, "hardware", "component", NULL);
	else
		difs = lydx_get_descendant(diff, "hardware", "component", NULL);

	LYX_LIST_FOR_EACH(difs, dif, "component") {
		const char *name = lydx_get_cattr(dif, "name");
		char src[40], dst[40], **ap_list = NULL;
		int ap_interfaces = 0, i;

		cif = lydx_get_xpathf(config, "/hardware/component[name='%s']", name);
		if (!cif || strcmp(lydx_get_cattr(cif, "class"), "infix-hardware:wifi"))
			continue;

		/* The changed interface need not be an AP, a wds-link or
		 * station on the radio also lands here, so judge by the
		 * APs left in config. */
		wifi_find_radio_aps(lydx_get_descendant(config, "interfaces", "interface", NULL),
				    name, &ap_list, &ap_interfaces);
		for (i = 0; i < ap_interfaces; i++)
			free(ap_list[i]);
		free(ap_list);

		if (ap_interfaces) {
			snprintf(src, sizeof(src), HOSTAPD_CONF_NEXT, name);
			snprintf(dst, sizeof(dst), HOSTAPD_CONF, name);

			if (fexistf(HOSTAPD_CONF_NEXT, name))
				(void)rename(src, dst);
		} else {
			erasef(HOSTAPD_CONF, name);
			erasef(HOSTAPD_CONF_NEXT, name);
		}
		wifi_changed = 1;
	}

	/*
	 * All AP radios run in a single hostapd process so that
	 * cross-radio directives (e.g. no_probe_resp_if_seen_on for band
	 * steering) resolve: those only consult interfaces managed by the
	 * same process.  Regenerate the combined service from every staged
	 * /etc/hostapd-<radio>.conf and (re)start it.  A restart re-reads
	 * all radio configs, so band edits and radio add/remove both apply.
	 */
	if (wifi_changed) {
		struct lyd_node *cifs;
		bool lar_scan = false;
		glob_t gl = { 0 };
		size_t i;

		/* Need to remove old hostapd config files, for radios not used anymore */
		cifs = lydx_get_descendant(config, "interfaces", "interface", NULL);
		if (glob("/etc/hostapd-*.conf", 0, NULL, &gl) == 0) {
			for (i = 0; i < gl.gl_pathc; i++) {
				char radio[64], **aps = NULL;
				int n = 0, j;

				if (sscanf(gl.gl_pathv[i], "/etc/hostapd-%63[^.].conf", radio) != 1)
					continue;
				wifi_find_radio_aps(cifs, radio, &aps, &n);
				for (j = 0; j < n; j++)
					free(aps[j]);
				free(aps);
				if (!n) {
					unlink(gl.gl_pathv[i]);
					erasef(HOSTAPD_CONF_NEXT, radio);
				} else if (iface_has_quirk(radio, "lar-needs-scan")) {
					lar_scan = true;
				}
			}
			globfree(&gl);
		}

		gl = (glob_t){ 0 };
		if (glob("/etc/hostapd-*.conf", 0, NULL, &gl) == 0 && gl.gl_pathc > 0) {
			FILE *fp;

			fp = fopenp(HOSTAPD_SERVICE, 0644, NULL);
			if (!fp) {
				ERRNO("Could not open " HOSTAPD_SERVICE);
				rc = SR_ERR_INTERNAL;
			} else {
				fprintf(fp, "# Generated by confd, do not edit.\n");
				/* kill:10 gives the stop script time to hand over clients */
				fprintf(fp, "service <!> name:hostapd pid:!/run/hostapd.pid kill:10"
					" stop:/usr/libexec/infix/wifi-handover.sh \\\n");
				if (lar_scan)
					fprintf(fp, "\tpre:60,/usr/libexec/infix/wifi-scan-lar.sh \\\n");
				fprintf(fp, "\t[2345] hostapd -g /run/hostapd.global -P /run/hostapd.pid");
				for (i = 0; i < gl.gl_pathc; i++)
					fprintf(fp, " %s", gl.gl_pathv[i]);
				fprintf(fp, " \\\n\t-- Wi-Fi Access Points\n");
				wifi_gen_helpers(fp, &gl);
				fclose(fp);

				finit_enable("hostapd");
				finit_reload("hostapd");
			}
		} else {
			unlink(HOSTAPD_SERVICE);
			finit_disable("hostapd");
		}
		globfree(&gl);
	}

	return rc;
}

/*
 * Scan all channels on a radio and report how busy each one is, the
 * channel-survey action in infix-hardware.yang.  The scan takes the
 * radio off its operating channel for a few seconds, which is why this
 * is an action and not operational data.  The helper does the scan and
 * the parsing, see iw.py survey-scan.
 */
static int wifi_channel_survey(sr_session_ctx_t *session, uint32_t sub_id, const char *op_path,
			       const struct lyd_node *input, sr_event_t event, uint32_t request_id,
			       struct lyd_node *output, void *priv)
{
	static const struct { const char *json, *yang; } times[] = {
		{ "noise",         "noise"         },
		{ "active_time",   "active-time"   },
		{ "busy_time",     "busy-time"     },
		{ "receive_time",  "receive-time"  },
		{ "transmit_time", "transmit-time" },
	};
	struct lyd_node *component, *chan;
	const char *radio, *passive;
	json_error_t jerr;
	json_t *root, *entry;
	size_t index;
	FILE *pp;

	if (event != SR_EV_RPC)
		return SR_ERR_OK;

	component = lyd_parent(lyd_parent(input));
	radio = component ? lydx_get_cattr(component, "name") : NULL;
	if (!radio)
		return rpc_failed(session, "Cannot tell which radio to survey");

	passive = lydx_get_cattr((struct lyd_node *)input, "passive");
	pp = popenf("r", "/usr/libexec/infix/iw.py survey-scan %s %s", radio,
		    passive && !strcmp(passive, "true") ? "passive" : "");
	if (!pp)
		return rpc_failed(session, "Failed starting channel survey");

	root = json_loadf(pp, 0, &jerr);
	pclose(pp);
	if (!root)
		return rpc_failed(session, "Channel survey returned no data");

	if (!json_is_array(root)) {
		json_t *err = json_object_get(root, "error");
		char msg[256];

		snprintf(msg, sizeof(msg), "Channel survey failed: %s",
			 json_is_string(err) ? json_string_value(err) : "unknown error");
		json_decref(root);
		return rpc_failed(session, msg);
	}

	json_array_foreach(root, index, entry) {
		json_t *freq = json_object_get(entry, "frequency");
		char val[32];

		if (!json_is_integer(freq))
			continue;

		snprintf(val, sizeof(val), "%lld", json_integer_value(freq));
		if (lyd_new_list(output, NULL, "channel", LYD_NEW_VAL_OUTPUT, &chan, val)) {
			ERROR("channel-survey: failed adding channel %s", val);
			continue;
		}

		lyd_new_term(chan, NULL, "in-use",
			     json_is_true(json_object_get(entry, "in_use")) ? "true" : "false",
			     0, NULL);

		for (size_t i = 0; i < NELEMS(times); i++) {
			json_t *v = json_object_get(entry, times[i].json);

			if (!json_is_integer(v))
				continue;

			snprintf(val, sizeof(val), "%lld", json_integer_value(v));
			lyd_new_term(chan, NULL, times[i].yang, val, 0, NULL);
		}
	}
	json_decref(root);

	return SR_ERR_OK;
}

static int wifi_scan(sr_session_ctx_t *session, uint32_t sub_id, const char *op_path,
		     const struct lyd_node *input, sr_event_t event, uint32_t request_id,
		     struct lyd_node *output, void *priv)
{
	static const struct { const char *json, *yang; } nums[] = {
		{ "frequency", "frequency"       },
		{ "channel",   "channel"         },
		{ "signal",    "signal-strength" },
	};
	struct lyd_node *component, *net;
	json_t *root, *entry;
	const char *radio;
	json_error_t jerr;
	size_t index;
	FILE *pp;

	if (event != SR_EV_RPC)
		return SR_ERR_OK;

	component = lyd_parent(lyd_parent(input));
	radio = component ? lydx_get_cattr(component, "name") : NULL;
	if (!radio)
		return rpc_failed(session, "Cannot tell which radio to scan");

	pp = popenf("r", "/usr/libexec/infix/iw.py scan %s", radio);
	if (!pp)
		return rpc_failed(session, "Failed starting scan");

	root = json_loadf(pp, 0, &jerr);
	pclose(pp);
	if (!root)
		return rpc_failed(session, "Scan returned no data");

	if (!json_is_array(root)) {
		json_t *err = json_object_get(root, "error");
		char msg[256];

		snprintf(msg, sizeof(msg), "Scan failed: %s",
			 json_is_string(err) ? json_string_value(err) : "unknown error");
		json_decref(root);
		return rpc_failed(session, msg);
	}

	json_array_foreach(root, index, entry) {
		json_t *bssid = json_object_get(entry, "bssid");
		json_t *val, *enc;
		char buf[32];
		size_t i;

		if (!json_is_string(bssid))
			continue;

		if (lyd_new_list(output, NULL, "network", LYD_NEW_VAL_OUTPUT, &net,
				 json_string_value(bssid))) {
			ERROR("scan: failed adding network %s", json_string_value(bssid));
			continue;
		}

		val = json_object_get(entry, "ssid");
		if (json_is_string(val))
			lyd_new_term(net, NULL, "ssid", json_string_value(val),
				     LYD_NEW_VAL_OUTPUT, NULL);

		for (i = 0; i < NELEMS(nums); i++) {
			val = json_object_get(entry, nums[i].json);
			if (!json_is_integer(val))
				continue;

			snprintf(buf, sizeof(buf), "%lld", json_integer_value(val));
			lyd_new_term(net, NULL, nums[i].yang, buf, LYD_NEW_VAL_OUTPUT, NULL);
		}

		enc = json_object_get(entry, "encryption");
		json_array_foreach(enc, i, val) {
			if (json_is_string(val))
				lyd_new_term(net, NULL, "encryption", json_string_value(val),
					     LYD_NEW_VAL_OUTPUT, NULL);
		}
	}
	json_decref(root);

	return SR_ERR_OK;
}

/* The actions only exist when the wifi feature is enabled, see wifi.inc */
static bool wifi_feature_enabled(struct confd *confd)
{
	const struct lys_module *mod;
	const struct ly_ctx *ctx;
	bool enabled = false;

	ctx = sr_acquire_context(confd->conn);
	if (!ctx)
		return false;

	mod = ly_ctx_get_module_implemented(ctx, "infix-hardware");
	if (mod)
		enabled = lys_feature_value(mod, "wifi") == LY_SUCCESS;
	sr_release_context(confd->conn);

	return enabled;
}

int hardware_rpc_init(struct confd *confd)
{
	int rc = 0;

	if (!wifi_feature_enabled(confd))
		return SR_ERR_OK;

	REGISTER_RPC_TREE(confd->session, XPATH_BASE_ "/component/infix-hardware:wifi-radio/channel-survey",
			  wifi_channel_survey, NULL, &confd->sub);
	REGISTER_RPC_TREE(confd->session, XPATH_BASE_ "/component/infix-hardware:wifi-radio/scan",
			  wifi_scan, NULL, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("Init hardware rpc failed: %s", sr_strerror(rc));
	return rc;
}
