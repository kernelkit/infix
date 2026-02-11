/* SPDX-License-Identifier: BSD-3-Clause */

/*
 * Linux kernel backend for netd - sets routes directly in kernel
 * via rtnetlink. Does not use FRR.
 */

#include <errno.h>
#include <string.h>
#include <unistd.h>
#include <sys/socket.h>
#include <linux/netlink.h>
#include <linux/rtnetlink.h>
#include <net/if.h>

#include "netd.h"
#include "linux_backend.h"

static int nl_sock = -1;
static uint32_t nl_seq = 0;

/* Helper: add rtnetlink attribute */
static void rta_add(struct nlmsghdr *nlh, size_t maxlen, int type, const void *data, int len)
{
	size_t rtalen = RTA_LENGTH(len);
	struct rtattr *rta;

	if (NLMSG_ALIGN(nlh->nlmsg_len) + RTA_ALIGN(rtalen) > maxlen) {
		ERROR("rtnetlink: attribute overflow");
		return;
	}

	rta = (struct rtattr *)(((char *)nlh) + NLMSG_ALIGN(nlh->nlmsg_len));
	rta->rta_type = type;
	rta->rta_len = rtalen;
	if (len)
		memcpy(RTA_DATA(rta), data, len);
	nlh->nlmsg_len = NLMSG_ALIGN(nlh->nlmsg_len) + RTA_ALIGN(rtalen);
}

/* Send netlink message and wait for ACK */
static int nl_talk(struct nlmsghdr *nlh)
{
	struct iovec iov = { .iov_base = nlh, .iov_len = nlh->nlmsg_len };
	struct sockaddr_nl sa = { .nl_family = AF_NETLINK };
	struct msghdr msg = {
		.msg_name = &sa,
		.msg_namelen = sizeof(sa),
		.msg_iov = &iov,
		.msg_iovlen = 1,
	};
	struct nlmsgerr *err;
	struct nlmsghdr *h;
	char buf[4096];
	int ret;

	/* Send request */
	ret = sendmsg(nl_sock, &msg, 0);
	if (ret < 0) {
		ERROR("netlink sendmsg: %s", strerror(errno));
		return -1;
	}

	/* Receive ACK */
	iov.iov_base = buf;
	iov.iov_len = sizeof(buf);
	ret = recvmsg(nl_sock, &msg, 0);
	if (ret < 0) {
		ERROR("netlink recvmsg: %s", strerror(errno));
		return -1;
	}

	/* Parse ACK */
	h = (struct nlmsghdr *)buf;
	if (h->nlmsg_type == NLMSG_ERROR) {
		err = (struct nlmsgerr *)NLMSG_DATA(h);
		if (err->error) {
			errno = -err->error;
			return -1;
		}
		return 0; /* Success */
	}

	ERROR("netlink: unexpected response type %d", h->nlmsg_type);
	return -1;
}

/* Add or delete a route */
static int netlink_route_op(const struct route *r, int cmd)
{
	char addrstr[INET6_ADDRSTRLEN];
	struct nlmsghdr *nlh;
	struct rtmsg *rtm;
	uint32_t ifindex;
	char buf[4096];

	memset(buf, 0, sizeof(buf));
	nlh = (struct nlmsghdr *)buf;
	nlh->nlmsg_len = NLMSG_LENGTH(sizeof(struct rtmsg));
	nlh->nlmsg_type = cmd;
	nlh->nlmsg_flags = NLM_F_REQUEST | NLM_F_ACK;
	if (cmd == RTM_NEWROUTE)
		nlh->nlmsg_flags |= NLM_F_CREATE | NLM_F_REPLACE;
	nlh->nlmsg_seq = ++nl_seq;

	rtm = (struct rtmsg *)NLMSG_DATA(nlh);
	rtm->rtm_family = r->family;
	rtm->rtm_dst_len = r->prefixlen;
	rtm->rtm_table = RT_TABLE_MAIN;
	rtm->rtm_protocol = RTPROT_STATIC;
	rtm->rtm_scope = RT_SCOPE_UNIVERSE;
	rtm->rtm_type = RTN_UNICAST;

	/* Destination prefix */
	if (r->family == AF_INET)
		rta_add(nlh, sizeof(buf), RTA_DST, &r->prefix.ip4, sizeof(r->prefix.ip4));
	else
		rta_add(nlh, sizeof(buf), RTA_DST, &r->prefix.ip6, sizeof(r->prefix.ip6));

	/* Nexthop */
	switch (r->nh_type) {
	case NH_ADDR:
		/* Gateway address */
		if (r->family == AF_INET) {
			rta_add(nlh, sizeof(buf), RTA_GATEWAY, &r->gateway.gw4, sizeof(r->gateway.gw4));
			inet_ntop(AF_INET, &r->gateway.gw4, addrstr, sizeof(addrstr));
		} else {
			rta_add(nlh, sizeof(buf), RTA_GATEWAY, &r->gateway.gw6, sizeof(r->gateway.gw6));
			inet_ntop(AF_INET6, &r->gateway.gw6, addrstr, sizeof(addrstr));
		}
		DEBUG("netlink: %s route via %s",
		      cmd == RTM_NEWROUTE ? "add" : "del", addrstr);
		break;

	case NH_IFNAME:
		/* Output interface */
		ifindex = if_nametoindex(r->ifname);
		if (!ifindex) {
			ERROR("netlink: interface %s not found", r->ifname);
			return -1;
		}
		rta_add(nlh, sizeof(buf), RTA_OIF, &ifindex, sizeof(ifindex));
		DEBUG("netlink: %s route dev %s",
		      cmd == RTM_NEWROUTE ? "add" : "del", r->ifname);
		break;

	case NH_BLACKHOLE:
		/* Blackhole route */
		switch (r->bh_type) {
		case BH_DROP:
			rtm->rtm_type = RTN_BLACKHOLE;
			break;
		case BH_REJECT:
			rtm->rtm_type = RTN_UNREACHABLE;
			break;
		case BH_NULL:
			rtm->rtm_type = RTN_BLACKHOLE;
			break;
		}
		DEBUG("netlink: %s blackhole route",
		      cmd == RTM_NEWROUTE ? "add" : "del");
		break;
	}

	/* Priority (metric/distance) - kernel expects 32-bit value */
	if (r->distance) {
		uint32_t priority = r->distance;

		rta_add(nlh, sizeof(buf), RTA_PRIORITY, &priority, sizeof(priority));
	}

	/* Send and wait for ACK */
	if (nl_talk(nlh)) {
		if (errno == EEXIST && cmd == RTM_NEWROUTE) {
			DEBUG("netlink: route already exists");
			return 0;
		}
		if (errno == ESRCH && cmd == RTM_DELROUTE) {
			DEBUG("netlink: route doesn't exist");
			return 0;
		}
		ERROR("netlink: %s route failed: %s",
		      cmd == RTM_NEWROUTE ? "add" : "del", strerror(errno));
		return -1;
	}

	return 0;
}

int netlink_route_add(const struct route *r)
{
	return netlink_route_op(r, RTM_NEWROUTE);
}

int netlink_route_del(const struct route *r)
{
	return netlink_route_op(r, RTM_DELROUTE);
}

int linux_backend_init(void)
{
	struct sockaddr_nl sa = {
		.nl_family = AF_NETLINK,
	};

	INFO("Using Linux kernel backend (direct rtnetlink, no FRR)");

	nl_sock = socket(AF_NETLINK, SOCK_RAW, NETLINK_ROUTE);
	if (nl_sock < 0) {
		ERROR("Failed to create netlink socket: %s", strerror(errno));
		return -1;
	}

	if (bind(nl_sock, (struct sockaddr *)&sa, sizeof(sa)) < 0) {
		ERROR("Failed to bind netlink socket: %s", strerror(errno));
		close(nl_sock);
		nl_sock = -1;
		return -1;
	}

	DEBUG("Linux backend initialized, netlink socket fd=%d", nl_sock);
	return 0;
}

void linux_backend_cleanup(void)
{
	if (nl_sock >= 0) {
		close(nl_sock);
		nl_sock = -1;
		DEBUG("Linux backend shutdown");
	}
}

/* Check if route exists in list */
static int route_exists(struct route_head *list, const struct route *needle)
{
	struct route *r;

	TAILQ_FOREACH(r, list, entries) {
		if (r->family != needle->family)
			continue;
		if (r->prefixlen != needle->prefixlen)
			continue;

		/* Compare prefix */
		if (r->family == AF_INET) {
			if (memcmp(&r->prefix.ip4, &needle->prefix.ip4, sizeof(r->prefix.ip4)))
				continue;
		} else {
			if (memcmp(&r->prefix.ip6, &needle->prefix.ip6, sizeof(r->prefix.ip6)))
				continue;
		}

		/* Compare nexthop type */
		if (r->nh_type != needle->nh_type)
			continue;

		/* Compare nexthop details */
		switch (r->nh_type) {
		case NH_ADDR:
			if (r->family == AF_INET) {
				if (memcmp(&r->gateway.gw4, &needle->gateway.gw4, sizeof(r->gateway.gw4)))
					continue;
			} else {
				if (memcmp(&r->gateway.gw6, &needle->gateway.gw6, sizeof(r->gateway.gw6)))
					continue;
			}
			break;
		case NH_IFNAME:
			if (strcmp(r->ifname, needle->ifname))
				continue;
			break;
		case NH_BLACKHOLE:
			if (r->bh_type != needle->bh_type)
				continue;
			break;
		}

		/* Match found */
		return 1;
	}

	return 0;
}

/* Read installed routes from kernel (proto=RTPROT_STATIC only) */
static int kernel_read_routes(struct route_head *routes, int family)
{
	struct sockaddr_nl sa = { .nl_family = AF_NETLINK };
	struct nlmsghdr *nlh;
	struct rtattr *rta;
	struct msghdr msg;
	struct rtmsg *rtm;
	struct iovec iov;
	struct route *r;
	char buf[8192];
	int rta_len;
	int ret;

	msg.msg_name = &sa;
	msg.msg_namelen = sizeof(sa);
	msg.msg_iov = &iov;
	msg.msg_iovlen = 1;

	/* Request route dump */
	memset(buf, 0, sizeof(buf));
	nlh = (struct nlmsghdr *)buf;
	nlh->nlmsg_len = NLMSG_LENGTH(sizeof(struct rtmsg));
	nlh->nlmsg_type = RTM_GETROUTE;
	nlh->nlmsg_flags = NLM_F_REQUEST | NLM_F_DUMP;
	nlh->nlmsg_seq = ++nl_seq;

	rtm = (struct rtmsg *)NLMSG_DATA(nlh);
	rtm->rtm_family = family;

	iov.iov_base = nlh;
	iov.iov_len = nlh->nlmsg_len;

	if (sendmsg(nl_sock, &msg, 0) < 0) {
		ERROR("netlink: failed to request route dump: %s", strerror(errno));
		return -1;
	}

	/* Read response */
	while (1) {
		iov.iov_base = buf;
		iov.iov_len = sizeof(buf);

		ret = recvmsg(nl_sock, &msg, 0);
		if (ret < 0) {
			ERROR("netlink: route dump recvmsg: %s", strerror(errno));
			return -1;
		}

		for (nlh = (struct nlmsghdr *)buf; NLMSG_OK(nlh, ret); nlh = NLMSG_NEXT(nlh, ret)) {
			if (nlh->nlmsg_type == NLMSG_DONE)
				return 0;

			if (nlh->nlmsg_type == NLMSG_ERROR) {
				ERROR("netlink: route dump error");
				return -1;
			}

			if (nlh->nlmsg_type != RTM_NEWROUTE)
				continue;

			rtm = (struct rtmsg *)NLMSG_DATA(nlh);

			/* Only handle routes we manage (proto=RTPROT_STATIC) */
			if (rtm->rtm_protocol != RTPROT_STATIC)
				continue;

			/* Parse route attributes */
			r = calloc(1, sizeof(*r));
			if (!r)
				continue;

			r->family = rtm->rtm_family;
			r->prefixlen = rtm->rtm_dst_len;

			/* Parse attributes */
			rta = RTM_RTA(rtm);
			rta_len = RTM_PAYLOAD(nlh);

			for (; RTA_OK(rta, rta_len); rta = RTA_NEXT(rta, rta_len)) {
				switch (rta->rta_type) {
				case RTA_DST:
					if (r->family == AF_INET)
						memcpy(&r->prefix.ip4, RTA_DATA(rta), sizeof(r->prefix.ip4));
					else
						memcpy(&r->prefix.ip6, RTA_DATA(rta), sizeof(r->prefix.ip6));
					break;

				case RTA_GATEWAY:
					r->nh_type = NH_ADDR;
					if (r->family == AF_INET)
						memcpy(&r->gateway.gw4, RTA_DATA(rta), sizeof(r->gateway.gw4));
					else
						memcpy(&r->gateway.gw6, RTA_DATA(rta), sizeof(r->gateway.gw6));
					break;

				case RTA_OIF:
					r->nh_type = NH_IFNAME;
					if_indextoname(*(uint32_t *)RTA_DATA(rta), r->ifname);
					break;

				case RTA_PRIORITY:
					r->distance = *(uint32_t *)RTA_DATA(rta);
					break;
				}
			}

			/* Detect blackhole routes */
			if (rtm->rtm_type == RTN_BLACKHOLE || rtm->rtm_type == RTN_UNREACHABLE) {
				r->nh_type = NH_BLACKHOLE;
				if (rtm->rtm_type == RTN_UNREACHABLE)
					r->bh_type = BH_REJECT;
				else
					r->bh_type = BH_DROP;
			}

			TAILQ_INSERT_TAIL(routes, r, entries);
		}
	}

	return 0;
}

int linux_backend_apply(struct route_head *routes, struct rip_config *rip)
{
	struct route_head kernel_routes = TAILQ_HEAD_INITIALIZER(kernel_routes);
	struct route *r, *tmp;
	int removed = 0;
	int errors = 0;
	int added = 0;

	if (rip->enabled)
		DEBUG("Linux backend: RIP not supported without FRR");

	/* Read current static routes from kernel (both IPv4 and IPv6) */
	kernel_read_routes(&kernel_routes, AF_INET);
	kernel_read_routes(&kernel_routes, AF_INET6);

	/* Remove routes no longer in config */
	TAILQ_FOREACH_SAFE(r, &kernel_routes, entries, tmp) {
		if (!route_exists(routes, r)) {
			DEBUG("Removing old route");
			if (netlink_route_del(r) == 0)
				removed++;
		}
	}

	/* Add new routes from config (kernel_routes still has old state) */
	TAILQ_FOREACH(r, routes, entries) {
		if (!route_exists(&kernel_routes, r)) {
			DEBUG("Adding new route");
			if (netlink_route_add(r) == 0)
				added++;
			else
				errors++;
		}
	}

	/* Free kernel routes list */
	while ((tmp = TAILQ_FIRST(&kernel_routes)) != NULL) {
		TAILQ_REMOVE(&kernel_routes, tmp, entries);
		free(tmp);
	}

	INFO("Linux backend: +%d -%d routes (%d errors)", added, removed, errors);
	return errors ? -1 : 0;
}
