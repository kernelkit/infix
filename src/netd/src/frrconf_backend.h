/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef NETD_FRRCONF_BACKEND_H_
#define NETD_FRRCONF_BACKEND_H_

#include "netd.h"

int  frrconf_backend_init(void);
void frrconf_backend_cleanup(void);
int  frrconf_backend_apply(struct route_head *routes, struct rip_config *rip,
			   struct rip_config *ripng);

#endif /* NETD_FRRCONF_BACKEND_H_ */
