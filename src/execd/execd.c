/* SPDX-License-Identifier: ISC */

#include "execd.h"
#define RETRY_TIMER 60

static uev_t retry_watcher;
static int   retry = RETRY_TIMER;

static int   logmask = LOG_UPTO(LOG_NOTICE);
static char  buffer[BUFSIZ];

static int run_job(const char *path, const char *file)
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
		errx("%s skipping, not executable.", cmd);
		return -1;
	}

	dbg("running job %s", cmd);
	if ((rc = systemf("%s", cmd))) {
		errx("%s failed, exit code: %d", cmd, rc);
		return -1;
	}

	return erase(cmd);
}

static int run_dir(const char *path, int type)
{
	struct dirent **namelist;
	int n, i, num = 0;

	n = scandir(path, &namelist, NULL, alphasort);
	if (n < 0) {
		err("scandir %s", path);
		return 0;
	}

	for (i = 0; i < n; i++) {
		struct dirent *d = namelist[i];

		if (d->d_type == DT_DIR)
			continue;

		if (should_run(d->d_name, type))
			num += !!run_job(path, d->d_name);

		free(d);
	}

	free(namelist);

	return num;
}

/*
 * Call stop/cleanup jobs first, may use same container name or
 * resources as replacement container start scripts use.
 */
static void run_queue(const char *path)
{
	int num;

	num  = run_dir(path, 'K');
	num += run_dir(path, 'S');

	if (num)
		uev_timer_set(&retry_watcher, retry, 0);
}

static void signal_cb(uev_t *w, void *arg, int signo)
{
	dbg("signal %d, calling job queue", signo);
	run_queue(arg);
}

static void timer_cb(uev_t *w, void *arg, int _)
{
	dbg("timer, retry job queue");
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
	int num = 0;

	bytes = read(w->fd, buffer, sizeof(buffer));
	if (bytes == -1) {
		err("read");
		return;
	}

	for (char *p = buffer; p < buffer + bytes;) {
		struct inotify_event *event = (struct inotify_event *)p;
		char *name = event->name;

		if (event->mask & (IN_CLOSE_WRITE | IN_ATTRIB | IN_MOVED_TO)) {
			dbg("Got inotify event %s 0x%04x", name, event->mask);
			if (!should_run(name, '*'))
				continue;

			num += !!run_job(arg, name);
		}

		p += sizeof(struct inotify_event) + event->len;
	}

	if (num)
		uev_timer_set(&retry_watcher, retry, 0);
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

static int usage(const char *arg0, int rc)
{
	printf("Usage:\n"
	       "  %s [-dh] [-l LVL] [-t SEC] QUEUE\n"
	       "Options:\n"
	       "  -d      Log to stderr as well\n"
	       "  -h      This help text\n"
	       "  -l LVL  Set log level: none, err, warn, notice*, info, debug\n"
	       "  -t SEC  Retry timer in seconds [10, 604800], default: %d\n"
	       "\n"
	       "Run jobs from QUEUE.  Triggers on inotify of new jobs, route changes, and\n"
               "retries failing jobs every minute until the queue has been emtied.\n"
	       "Use SIGHUP to trigger a manual retry.\n"
	       "Use SIGUSR1 to toggle debug messages at runtime.\n", arg0, RETRY_TIMER);

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
	char *queue;
	uev_ctx_t ctx;
	int rc = 0;

	while ((c = getopt(argc, argv, "dhl:t:")) != EOF) {
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
		case 't':
			retry = atoi(optarg);
			if (retry < 10 || retry > 604800) {
				fprintf(stderr, "Invalid value %d, accepted [10, 604800]", retry);
				return 1;
			}
			break;
		default:
			return usage(argv[0], 1);
		}
	}

	if (optind >= argc)
		return usage(argv[0], 1);

	queue = argv[optind];
	retry *= 1000;

	if (access(queue, X_OK)) {
		fprintf(stderr, "Cannot find job directory %s, errno %d: %s\n",
			queue, errno, strerror(errno));
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

	wd = inotify_add_watch(fd, queue, IN_CLOSE_WRITE | IN_ATTRIB | IN_MOVED_TO);
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
	if (uev_signal_init(&ctx, &sighup_watcher, signal_cb, queue, SIGHUP) == -1) {
		err("uev_signal_init (sighup)");
		rc = 1;
		goto done;
	}

	/* Initial delay of 1 sec, lots of other events happening at boot. */
	if (uev_timer_init(&ctx, &retry_watcher, timer_cb, queue, 1000, 0) == -1) {
		err("uev_timer_init (1000, %d)", retry);
		rc = 1;
		goto done;
	}

	if (uev_signal_init(&ctx, &sigusr1_watcher, toggle_debug, NULL, SIGUSR1) == -1) {
		err("uev_signal_init (sigusr1)");
		rc = 1;
		goto done;
	}

	if (uev_io_init(&ctx, &inotify_watcher, inotify_cb, queue, fd, UEV_READ) == -1) {
		err("uev_io_init (inotify)");
		rc = 1;
		goto done;
	}

	if (uev_io_init(&ctx, &netlink_watcher, netlink_cb, queue, sd, UEV_READ) == -1) {
		err("uev_io_init (netlink)");
		rc = 1;
		goto done;
	}

	if (uev_run(&ctx, 0) == -1) {
		err("uev_run");
		rc = 1;
	}

done:
	close(fd);
	close(sd);

	return rc;
}
