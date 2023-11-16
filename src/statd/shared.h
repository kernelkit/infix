/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef STATD_SHARED_H_
#define STATD_SHARED_H_

#include <jansson.h>

json_t *json_get_output(const char *cmd);
int ip_link_check_group(const char *ifname, const char *group);

#endif
