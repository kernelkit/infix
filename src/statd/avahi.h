/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef STATD_AVAHI_H_
#define STATD_AVAHI_H_

#include <stdint.h>
#include <sys/queue.h>

#include <ev.h>
#include <sysrepo.h>
#include <avahi-client/client.h>
#include <avahi-client/lookup.h>
#include <avahi-common/watch.h>

/*
 * In-memory state for avahi mDNS neighbor tracking.
 * Services are kept in a flat list; neighbors track addresses only.
 */

struct avahi_addr {
	char val[64];
	LIST_ENTRY(avahi_addr) link;
};

struct avahi_txt {
	char val[256];
	LIST_ENTRY(avahi_txt) link;
};

struct avahi_service {
	int           ifindex;
	AvahiProtocol proto;
	char          name[256];
	char          type[64];
	char          domain[64];
	char          hostname[256];
	uint16_t      port;
	LIST_HEAD(, avahi_txt) txts;
	LIST_ENTRY(avahi_service) link;
};

struct avahi_neighbor {
	char hostname[256];
	LIST_HEAD(, avahi_addr) addrs;
	LIST_ENTRY(avahi_neighbor) link;
};

struct avahi_type_entry {
	AvahiServiceBrowser *browser;
	char type[64];
	LIST_ENTRY(avahi_type_entry) link;
};

struct avahi_ctx {
	struct ev_loop          *loop;
	sr_conn_ctx_t           *sr_conn;      /* Connection for running-DS config queries */
	sr_session_ctx_t        *sr_ses;       /* Dedicated operational DS write session */
	AvahiClient             *client;
	AvahiServiceTypeBrowser *type_browser;
	AvahiPoll                poll_api;     /* libev-backed vtable */
	unsigned int             fail_count;   /* Consecutive avahi-daemon connection failures */
	ev_timer                 retry_timer;  /* Deferred error-log timer */
	LIST_HEAD(, avahi_neighbor)   neighbors;
	LIST_HEAD(, avahi_service)    services;    /* Flat list; keyed by 5-tuple */
	LIST_HEAD(, avahi_type_entry) type_entries;
};

int  avahi_ctx_init(struct avahi_ctx *ctx, struct ev_loop *loop, sr_conn_ctx_t *sr_conn);
void avahi_ctx_exit(struct avahi_ctx *ctx);

#endif
