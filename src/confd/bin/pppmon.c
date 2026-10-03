/* SPDX-License-Identifier: BSD-3-Clause */
/*
 * pppmon - create and hold a PPP interface
 *
 * The kernel removes a ppp interface when the file that created it is
 * closed, so the interface needs a process to own it.  pppmon creates
 * the interface and holds it until SIGTERM.  pppd, run by Finit,
 * attaches to it for each session, so the interface exists before the
 * first session and stays across reconnects.
 *
 *    pppmon [-o OPTFILE] [-p PIDFILE] IFNAME
 *
 * OPTFILE gets the pppd option to attach to the interface, for pppd's
 * file option.  pppmon returns once the interface exists and both files
 * are written, then holds the interface in the background.
 */

#include <errno.h>
#include <fcntl.h>
#include <signal.h>
#include <stdio.h>
#include <string.h>
#include <syslog.h>
#include <unistd.h>
#include <sys/ioctl.h>
#include <sys/socket.h>
#include <linux/if_link.h>
#include <linux/netlink.h>
#include <linux/ppp-ioctl.h>
#include <linux/rtnetlink.h>
#include <libite/lite.h>

static void addattr(struct nlmsghdr *nlh, int type, const void *data, size_t len)
{
	struct rtattr *rta = (struct rtattr *)((char *)nlh + NLMSG_ALIGN(nlh->nlmsg_len));

	rta->rta_type = type;
	rta->rta_len = RTA_LENGTH(len);
	if (len)
		memcpy(RTA_DATA(rta), data, len);
	nlh->nlmsg_len = NLMSG_ALIGN(nlh->nlmsg_len) + RTA_ALIGN(rta->rta_len);
}

static struct rtattr *nest_start(struct nlmsghdr *nlh, int type)
{
	struct rtattr *nest = (struct rtattr *)((char *)nlh + NLMSG_ALIGN(nlh->nlmsg_len));

	addattr(nlh, type, NULL, 0);
	return nest;
}

static void nest_end(struct nlmsghdr *nlh, struct rtattr *nest)
{
	nest->rta_len = (char *)nlh + nlh->nlmsg_len - (char *)nest;
}

/* Create a ppp interface named ifname for the /dev/ppp file fd */
static int ppp_create(int fd, const char *ifname)
{
	struct {
		struct nlmsghdr  nlh;
		struct ifinfomsg ifm;
		char             buf[256];
	} req = { 0 };
	struct rtattr *linkinfo, *data;
	char ack[512];
	int sd, err = EIO;

	req.nlh.nlmsg_len = NLMSG_LENGTH(sizeof(req.ifm));
	req.nlh.nlmsg_type = RTM_NEWLINK;
	req.nlh.nlmsg_flags = NLM_F_REQUEST | NLM_F_ACK | NLM_F_CREATE | NLM_F_EXCL;
	req.ifm.ifi_family = AF_UNSPEC;

	addattr(&req.nlh, IFLA_IFNAME, ifname, strlen(ifname) + 1);
	linkinfo = nest_start(&req.nlh, IFLA_LINKINFO);
	addattr(&req.nlh, IFLA_INFO_KIND, "ppp", 4);
	data = nest_start(&req.nlh, IFLA_INFO_DATA);
	addattr(&req.nlh, IFLA_PPP_DEV_FD, &fd, sizeof(fd));
	nest_end(&req.nlh, data);
	nest_end(&req.nlh, linkinfo);

	sd = socket(AF_NETLINK, SOCK_RAW | SOCK_CLOEXEC, NETLINK_ROUTE);
	if (sd < 0)
		return -1;

	/* The kernel may ask us to retry, see ppp_nl_newlink() */
	do {
		struct nlmsgerr *nlerr;
		ssize_t len;

		if (send(sd, &req, req.nlh.nlmsg_len, 0) < 0)
			break;
		len = recv(sd, ack, sizeof(ack), 0);
		if (len < (ssize_t)NLMSG_LENGTH(sizeof(*nlerr)))
			break;

		nlerr = NLMSG_DATA((struct nlmsghdr *)ack);
		err = -nlerr->error;
	} while (err == EBUSY);

	close(sd);
	if (err) {
		errno = err;
		return -1;
	}

	return 0;
}

/*
 * Like daemon(), but the parent only returns when the child has written
 * its pidfile, so the caller can signal it from then on.
 */
static int background(const char *pidfn)
{
	int fd, pfd[2];
	pid_t pid;
	char c;

	if (pipe(pfd))
		return -1;

	pid = fork();
	if (pid < 0)
		return -1;
	if (pid > 0) {
		close(pfd[1]);
		/* EOF when the child is ready */
		_exit(read(pfd[0], &c, 1) == 0 ? 0 : 1);
	}

	close(pfd[0]);
	setsid();
	if (chdir("/"))
		return -1;
	fd = open("/dev/null", O_RDWR);
	if (fd >= 0) {
		dup2(fd, STDIN_FILENO);
		dup2(fd, STDOUT_FILENO);
		dup2(fd, STDERR_FILENO);
		if (fd > STDERR_FILENO)
			close(fd);
	}

	/* Removed again at exit */
	if (pidfn && pidfile(pidfn))
		syslog(LOG_ERR, "failed writing %s: %m", pidfn);
	close(pfd[1]);

	return 0;
}

static int usage(int rc)
{
	fprintf(stderr, "usage: pppmon [-o OPTFILE] [-p PIDFILE] IFNAME\n");
	return rc;
}

int main(int argc, char *argv[])
{
	const char *optfn = NULL, *pidfn = NULL, *ifname;
	int c, fd, sig, unit;
	sigset_t set;

	while ((c = getopt(argc, argv, "ho:p:")) != EOF) {
		switch (c) {
		case 'h':
			return usage(0);
		case 'o':
			optfn = optarg;
			break;
		case 'p':
			pidfn = optarg;
			break;
		default:
			return usage(1);
		}
	}
	if (optind != argc - 1)
		return usage(1);

	ifname = argv[optind];
	openlog("pppmon", LOG_PID | LOG_PERROR, LOG_DAEMON);

	fd = open("/dev/ppp", O_RDWR);
	if (fd < 0) {
		syslog(LOG_ERR, "%s: failed opening /dev/ppp: %m", ifname);
		return 1;
	}
	if (ppp_create(fd, ifname) || ioctl(fd, PPPIOCGUNIT, &unit)) {
		syslog(LOG_ERR, "%s: failed creating interface: %m", ifname);
		return 1;
	}

	if (optfn) {
		FILE *fp = fopen(optfn, "w");

		if (!fp) {
			syslog(LOG_ERR, "%s: failed writing %s: %m", ifname, optfn);
			return 1;
		}
		fprintf(fp, "attach-unit %d\n", unit);
		fclose(fp);
	}

	/* The interface exists, let the caller configure it */
	closelog();
	openlog("pppmon", LOG_PID, LOG_DAEMON);
	if (background(pidfn)) {
		syslog(LOG_ERR, "%s: failed going to background: %m", ifname);
		return 1;
	}
	syslog(LOG_INFO, "%s: created, unit %d", ifname, unit);

	/* A stray SIGHUP must not take the interface with it */
	signal(SIGHUP, SIG_IGN);
	sigemptyset(&set);
	sigaddset(&set, SIGTERM);
	sigaddset(&set, SIGINT);
	sigprocmask(SIG_BLOCK, &set, NULL);
	sigwait(&set, &sig);

	if (optfn)
		erase(optfn);
	syslog(LOG_INFO, "%s: removed", ifname);

	/* Closing the last reference to fd removes the interface */
	return 0;
}
