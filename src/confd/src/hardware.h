/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_HARDWARE_H_
#define CONFD_HARDWARE_H_

#include "core.h"

#define XPATH_BASE_              "/ietf-hardware:hardware"

/* hw-usb.c */
int usb_change(struct lyd_node *cif, const char *name, sr_event_t event, struct confd *confd);

/* hw-wifi-ap.c */
void wifi_gen_ssid_config(FILE *hostapd, struct lyd_node *cif, struct lyd_node *config, bool is_bss,
			  const char *band, struct confd *confd);

/* hw-wifi.c */
void wifi_country_change(struct lyd_node *config, struct lyd_node *diff, sr_event_t event);
int wifi_radio_change(struct lyd_node *config, struct lyd_node *cif, sr_event_t event, struct confd *confd);

/* hw-modem.c */
const char *modem_usb_port(struct lyd_node *cif, char *buf, size_t len);
int modem_wwan_diff(struct lyd_node *diff);
int modem_rule_port(const char *path, char *buf, size_t len);
void modem_trigger_port(const char *port);
int modem_port_find(char (*ports)[64], int num, const char *port);
int modem_change(struct lyd_node *config, sr_event_t event, bool changed);

/* hw-gps.c */
int gps_change(struct lyd_node *config, sr_event_t event);

#endif /* CONFD_HARDWARE_H_ */
