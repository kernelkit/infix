/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_HARDWARE_H_
#define CONFD_HARDWARE_H_

#include "core.h"

/* hw-usb.c */
bool usb_authorize(struct json_t *root, const char *name, int enabled);

#endif /* CONFD_HARDWARE_H_ */
