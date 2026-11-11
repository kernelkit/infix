/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_HARDWARE_H_
#define CONFD_HARDWARE_H_

#include "core.h"

#define XPATH_BASE_              "/ietf-hardware:hardware"

#define USB_PORTS_MAX            32

/* USB ports named in generated udev rules */
struct usb_ports {
	char port[USB_PORTS_MAX][64];
	int  num;
};

/* hw-usb.c */
int usb_change(struct lyd_node *cif, const char *name, sr_event_t event, struct confd *confd);
const char *usb_uri_port(struct lyd_node *cif, char *buf, size_t len);
bool usb_ports_has(const struct usb_ports *ports, const char *port);
void usb_ports_add(struct usb_ports *ports, const char *port);
void usb_ports_trigger(const struct usb_ports *ports);
void usb_rules_purge(const char *pattern, struct usb_ports *ports);

/* hw-wifi-ap.c */
void wifi_gen_ssid_config(FILE *hostapd, struct lyd_node *cif, struct lyd_node *config, bool is_bss,
			  const char *band, struct confd *confd);

/* hw-wifi.c */
void wifi_country_change(struct lyd_node *config, struct lyd_node *diff, sr_event_t event);
int wifi_radio_change(struct lyd_node *config, struct lyd_node *cif, sr_event_t event, struct confd *confd);

/* hw-modem.c */
int modem_wwan_diff(struct lyd_node *diff);
int modem_change(struct lyd_node *config, sr_event_t event, bool changed);

/* hw-gps.c */
int gps_change(struct lyd_node *config, sr_event_t event);

#endif /* CONFD_HARDWARE_H_ */
