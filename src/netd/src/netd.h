/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef NETD_H_
#define NETD_H_

#include <arpa/inet.h>
#include <net/if.h>
#include <netinet/in.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <syslog.h>
#include <unistd.h>
#include <libite/queue.h>

extern int debug;

#define LOG(level, fmt, args...) syslog(level, fmt, ##args)
#define ERROR(fmt, args...)      LOG(LOG_ERR, fmt, ##args)
#define INFO(fmt, args...)       LOG(LOG_INFO, fmt, ##args)
#define DEBUG(fmt, args...)      do { if (debug) LOG(LOG_DEBUG, fmt, ##args); } while (0)

#define CONF_DIR    "/etc/netd/conf.d"

/* Nexthop types */
enum nh_type {
	NH_IFNAME,    /* Nexthop is interface name */
	NH_ADDR,      /* Nexthop is IP address */
	NH_BLACKHOLE, /* Blackhole route */
};

/* Blackhole subtypes */
enum bh_type {
	BH_NULL,      /* Null0 interface */
	BH_REJECT,    /* ICMP unreachable */
	BH_DROP,      /* Silent drop */
};

/* Static route entry */
struct route {
	int      family;         /* AF_INET or AF_INET6 */
	uint8_t  prefixlen;      /* Prefix length */
	uint8_t  distance;       /* Administrative distance */
	uint32_t tag;            /* Route tag */

	union {
		struct in_addr  ip4;
		struct in6_addr ip6;
	} prefix;

	enum nh_type nh_type;
	enum bh_type bh_type;    /* For NH_BLACKHOLE */

	union {
		struct in_addr  gw4;
		struct in6_addr gw6;
	} gateway;               /* For NH_ADDR */

	char ifname[IFNAMSIZ];   /* For NH_IFNAME */

	TAILQ_ENTRY(route) entries;
};

/* Route list head */
TAILQ_HEAD(route_head, route);

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

#endif /* NETD_H_ */
