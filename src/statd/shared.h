/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef STATD_SHARED_H_
#define STATD_SHARED_H_

#include <stddef.h>
#include <time.h>
#include <jansson.h>

json_t *json_get_output(const char *cmd);
int ip_link_check_group(const char *ifname, const char *group);
void format_timestamp(time_t when, char *buf, size_t sz);

#endif
