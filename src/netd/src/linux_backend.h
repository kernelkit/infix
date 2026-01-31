/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef NETD_LINUX_BACKEND_H_
#define NETD_LINUX_BACKEND_H_

#include "route.h"
#include "rip.h"

int  linux_backend_init(void);
void linux_backend_fini(void);
int  linux_backend_apply(struct route_head *routes, struct rip_config *rip);

/* Internal netlink operations */
int netlink_route_add(const struct route *r);
int netlink_route_del(const struct route *r);

#endif /* NETD_LINUX_BACKEND_H_ */
