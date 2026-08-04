/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef NETD_VTYSH_BACKEND_H_
#define NETD_VTYSH_BACKEND_H_

#include "netd.h"

int  vtysh_backend_init(void);
void vtysh_backend_cleanup(void);
int  vtysh_backend_apply(struct route_head *routes, struct rip_config *rip,
			 struct rip_config *ripng);

#endif /* NETD_VTYSH_BACKEND_H_ */
