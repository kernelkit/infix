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

#include "common.h"
#include "helpers.h"
#include "lyx.h"

#define XPATH_MAX PATH_MAX
#define XPATH_IFACE_BASE "/ietf-interfaces:interfaces"

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


static json_t *json_get_ip_link(char *ifname)
{
	char cmd[512] = {}; /* Size is arbitrary */
	json_error_t j_err;
	json_t *j_root;
	FILE *proc;

	if (ifname)
		snprintf(cmd, sizeof(cmd), "ip -s -d -j link show dev %s 2>/dev/null", ifname);
	else
		snprintf(cmd, sizeof(cmd), "ip -s -d -j link show 2>/dev/null");

	proc = popenf("re", cmd);
	if (!proc) {
		ERROR("Error, running ip link command");
		return NULL;
	}

	j_root = json_loadf(proc, 0, &j_err);
	pclose(proc);
	if (!j_root) {
		ERROR("Error, parsing ip link JSON");
		return NULL;
	}

	if (!json_is_array(j_root)) {
		ERROR("Expected a JSON array from ip link");
		json_decref(j_root);
		return NULL;
	}

	return j_root;
}

static const char *get_yang_operstate(const char *operstate)
{
	size_t i;
	struct {
		const char *kern;
		const char *yang;

	} map[] = {
		{"DOWN",                "down"},
		{"UP",                  "up"},
		{"DORMANT",             "dormant"},
		{"TESTING",             "testing"},
		{"LOWERLAYERDOWN",      "lower-layer-down"},
		{"NOTPRESENT",          "not-present"},
	};

	for (i = 0; i < sizeof(map) / sizeof(map[0]); i++) {
		if (strcmp(operstate, map[i].kern) != 0)
			continue;

		return map[i].yang;
	}

	return "unknown";
}

static int ly_add_ip_link_stat(const struct ly_ctx *ctx, struct lyd_node **parent,
			       char *xpath, json_t *j_iface)
{
	struct lyd_node *node = NULL;
	char *stat_xpath;
	json_t *j_stat;
	json_t *j_val;
	int err;

	err = asprintf(&stat_xpath, "%s/statistics", xpath);
	if (err == -1) {
		ERROR("Error, creating statistics xpath");
		return SR_ERR_SYS;
	}

	j_stat = json_object_get(j_iface, "stats64");
	if (!j_stat) {
		ERROR("Didn't find 'stats64' in JSON data");
		goto err_out;
	}

	err = lyd_new_path(*parent, ctx, stat_xpath, NULL, 0, &node);
	if (err) {
		ERROR("Failed adding 'statistics' node, libyang error %d: %s",
		      err, ly_errmsg(ctx));
		goto err_out;
	}

	j_val = json_object_get(j_stat, "tx");
	if (!j_val) {
		ERROR("Didn't find 'tx' in JSON stats64 data");
		goto err_out;
	}

	j_val = json_object_get(j_val, "bytes");
	if (!j_val) {
		ERROR("Didn't find 'bytes' in JSON stats64 tx data");
		goto err_out;
	}
	if (!json_is_integer(j_val)) {
		ERROR("Didn't get integer for 'bytes' in JSON stats64 tx data");
		goto err_out;
	}
	err = lydx_new_path(ctx, &node, stat_xpath, "out-octets", "%lld",
			    json_integer_value(j_val));
	if (err) {
		ERROR("Error, adding 'out-octets' to data tree, libyang error %d", err);
		goto err_out;
	}

	j_val = json_object_get(j_stat, "rx");
	if (!j_val) {
		ERROR("Didn't find 'rx' in JSON stats64 data");
		goto err_out;
	}

	j_val = json_object_get(j_val, "bytes");
	if (!j_val) {
		ERROR("Didn't find 'bytes' in JSON stats64 rx data");
		goto err_out;
	}
	if (!json_is_integer(j_val)) {
		ERROR("Didn't get integer for 'bytes' in JSON stats64 rx data");
		goto err_out;
	}
	err = lydx_new_path(ctx, &node, stat_xpath, "in-octets", "%lld",
			    json_integer_value(j_val));
	if (err) {
		ERROR("Error, adding 'in-octets' to data tree, libyang error %d", err);
		goto err_out;
	}

	free(stat_xpath);

	return SR_ERR_OK;

err_out:
	free(stat_xpath);

	return SR_ERR_SYS;
}

static const char *get_yang_link_type(char *xpath, json_t *iface)
{
	json_t *j_type;
	const char *type;

	j_type = json_object_get(iface, "link_type");
	if (!json_is_string(j_type)) {
		ERROR("Expected a JSON string for 'link_type'");
		/* This will throw a YANG / ly error */
		return "";
	}

	type = json_string_value(j_type);

	if (strcmp(type, "ether") == 0) {
		const char *kind;
		json_t *j_val;

		j_val = json_object_get(iface, "linkinfo");
		if (!j_val)
			return "iana-if-type:ethernetCsmacd";

		j_val = json_object_get(j_val, "info_kind");
		if (!j_val)
			return "iana-if-type:ethernetCsmacd";

		if (!json_is_string(j_val)) {
			ERROR("Expected a JSON string for 'info_kind'");
			return "iana-if-type:other";
		}
		kind = json_string_value(j_val);

		if (strcmp(kind, "veth") == 0)
			return "infix-if-type:veth";
		if (strcmp(kind, "vlan") == 0)
			return "iana-if-type:l2vlan";
		if (strcmp(kind, "bridge") == 0)
			return "iana-if-type:bridge";

		/**
		 * We could return ethernetCsmacd here, but it might hide some
		 * special type that we actually want to explicitly identify.
		 */
		ERROR("Unable to determine info_kind for \"%s\"", xpath);

		return "iana-if-type:other";
	}

	if (strcmp(type, "loopback") == 0)
		return "iana-if-type:softwareLoopback";

	ERROR("Unable to determine iana-if-type for \"%s\"", xpath);

	return "iana-if-type:other";
}

static int ly_add_ip_link_data(const struct ly_ctx *ctx, struct lyd_node **parent,
			       char *xpath, json_t *iface)
{
	const char *val;
	const char *type;
	json_t *j_val;
	int err;

	j_val = json_object_get(iface, "ifindex");
	if (!json_is_integer(j_val)) {
		ERROR("Expected a JSON integer for 'ifindex'");
		return SR_ERR_SYS;
	}

	err = lydx_new_path(ctx, parent, xpath, "if-index", "%lld",
			    json_integer_value(j_val));
	if (err) {
		ERROR("Error, adding 'if-index' to data tree, libyang error %d", err);
		return SR_ERR_LY;
	}

	j_val = json_object_get(iface, "operstate");
	if (!json_is_string(j_val)) {
		ERROR("Expected a JSON string for 'operstate'");
		return SR_ERR_SYS;
	}

	val = get_yang_operstate(json_string_value(j_val));
	err = lydx_new_path(ctx, parent, xpath, "oper-status", val);
	if (err) {
		ERROR("Error, adding 'oper-status' to data tree, libyang error %d", err);
		return SR_ERR_LY;
	}

	err = ly_add_ip_link_stat(ctx, parent, xpath, iface);
	if (err) {
		ERROR("Error, adding 'stats64' to data tree");
		return err;
	}

	type = get_yang_link_type(xpath, iface);
	err = lydx_new_path(ctx, parent, xpath, "type", type);
	if (err) {
		ERROR("Error, adding 'type' to data tree, libyang error %d", err);
		return SR_ERR_LY;
	}

	return SR_ERR_OK;
}

static int ly_add_ip_link(const struct ly_ctx *ctx, struct lyd_node **parent, char *ifname)
{
	char xpath[XPATH_MAX] = {};
	json_t *j_root;
	json_t *j_iface;
	int err;

	j_root = json_get_ip_link(ifname);
	if (!j_root) {
		ERROR("Error, parsing ip-link JSON");
		return SR_ERR_SYS;
	}
	if (json_array_size(j_root) != 1) {
		ERROR("Error, expected JSON array of single iface");
		json_decref(j_root);
		return SR_ERR_SYS;
	}

	j_iface = json_array_get(j_root, 0);

	snprintf(xpath, sizeof(xpath), "%s/interface[name='%s']",
		 XPATH_IFACE_BASE, ifname);

	err = ly_add_ip_link_data(ctx, parent, xpath, j_iface);
	if (err) {
		ERROR("Error, adding ip-link info for %s", ifname);
		json_decref(j_root);
		return err;
	}
	json_decref(j_root);

	return SR_ERR_OK;
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

	err = ly_add_ip_link(ctx, parent, sub->ifname);
	if (err)
		ERROR("Error, adding ip link info");

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

	j_root = json_get_ip_link(NULL);
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
