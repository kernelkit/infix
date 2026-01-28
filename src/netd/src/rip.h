/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef NETD_RIP_H_
#define NETD_RIP_H_

#include "netd.h"

/* RIP network/interface configuration */
struct rip_network {
	char ifname[IFNAMSIZ];
	int  passive;                /* Is this interface passive? */
	TAILQ_ENTRY(rip_network) entries;
};

/* RIP neighbor configuration */
struct rip_neighbor {
	struct in_addr addr;
	TAILQ_ENTRY(rip_neighbor) entries;
};

/* RIP redistribution configuration */
enum rip_redist_type {
	RIP_REDIST_CONNECTED,
	RIP_REDIST_STATIC,
	RIP_REDIST_KERNEL,
	RIP_REDIST_OSPF,
};

struct rip_redistribute {
	enum rip_redist_type type;
	TAILQ_ENTRY(rip_redistribute) entries;
};

/* System commands to execute after applying config */
#define RIP_SYSTEM_CMD_MAX 256
struct rip_system_cmd {
	char command[RIP_SYSTEM_CMD_MAX];
	TAILQ_ENTRY(rip_system_cmd) entries;
};

/* RIP timers */
struct rip_timers {
	uint32_t update;             /* Update interval (default 30) */
	uint32_t invalid;            /* Invalid interval (default 180) */
	uint32_t flush;              /* Flush interval (default 240) */
};

/* Main RIP configuration */
struct rip_config {
	int      enabled;            /* Is RIP enabled? */
	uint8_t  default_metric;     /* Default metric (default 1) */
	uint8_t  distance;           /* Administrative distance (default 120) */
	int      default_route;      /* Originate default route? */
	int      debug_events;       /* Enable RIP events debug? */
	int      debug_packet;       /* Enable RIP packet debug? */
	int      debug_kernel;       /* Enable kernel routing debug? */

	struct rip_timers timers;

	TAILQ_HEAD(, rip_network) networks;
	TAILQ_HEAD(, rip_neighbor) neighbors;
	TAILQ_HEAD(, rip_redistribute) redistributes;
	TAILQ_HEAD(, rip_system_cmd) system_cmds;
};

void rip_config_init(struct rip_config *cfg);
void rip_config_free(struct rip_config *cfg);

#endif /* NETD_RIP_H_ */
