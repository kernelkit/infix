/* SPDX-License-Identifier: BSD-3-Clause */

#include "route.h"

void route_list_free(struct route_head *head)
{
	struct route *r, *tmp;

	TAILQ_FOREACH_SAFE(r, head, entries, tmp) {
		TAILQ_REMOVE(head, r, entries);
		free(r);
	}
}
