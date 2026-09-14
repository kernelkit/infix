/* SPDX-License-Identifier: BSD-3-Clause */
/*
 * Track interface state changes over rtnetlink to provide the
 * ietf-interfaces last-change leaf, see issue #514.
 */

#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>
#include <sys/socket.h>
#include <linux/netlink.h>
#include <linux/rtnetlink.h>

#include <srx/common.h>
#include <srx/lyx.h>

#include "iface.h"
#include "shared.h"

static time_t monotonic(void)
{
	struct timespec ts;

	clock_gettime(CLOCK_MONOTONIC, &ts);
	return ts.tv_sec;
}

static struct iface *iface_find(struct iface_ctx *ctx, const char *name)
{
	struct iface *l;

	TAILQ_FOREACH(l, &ctx->ifaces, entries) {
		if (!strcmp(l->name, name))
			return l;
	}

	return NULL;
}

static struct iface *iface_find_index(struct iface_ctx *ctx, int ifindex)
{
	struct iface *l;

	TAILQ_FOREACH(l, &ctx->ifaces, entries) {
		if (l->ifindex == ifindex)
			return l;
	}

	return NULL;
}

static void iface_del(struct iface_ctx *ctx, struct iface *l)
{
	TAILQ_REMOVE(&ctx->ifaces, l, entries);
	free(l);
}

static void iface_update(struct iface_ctx *ctx, int ifindex, const char *name,
			uint8_t operstate, int dump)
{
	struct iface *l;

	l = iface_find(ctx, name);
	if (!l) {
		l = iface_find_index(ctx, ifindex);
		if (l) {
			DEBUG("Link %s renamed %s", l->name, name);
			snprintf(l->name, sizeof(l->name), "%s", name);
		}
	}

	if (!l) {
		l = calloc(1, sizeof(*l));
		if (!l) {
			ERRNO("Failed allocating link %s", name);
			return;
		}

		snprintf(l->name, sizeof(l->name), "%s", name);
		l->operstate = operstate;
		if (!dump)
			l->changed = monotonic();
		TAILQ_INSERT_TAIL(&ctx->ifaces, l, entries);
		DEBUG("Link %s added, operstate %u", name, operstate);
	} else if (l->operstate != operstate) {
		DEBUG("Link %s operstate %u -> %u", name, l->operstate, operstate);
		l->operstate = operstate;
		l->changed = monotonic();
	}

	l->ifindex = ifindex;
}

static void iface_parse(struct iface_ctx *ctx, struct nlmsghdr *nlh, int dump)
{
	struct ifinfomsg *ifi = NLMSG_DATA(nlh);
	int len = nlh->nlmsg_len - NLMSG_LENGTH(sizeof(*ifi));
	uint8_t operstate = IF_OPER_UNKNOWN;
	const char *name = NULL;
	struct rtattr *rta;
	struct iface *l;

	for (rta = IFLA_RTA(ifi); RTA_OK(rta, len); rta = RTA_NEXT(rta, len)) {
		switch (rta->rta_type & NLA_TYPE_MASK) {
		case IFLA_IFNAME:
			name = RTA_DATA(rta);
			break;
		case IFLA_OPERSTATE:
			operstate = *(uint8_t *)RTA_DATA(rta);
			break;
		}
	}

	if (!name)
		return;

	if (nlh->nlmsg_type == RTM_DELLINK) {
		l = iface_find(ctx, name);
		if (l) {
			DEBUG("Link %s removed", name);
			iface_del(ctx, l);
		}
		return;
	}

	iface_update(ctx, ifi->ifi_index, name, operstate, dump);
}

static void iface_purge(struct iface_ctx *ctx)
{
	struct iface *l;

	while ((l = TAILQ_FIRST(&ctx->ifaces)))
		iface_del(ctx, l);
}

/* Request a full link dump, replies are told apart from events by sequence */
static int iface_dump(struct iface_ctx *ctx)
{
	struct {
		struct nlmsghdr  nlh;
		struct ifinfomsg ifi;
	} req = {
		.nlh = {
			.nlmsg_len   = NLMSG_LENGTH(sizeof(struct ifinfomsg)),
			.nlmsg_type  = RTM_GETLINK,
			.nlmsg_flags = NLM_F_REQUEST | NLM_F_DUMP,
		},
		.ifi = {
			.ifi_family = AF_UNSPEC,
		},
	};

	if (!++ctx->seq)
		ctx->seq = 1;
	req.nlh.nlmsg_seq = ctx->seq;

	if (send(ctx->sd, &req, req.nlh.nlmsg_len, 0) < 0) {
		ERRNO("Failed requesting link dump");
		ctx->seq = 0;
		return -1;
	}

	return 0;
}

static void iface_io_cb(struct ev_loop *, ev_io *w, int)
{
	struct iface_ctx *ctx = (struct iface_ctx *)w;
	static char buf[32768];
	struct nlmsghdr *nlh;
	int len;

	for (;;) {
		len = recv(ctx->sd, buf, sizeof(buf), MSG_DONTWAIT);
		if (len < 0) {
			if (errno == EINTR)
				continue;
			if (errno == EAGAIN)
				break;
			if (errno == ENOBUFS) {
				WARN("Link event overrun, resyncing");
				iface_purge(ctx);
				iface_dump(ctx);
				continue;
			}

			ERRNO("Failed reading link events");
			break;
		}

		for (nlh = (struct nlmsghdr *)buf; NLMSG_OK(nlh, len); nlh = NLMSG_NEXT(nlh, len)) {
			int dump = ctx->seq && nlh->nlmsg_seq == ctx->seq;

			switch (nlh->nlmsg_type) {
			case RTM_NEWLINK:
			case RTM_DELLINK:
				iface_parse(ctx, nlh, dump);
				break;
			case NLMSG_DONE:
				if (dump)
					ctx->seq = 0;
				break;
			case NLMSG_ERROR:
				if (dump) {
					struct nlmsgerr *e = NLMSG_DATA(nlh);

					errno = -e->error;
					ERRNO("Link dump failed");
					ctx->seq = 0;
				}
				break;
			default:
				break;
			}
		}
	}
}

int iface_ctx_init(struct iface_ctx *ctx, struct ev_loop *loop)
{
	struct sockaddr_nl sa = {
		.nl_family = AF_NETLINK,
		.nl_groups = RTMGRP_LINK,
	};

	TAILQ_INIT(&ctx->ifaces);
	ctx->loop = loop;

	ctx->sd = socket(AF_NETLINK, SOCK_RAW | SOCK_CLOEXEC | SOCK_NONBLOCK, NETLINK_ROUTE);
	if (ctx->sd < 0) {
		ERRNO("Failed opening link event socket");
		return -1;
	}

	if (bind(ctx->sd, (struct sockaddr *)&sa, sizeof(sa)) < 0) {
		ERRNO("Failed subscribing to link events");
		close(ctx->sd);
		ctx->sd = -1;
		return -1;
	}

	ev_io_init(&ctx->io, iface_io_cb, ctx->sd, EV_READ);
	ev_io_start(loop, &ctx->io);

	return iface_dump(ctx);
}

void iface_ctx_exit(struct iface_ctx *ctx)
{
	if (ctx->sd >= 0) {
		ev_io_stop(ctx->loop, &ctx->io);
		close(ctx->sd);
		ctx->sd = -1;
	}

	iface_purge(ctx);
}

/* Add last-change to every interface in tree whose state changed since start */
void iface_annotate(struct iface_ctx *ctx, struct lyd_node *tree)
{
	time_t now = time(NULL), mono = monotonic();
	struct lyd_node *iface;

	if (!tree)
		return;

	LY_LIST_FOR(lyd_child(tree), iface) {
		struct iface *l;
		char buf[32];

		l = iface_find(ctx, lydx_get_cattr(iface, "name") ?: "");
		if (!l || !l->changed)
			continue;

		format_timestamp(now - (mono - l->changed), buf, sizeof(buf));
		if (lyd_new_term(iface, NULL, "last-change", buf, 0, NULL))
			WARN("Failed adding last-change to interface %s", l->name);
	}
}
