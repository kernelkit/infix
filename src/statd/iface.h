/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef STATD_IFACE_H_
#define STATD_IFACE_H_

#include <stdint.h>
#include <time.h>
#include <sys/queue.h>
#include <linux/if.h>

#include <ev.h>
#include <libyang/libyang.h>

/*
 * In-memory link state, keyed by interface name, tracked over rtnetlink.
 * Renames are recognized by ifindex, which the kernel keeps across them.
 */

struct iface {
	char     name[IFNAMSIZ];
	int      ifindex;
	uint8_t  operstate;	/* IF_OPER_* */
	time_t   changed;	/* CLOCK_MONOTONIC, 0: state predates statd */
	TAILQ_ENTRY(iface) entries;
};

struct iface_ctx {
	ev_io           io;	/* MUST be first (cast from ev_io *) */
	struct ev_loop *loop;
	int             sd;
	uint32_t        seq;	/* sequence of ongoing dump, 0: none */
	TAILQ_HEAD(, iface) ifaces;
};

int  iface_ctx_init(struct iface_ctx *ctx, struct ev_loop *loop);
void iface_ctx_exit(struct iface_ctx *ctx);

void iface_annotate(struct iface_ctx *ctx, struct lyd_node *tree);

#endif
