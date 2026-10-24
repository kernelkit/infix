/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef STATD_SHARED_H_
#define STATD_SHARED_H_

#include <stddef.h>
#include <time.h>

void format_timestamp(time_t when, char *buf, size_t sz);

#endif
