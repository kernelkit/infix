/* SPDX-License-Identifier: ISC */

#include <ctype.h>
#include <dirent.h>
#include <errno.h>
#include <getopt.h>
#include <libgen.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#define SYSLOG_NAMES
#include <syslog.h>
#include <unistd.h>

#include <linux/netlink.h>
#include <linux/rtnetlink.h>

#include <sys/inotify.h>
#include <sys/socket.h>

#include <uev/uev.h>
#include <libite/lite.h>

#define err(fmt, args...)   syslog(LOG_ERR,     fmt ": %s", ##args, strerror(errno))
#define errx(fmt, args...)  syslog(LOG_ERR,     fmt, ##args)
#define warn(fmt, args...)  syslog(LOG_WARNING, fmt, ": %s", ##args, strerror(errno))
#define warnx(fmt, args...) syslog(LOG_WARNING, fmt, ##args)
#define log(fmt, args...)   syslog(LOG_NOTICE,  fmt, ##args)
#define dbg(fmt, args...)   syslog(LOG_DEBUG,   fmt, ##args)

static int   logmask = LOG_UPTO(LOG_NOTICE);
static char  buffer[BUFSIZ];
static char *done;

static void run_job(const char *path, char *file, int archive)
{
	char cmd[strlen(path) + strlen(file) + 2];
	int rc;

	/*
	 * Unfortunately, on some systems (x86_64), execd reacts too
	 * quickly to route and inotify events.  So we have this load
	 * bearing sleep here to guard against "text file busy" and
	 * "destination unreachable" errors.
	 */
	usleep(500000);

	snprintf(cmd, sizeof(cmd), "%s/%s", path, file);
	if (access(cmd, X_OK)) {
		errx("skipping %s, not executable", cmd);
		return;
	}

	dbg("running job %s", cmd);
	if ((rc = systemf("%s", cmd))) {
		errx("failed %s: rc %d", cmd, rc);
		return;
	}

	dbg("job %s in %s done %p, archive: %d", file, path, done, archive);
	if (done && archive)
		movefile(cmd, done);
	else
		erase(cmd);
}

/*
 * Allow SNN and KNN style jobs, for inotyify_cb() we also allow
 * a type '*' just to figure out if a job should be archived in
 * the done directory.
 */
static int should_run(const char *name, int type, int *archive)
{
	if (!name || strlen(name) < 3)
		return 0;

	if (isdigit(name[1]) && isdigit(name[2])) {
		if (type == '*') {
			switch (name[0]) {
			case 'K':
				*archive = 0;
				return 1;
			case 'S':
				*archive = 1;
				return 1;
			default:
				errx("unsupported '%s', scripts must start with S or K", name);
				return 0;
			}
		}

		switch (type) {
		case 'K':
			*archive = 0;
			break;
		case 'S':
			*archive = 1;
			break;
		default:
			return 0;
		}

		dbg("name:%s type:'%c' archive:%d => run:%d", name, type, *archive, type == name[0]);
		return type == name[0];
	}

	errx("unsupported script %s, must follow pattern SNN/KNN", name);
	return 0;
}

static void run_dir(const char *path, int type)
{
	struct dirent **namelist;
	int archive = 0;
	int n, i;

	n = scandir(path, &namelist, NULL, alphasort);
	if (n < 0) {
		err("scandir %s", path);
		return;
	}

	for (i = 0; i < n; i++) {
		struct dirent *d = namelist[i];

		if (d->d_type == DT_DIR)
			continue;

		if (should_run(d->d_name, type, &archive))
			run_job(path, d->d_name, archive);

		free(d);
	}

	free(namelist);
}

/*
 * Call stop/cleanup jobs first, may use same container name or
 * resources as replacement container start scripts use.
 */
static void run_queue(char *path)
{
	run_dir(path, 'K');
	run_dir(path, 'S');
}

static void signal_cb(uev_t *w, void *arg, int _)
{
	dbg("Got signal, calling job queue");
	run_queue(arg);
}

static void toggle_debug(uev_t *w, void *arg, int _)
{
	int current = setlogmask(0);

	if (current == logmask)
		setlogmask(LOG_UPTO(LOG_DEBUG));
	else
		setlogmask(logmask);
}

static void inotify_cb(uev_t *w, void *arg, int _)
{
	ssize_t bytes;

	bytes = read(w->fd, buffer, sizeof(buffer));
	if (bytes == -1) {
		err("read");
		return;
	}

	for (char *p = buffer; p < buffer + bytes;) {
		struct inotify_event *event = (struct inotify_event *)p;
		char *name = event->name;
		int archive;

		if (event->mask & (IN_CLOSE_WRITE | IN_ATTRIB | IN_MOVED_TO)) {
			dbg("Got inotify event %s 0x%04x", name, event->mask);
			if (!should_run(name, '*', &archive))
				continue;

			run_job(arg, name, archive);
		}

		p += sizeof(struct inotify_event) + event->len;
	}
}

static void netlink_cb(uev_t *w, void *arg, int _)
{
	struct iovec iov = { buffer, sizeof(buffer) };
	struct sockaddr_nl addr;
	struct msghdr msg = {
		&addr, sizeof(addr),
		&iov, 1,
		NULL, 0,
		0
	};
	ssize_t bytes;
	int sd = w->fd;

	dbg("Got netlink event");

	/* Empty netlink queue, we just want the event */
	while ((bytes = recvmsg(sd, &msg, 0)) > 0)
		dbg("Read %ld netlink bytes", bytes);

	dbg("Calling run queue");
	run_queue(arg);
}

int logmask_from_str(const char *str)
{
	const CODE *code;

	for (code = prioritynames; code->c_name; code++)
		if (!strcmp(str, code->c_name))
			return LOG_UPTO(code->c_val);

	return -1;
}

static int usage(char *arg0, int rc)
{
	printf("Usage:\n"
	       "  %s [-dh] [-l LVL] JOBDIR\n"
	       "Options:\n"
	       "  -d        Log to stderr as well\n"
	       "  -h        This help text\n"
	       "  -l LVL  Set log level: none, err, warn, notice*, info, debug\n"
	       "\n"
	       "Runs jobs from JOBDIR, re-runs failing jobs on route changes or SIGHUP.\n"
	       "Use SIGUSR1 to toggle debug messages at runtime.\n", arg0);

	return rc;
}

int main(int argc, char *argv[])
{
	struct sockaddr_nl sa = { 0 };
	uev_t inotify_watcher;
	uev_t netlink_watcher;
	uev_t sigusr1_watcher;
	uev_t sighup_watcher;
	int logopt = LOG_PID;
	int wd, sd, fd, c;
	char *jobdir;
	uev_ctx_t ctx;
	int rc = 0;

	while ((c = getopt(argc, argv, "dhl:")) != EOF) {
		switch (c) {
		case 'd':
			logopt |= LOG_PERROR;
			break;
		case 'h':
			return usage(argv[0], 0);
		case 'l':
			logmask = logmask_from_str(optarg);
			if (logmask < 0) {
				fprintf(stderr, "Invalid loglevel '%s'\n\n", optarg);
				return usage(argv[0], 1);
			}
			break;
		default:
			return usage(argv[0], 1);
		}
	}

	if (optind >= argc)
		return usage(argv[0], 1);

	jobdir = argv[optind++];
	if (optind < argc)
		done = argv[optind];

	if (access(jobdir, X_OK)) {
		fprintf(stderr, "Cannot find job directory %s, errno %d: %s\n",
			jobdir, errno, strerror(errno));
		return 1;
	}

	/*
	 * We close stdin, while leaving stdout et stderr open so a user
	 * can redirect output from us to a logger process, or
	 * similar.
	 */
	close(STDIN_FILENO);

	/* The logs of this program go to syslog w/ regular daemon facility  */
	openlog(NULL, logopt, LOG_DAEMON);
	setlogmask(logmask);

	fd = inotify_init1(IN_NONBLOCK);
	if (fd == -1) {
		err("inotify_init");
		return 1;
	}

	wd = inotify_add_watch(fd, jobdir, IN_CLOSE_WRITE | IN_ATTRIB | IN_MOVED_TO);
	if (wd == -1) {
		err("inotify_add_watch");
		close(fd);
		return 1;
	}

	/* Set up netlink socket for route monitoring */
	sd = socket(AF_NETLINK, SOCK_RAW | SOCK_NONBLOCK, NETLINK_ROUTE);
	if (sd == -1) {
		err("socket");
		close(fd);
		return 1;
	}

	sa.nl_family = AF_NETLINK;
	sa.nl_groups = RTMGRP_IPV4_ROUTE | RTMGRP_IPV6_ROUTE;
	if (bind(sd, (struct sockaddr *)&sa, sizeof(sa)) == -1) {
		err("bind");
		rc = 1;
		goto done;
	}

	uev_init(&ctx);
	if (uev_signal_init(&ctx, &sighup_watcher, signal_cb, jobdir, SIGHUP) == -1) {
		err("uev_signal_init (sighup)");
		rc = 1;
		goto done;
	}
	if (uev_signal_init(&ctx, &sigusr1_watcher, toggle_debug, NULL, SIGUSR1) == -1) {
		err("uev_signal_init (sigusr1)");
		rc = 1;
		goto done;
	}

	if (uev_io_init(&ctx, &inotify_watcher, inotify_cb, jobdir, fd, UEV_READ) == -1) {
		err("uev_io_init (inotify)");
		rc = 1;
		goto done;
	}

	if (uev_io_init(&ctx, &netlink_watcher, netlink_cb, jobdir, sd, UEV_READ) == -1) {
		err("uev_io_init (netlink)");
		rc = 1;
		goto done;
	}

	run_queue(jobdir);
	if (uev_run(&ctx, 0) == -1) {
		err("uev_run");
		rc = 1;
	}

done:
	close(fd);
	close(sd);

	return rc;
}
