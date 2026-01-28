/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef NETD_JSON_BUILDER_H_
#define NETD_JSON_BUILDER_H_

#include "netd.h"

/*
 * Build JSON configuration for FRR routing daemons.
 * Returns pointer to static buffer containing JSON string.
 * Buffer is valid until next call to any build_*_json function.
 */

const char *build_staticd_json(struct route_head *routes);
const char *build_rip_json(struct rip_config *rip_cfg);
const char *build_routing_json(struct route_head *routes, struct rip_config *rip_cfg);

#endif /* NETD_JSON_BUILDER_H_ */
