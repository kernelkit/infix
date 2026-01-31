/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef NETD_ROUTE_H_
#define NETD_ROUTE_H_

#include "netd.h"

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

void route_list_free(struct route_head *head);

#endif /* NETD_ROUTE_H_ */
