/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_HARDWARE_H_
#define CONFD_HARDWARE_H_

#include "core.h"

#define XPATH_BASE_              "/ietf-hardware:hardware"

/* hw-usb.c */
bool usb_authorize(struct json_t *root, const char *name, int enabled);

/* hw-wifi-ap.c */
void wifi_gen_ssid_config(FILE *hostapd, struct lyd_node *cif, struct lyd_node *config, bool is_bss,
			  const char *band, struct confd *confd);

/* hw-wifi.c */
const char *wifi_country_code(struct lyd_node *config);
int wifi_find_interfaces_on_radio(struct lyd_node *ifs, const char *radio_name,
				  struct lyd_node ***iface_list, int *count);
int wifi_gen_aps_on_radio(const char *radio_name, struct lyd_node *cifs,
			  struct lyd_node *radio_node, struct lyd_node *config,
			  struct confd *confd);

#endif /* CONFD_HARDWARE_H_ */
