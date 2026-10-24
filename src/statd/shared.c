/* SPDX-License-Identifier: BSD-3-Clause */

#include <time.h>

#include "shared.h"

/* YANG date-and-time, always in UTC */
void format_timestamp(time_t when, char *buf, size_t sz)
{
	struct tm tm;

	gmtime_r(&when, &tm);
	strftime(buf, sz, "%Y-%m-%dT%H:%M:%S+00:00", &tm);
}
