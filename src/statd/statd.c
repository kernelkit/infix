/* SPDX-License-Identifier: BSD-3-Clause */

#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <sysrepo.h>
#include <ev.h>
#include <string.h>
#include <errno.h>

#include <asm/types.h>
#include <sys/socket.h>
#include <linux/netlink.h>
#include <linux/rtnetlink.h>

#include <jansson.h>
#include <ctype.h>
#include <linux/if.h>
#include <sys/queue.h>

#include <srx/common.h>
#include <srx/helpers.h>
#include <srx/lyx.h>

#include "shared.h"
#include "iface-ip-link.h"
#include "iface-ip-addr.h"
#include "iface-ethtool.h"

#define SOCK_RMEM_SIZE 1000000  /* Arbitrary chosen, default = 212992 */
#define NL_BUF_SIZE 4096 /* Arbitrary chosen */

TAILQ_HEAD(sub_head, sub);

/* This should, with some modifications, be able to hold other subscription
 * types, not only interfaces.
 */
struct sub {
	char ifname[IFNAMSIZ];
	struct ev_io watcher;
	sr_subscription_ctx_t *sr_sub;

	TAILQ_ENTRY(sub) entries;
};

struct netlink {
	int sd;
	struct ev_io watcher;
};

struct statd {
	struct netlink nl;
	struct ev_loop *ev_loop;
	struct sub_head subs;
	sr_session_ctx_t *sr_ses;
};

static void set_sock_rcvbuf(int sd, int size)
{
       if (setsockopt(sd, SOL_SOCKET, SO_RCVBUF, &size, sizeof(size)) < 0) {
               perror("setsockopt");
               return;
       }
       DEBUG("Socket receive buffer size increased to: %d bytes", size);
}

static int nl_sock_init(void)
{
	struct sockaddr_nl addr;
	int sock;

	sock = socket(PF_NETLINK, SOCK_RAW, NETLINK_ROUTE);
	if (sock < 0) {
		ERROR("Error, creating netlink socket: %s", strerror(errno));
		return -1;
	}

	memset(&addr, 0, sizeof(addr));
	addr.nl_family = AF_NETLINK;
	addr.nl_groups = RTMGRP_LINK;

	if (bind(sock, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
		ERROR("Error, binding netlink socket: %s", strerror(errno));
		close(sock);
		return -1;
	}

	return sock;
}

static struct sub *sub_find_iface(struct sub_head *subs, const char *ifname)
{
	struct sub *sub;

	TAILQ_FOREACH(sub, subs, entries) {
		if (strcmp(sub->ifname, ifname) == 0)
			return sub;
	}

	return NULL;
}

static void sub_delete(struct ev_loop *loop, struct sub_head *subs, struct sub *sub)
{
	TAILQ_REMOVE(subs, sub, entries);
	ev_io_stop(loop, &sub->watcher);
	sr_unsubscribe(sub->sr_sub);
	free(sub);
}

static json_t *json_get_ip_link(void)
{
	char cmd[512] = {}; /* Size is arbitrary */

	snprintf(cmd, sizeof(cmd), "ip -s -d -j link show 2>/dev/null");

	return json_get_output(cmd);
}

static int sr_ifaces_cb(sr_session_ctx_t *session, uint32_t, const char *path,
			const char *, const char *, uint32_t,
			struct lyd_node **parent, void *priv)
{
	struct sub *sub = priv;
	const struct ly_ctx *ctx;
	sr_conn_ctx_t *con;
	int err;

	DEBUG("Incoming query for xpath: %s", path);

	con = sr_session_get_connection(session);
	if (!con) {
		ERROR("Error, getting connection");
		return SR_ERR_INTERNAL;
	}

	ctx = sr_acquire_context(con);
	if (!ctx) {
		ERROR("Error, acquiring context");
		return SR_ERR_INTERNAL;
	}

	/* Skip internal interfaces (such as dsa0) */
	if (ip_link_check_group(sub->ifname, "internal") == 1) {
		err = SR_ERR_OK;
		goto out;
	}

	err = ly_add_ip_link(ctx, parent, sub->ifname);
	if (err) {
		ERROR("Error, adding ip link info");
		goto out;
	}

	err = ly_add_ip_addr(ctx, parent, sub->ifname);
	if (err) {
		ERROR("Error, adding ip addr info");
		goto out;
	}

	err = ly_add_ethtool(ctx, parent, sub->ifname);
	if (err)
		ERROR("Error, adding ethtool info");
out:
	sr_release_context(con);

	return err;
}

static void sigint_cb(struct ev_loop *loop, struct ev_signal *, int)
{
	ev_break(loop, EVBREAK_ALL);
}

static void sigusr1_cb(struct ev_loop *, struct ev_signal *, int)
{
	debug ^= 1;
}

static void sr_event_cb(struct ev_loop *, struct ev_io *w, int)
{
	struct sub *sub = (struct sub *)w->data;

	sr_subscription_process_events(sub->sr_sub, NULL, NULL);
}

static int sub_to_iface(struct statd *statd, const char *ifname)
{
	char path[XPATH_MAX] = {};
	struct sub *sub;
	int sr_ev_pipe;
	int err;

	sub = sub_find_iface(&statd->subs, ifname);
	if (sub) {
		DEBUG("Interface %s already subscribed", ifname);
		return SR_ERR_OK;
	}

	sub = malloc(sizeof(struct sub));
	if (!sub)
		return SR_ERR_INTERNAL;

	memset(sub, 0, sizeof(struct sub));

	snprintf(sub->ifname, sizeof(sub->ifname), "%s", ifname);
	snprintf(path, sizeof(path), "%s/interface[name='%s']",
		 XPATH_IFACE_BASE, ifname);

	DEBUG("Subscribe to events for \"%s\"", path);
	err = sr_oper_get_subscribe(statd->sr_ses, "ietf-interfaces",
				    path, sr_ifaces_cb, sub,
				    SR_SUBSCR_DEFAULT | SR_SUBSCR_NO_THREAD | SR_SUBSCR_DONE_ONLY,
				    &sub->sr_sub);
	if (err) {
		ERROR("Error, subscribing to path \"%s\": %s", path, sr_strerror(err));
		free(sub);
		return SR_ERR_INTERNAL;
	}

	err = sr_get_event_pipe(sub->sr_sub, &sr_ev_pipe);
	if (err) {
		ERROR("Error, getting sysrepo event pipe: %s", sr_strerror(err));
		sr_unsubscribe(sub->sr_sub);
		free(sub);
		return SR_ERR_INTERNAL;
	}

	TAILQ_INSERT_TAIL(&statd->subs, sub, entries);

	ev_io_init(&sub->watcher, sr_event_cb, sr_ev_pipe, EV_READ);
	sub->watcher.data = sub;
	ev_io_start(statd->ev_loop, &sub->watcher);

	return SR_ERR_OK;
}

static void unsub_to_ifaces(struct statd *statd)
{
	struct sub *sub;

	while (!TAILQ_EMPTY(&statd->subs)) {
		sub = TAILQ_FIRST(&statd->subs);
		DEBUG("Unsubscribe from \"%s\" (all)", sub->ifname);
		sub_delete(statd->ev_loop, &statd->subs, sub);
	}
}

static int unsub_to_iface(struct statd *statd, char *ifname)
{
	struct sub *sub;

	sub = sub_find_iface(&statd->subs, ifname);
	if (!sub) {
		ERROR("Error, can't find interface to delete (%s)", ifname);
		return SR_ERR_INTERNAL;
	}
	DEBUG("Unsubscribe from \"%s\"", sub->ifname);
	sub_delete(statd->ev_loop, &statd->subs, sub);

	return SR_ERR_OK;
}

static int nl_process_msg(struct nlmsghdr *nlh, struct statd *statd)
{
	struct ifinfomsg *iface;
	struct rtattr *attr;
	int attr_len;

	iface = NLMSG_DATA(nlh);
	attr = IFLA_RTA(iface);
	attr_len = IFLA_PAYLOAD(nlh);

	for (; RTA_OK(attr, attr_len); attr = RTA_NEXT(attr, attr_len)) {
		if (attr->rta_type == IFLA_IFNAME) {
			char *ifname = (char *)RTA_DATA(attr);

			if (nlh->nlmsg_type == RTM_NEWLINK)
				return sub_to_iface(statd, ifname);
			else if (nlh->nlmsg_type == RTM_DELLINK)
				return unsub_to_iface(statd, ifname);
			else
				return SR_ERR_INTERNAL;
		}
	}

	/* Ignore nl messages with no interface name */
	return SR_ERR_OK;
}

static void nl_event_cb(struct ev_loop *, struct ev_io *w, int)
{
	struct statd *statd = (struct statd *) w->data;
	char buf[NL_BUF_SIZE];
	struct nlmsghdr *nlh;
	int err;
	int len;

	len = recv(statd->nl.sd, buf, sizeof(buf), 0);
	if (len < 0) {
		ERROR("Error, netlink recv failed: %s", strerror(errno));
		close(statd->nl.sd);
		/* NOTE: This is likely caused by a full kernel buffer, which
		 * means we can't trust our list. So we exit hard and let finit
		 * respawn us to handle this.
		 */
		exit(EXIT_FAILURE);
	}

	for (nlh = (struct nlmsghdr *)buf; NLMSG_OK(nlh, len); nlh = NLMSG_NEXT(nlh, len)) {
		err = nl_process_msg(nlh, statd);
		if (err)
			ERROR("Error, processing netlink message: %s", sr_strerror(err));
	}
}

static int sub_to_ifaces(struct statd *statd)
{
	json_t *j_iface;
	json_t *j_root;
	size_t i;

	j_root = json_get_ip_link();
	if (!j_root) {
		ERROR("Error, parsing ip-link JSON");
		return SR_ERR_SYS;
	}

	json_array_foreach(j_root, i, j_iface) {
		json_t *j_ifname;
		int err;

		j_ifname = json_object_get(j_iface, "ifname");
		if (!json_is_string(j_ifname)) {
			ERROR("Got unexpected JSON type for 'ifname'");
			continue;
		}

		err = sub_to_iface(statd, json_string_value(j_ifname));
		if (err) {
			ERROR("Unable to subscribe to %s", json_string_value(j_ifname));
			continue;
		}
	}
	json_decref(j_root);

	return SR_ERR_OK;
}

int main(int argc, char *argv[])
{
	struct ev_signal sigint_watcher, sigusr1_watcher;
	struct statd statd = {};
	int log_opts = LOG_USER;
	sr_conn_ctx_t *sr_conn;
	int err;

	if (argc > 1 && !strcmp(argv[1], "-d")) {
		log_opts |= LOG_PERROR;
		debug = 1;
	}

	openlog("statd", log_opts, 0);

	TAILQ_INIT(&statd.subs);
	statd.ev_loop = EV_DEFAULT;

	statd.nl.sd = nl_sock_init();
	if (statd.nl.sd < 0) {
		ERROR("Error, opening netlink socket");
		return EXIT_FAILURE;
	}
	INFO("Status daemon starting");

	set_sock_rcvbuf(statd.nl.sd, SOCK_RMEM_SIZE);

	err = sr_connect(SR_CONN_DEFAULT, &sr_conn);
	if (err) {
		ERROR("Error, connecting to sysrepo: %s", sr_strerror(err));
		return EXIT_FAILURE;
	}
	DEBUG("Connected to sysrepo");

	err = sr_session_start(sr_conn, SR_DS_OPERATIONAL, &statd.sr_ses);
	if (err) {
		ERROR("Error, start sysrepo session: %s", sr_strerror(err));
		sr_disconnect(sr_conn);
		return EXIT_FAILURE;
	}
	DEBUG("Session started (%p)", statd.sr_ses);

	DEBUG("Attempting to register existing interfaces");
	err = sub_to_ifaces(&statd);
	if (err) {
		ERROR("Error, registering existing interfaces");
		sr_disconnect(sr_conn);
		return EXIT_FAILURE;
	}

	ev_signal_init(&sigint_watcher, sigint_cb, SIGINT);
	sigint_watcher.data = &statd;
	ev_signal_start(statd.ev_loop, &sigint_watcher);

	ev_signal_init(&sigusr1_watcher, sigusr1_cb, SIGUSR1);
	sigusr1_watcher.data = &statd;
	ev_signal_start(statd.ev_loop, &sigusr1_watcher);

	ev_io_init(&statd.nl.watcher, nl_event_cb, statd.nl.sd, EV_READ);
	statd.nl.watcher.data = &statd;
	ev_io_start(statd.ev_loop, &statd.nl.watcher);

	INFO("Status daemon entering main event loop");
	ev_run(statd.ev_loop, 0);

	/* We should never get here during normal operation */
	INFO("Status daemon shutting down");
	unsub_to_ifaces(&statd);
	sr_session_stop(statd.sr_ses);
	sr_disconnect(sr_conn);

	return EXIT_SUCCESS;
}
