/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_HARDWARE_H_
#define CONFD_HARDWARE_H_

#include "core.h"

/* hw-usb.c */
bool usb_authorize(struct json_t *root, const char *name, int enabled);

/* hw-wifi-ap.c */
void wifi_gen_ssid_config(FILE *hostapd, struct lyd_node *cif, struct lyd_node *config, bool is_bss,
			  const char *band, struct confd *confd);

#endif /* CONFD_HARDWARE_H_ */
