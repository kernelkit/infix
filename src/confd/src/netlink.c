/* SPDX-License-Identifier: BSD-3-Clause */

#include <arpa/inet.h>
#include <ctype.h>
#include <err.h>
#include <errno.h>
#include <net/if.h>		/* IFNAMSIZ */
#include <sys/socket.h>
#include <linux/types.h>
#include <linux/netlink.h>
#include <linux/rtnetlink.h>
#include <poll.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#define  NL_BUFSZ	4096

#define  dbg(fmt, args...) if (nl_debug) warnx(fmt, ##args)
#define  log(fmt, args...)               warnx(fmt, ##args)

static int   nl_debug;
static char *nl_buf;

static void scpy(char *dst, const char *src, size_t len)
{
	size_t i;

	for (i = 0; i < len; i++) {
		if (src[i] == 0)
			break;
		dst[i] = src[i];
	}

	if (i < len)
		dst[i] = 0;
	else
		dst[len - 1] = 0;
}

static int nlmsg_validate(struct nlmsghdr *nlmsg, ssize_t len)
{
	ssize_t la;

	if (nlmsg->nlmsg_len < NLMSG_LENGTH(sizeof(struct ifinfomsg)))
		return - 1;	/* Packet too small or truncated! */

	la = NLMSG_PAYLOAD(nlmsg, sizeof(struct ifinfomsg));
	if (la >= len)
		return -1;	/* Packet too large! */

	return 0;
}

static int nl_link(struct nlmsghdr *nlmsg, ssize_t len)
{
	char ifname[IFNAMSIZ];
	struct ifinfomsg *i;
	struct rtattr *a;
	ssize_t la;

	i  = NLMSG_DATA(nlmsg);
	a  = (struct rtattr *)((char *)i + NLMSG_ALIGN(sizeof(struct ifinfomsg)));
	la = NLMSG_PAYLOAD(nlmsg, sizeof(struct ifinfomsg));

	for (; RTA_OK(a, la); a = RTA_NEXT(a, la)) {
		if (a->rta_type != IFLA_IFNAME)
			continue;

		scpy(ifname, RTA_DATA(a), sizeof(ifname));

		switch (nlmsg->nlmsg_type) {
		case RTM_NEWLINK:
			dbg("%s: new link or IFF change, flags 0x%x, change 0x%x", ifname, i->ifi_flags, i->ifi_change);
			return 1;
		case RTM_DELLINK:
			dbg("%s: delete link", ifname);
			return 1;
		default:
			break;
		}
	}

	return 0;
}

int nl_callback(int sd)
{
	while (1) {
		struct nlmsghdr *nh;
		ssize_t len;
		size_t l;

		while ((len = recv(sd, nl_buf, NL_BUFSZ, 0)) < 0) {
			if (errno == EINTR)
				continue;

			return 0;
		}

		l = (size_t)len;
		for (nh = (struct nlmsghdr *)nl_buf; NLMSG_OK(nh, l); nh = NLMSG_NEXT(nh, l)) {
			switch (nh->nlmsg_type) {
			case RTM_NEWLINK:
			case RTM_DELLINK:
				if (nlmsg_validate(nh, len))
					continue;
				return nl_link(nh, len);
			case NLMSG_DONE:
			case NLMSG_ERROR:
				return 0;
			default:
				break;
			}
		}
	}

	return 0;
}

int main(void)
{
	struct sockaddr_nl sa = { 0 };
	struct pollfd pfd;
	int sd;

	sd = socket(AF_NETLINK, SOCK_RAW | SOCK_NONBLOCK | SOCK_CLOEXEC, NETLINK_ROUTE);
	if (sd < 0)
		err(1, "socket()");

	sa.nl_family = AF_NETLINK;
	sa.nl_groups = RTMGRP_IPV4_ROUTE | RTMGRP_LINK;
	sa.nl_pid    = getpid();

	if (bind(sd, (struct sockaddr *)&sa, sizeof(sa)) < 0)
		err(1, "bind()");

	nl_buf = malloc(NL_BUFSZ);
	if (!nl_buf)
		err(1, "malloc()");

	pfd.fd = sd;
	pfd.events = POLLIN;
	while (1) {
		int rc;

		rc = poll(&pfd, 1, -1);
		if (rc == -1)
			break;

		if (nl_callback(sd))
			log("link change");
	}

	return !!close(sd);
}
