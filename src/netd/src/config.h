/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef NETD_CONFIG_H_
#define NETD_CONFIG_H_

#include "netd.h"

int config_load(struct route_head *routes, struct rip_config *rip_cfg);

#endif /* NETD_CONFIG_H_ */
