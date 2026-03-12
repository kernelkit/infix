/* SPDX-License-Identifier: BSD-3-Clause */

#include <errno.h>
#include <getopt.h>
#include <sys/inotify.h>
#include <ev.h>
#include <libite/lite.h>

#include "netd.h"
#include "config.h"

int debug;

static struct route_head active_routes = TAILQ_HEAD_INITIALIZER(active_routes);
static struct rip_config active_rip;

/* Backend selection at compile time */
#ifdef HAVE_FRR_GRPC
#include "grpc_backend.h"
static int backend_init(void)    { return grpc_backend_init(); }
static void backend_cleanup(void) { grpc_backend_cleanup(); }
static int backend_apply(struct route_head *routes, struct rip_config *rip) {
	return grpc_backend_apply(routes, rip);
}
#elif defined(HAVE_FRR_CONF)
#include "frrconf_backend.h"
static int backend_init(void)    { return frrconf_backend_init(); }
static void backend_cleanup(void) { frrconf_backend_cleanup(); }
static int backend_apply(struct route_head *routes, struct rip_config *rip) {
	return frrconf_backend_apply(routes, rip);
}
#elif defined(HAVE_FRR_VTYSH)
#include "vtysh_backend.h"
static int backend_init(void)    { return vtysh_backend_init(); }
static void backend_cleanup(void) { vtysh_backend_cleanup(); }
static int backend_apply(struct route_head *routes, struct rip_config *rip) {
	return vtysh_backend_apply(routes, rip);
}
#else
#include "linux_backend.h"
static int backend_init(void)    { return linux_backend_init(); }
static void backend_cleanup(void) { linux_backend_cleanup(); }
static int backend_apply(struct route_head *routes, struct rip_config *rip) {
	return linux_backend_apply(routes, rip);
}
#endif

static ev_timer retry_w;

static void route_list_free(struct route_head *head)
{
	struct route *r, *tmp;

	TAILQ_FOREACH_SAFE(r, head, entries, tmp) {
		TAILQ_REMOVE(head, r, entries);
		free(r);
	}
}

static void rip_config_init(struct rip_config *cfg)
{
	memset(cfg, 0, sizeof(*cfg));

	/* Set defaults */
	cfg->default_metric = 1;
	cfg->distance = 120;
	cfg->timers.update = 30;
	cfg->timers.invalid = 180;
	cfg->timers.flush = 240;

	TAILQ_INIT(&cfg->networks);
	TAILQ_INIT(&cfg->neighbors);
	TAILQ_INIT(&cfg->redistributes);
	TAILQ_INIT(&cfg->system_cmds);
}

static void rip_config_free(struct rip_config *cfg)
{
	struct rip_redistribute *redist, *redist_tmp;
	struct rip_system_cmd *cmd, *cmd_tmp;
	struct rip_neighbor *nbr, *nbr_tmp;
	struct rip_network *net, *net_tmp;

	if (!cfg)
		return;

	TAILQ_FOREACH_SAFE(net, &cfg->networks, entries, net_tmp) {
		TAILQ_REMOVE(&cfg->networks, net, entries);
		free(net);
	}

	TAILQ_FOREACH_SAFE(nbr, &cfg->neighbors, entries, nbr_tmp) {
		TAILQ_REMOVE(&cfg->neighbors, nbr, entries);
		free(nbr);
	}

	TAILQ_FOREACH_SAFE(redist, &cfg->redistributes, entries, redist_tmp) {
		TAILQ_REMOVE(&cfg->redistributes, redist, entries);
		free(redist);
	}

	TAILQ_FOREACH_SAFE(cmd, &cfg->system_cmds, entries, cmd_tmp) {
		TAILQ_REMOVE(&cfg->system_cmds, cmd, entries);
		free(cmd);
	}
}

static void reload(struct ev_loop *loop)
{
	struct route_head new_routes = TAILQ_HEAD_INITIALIZER(new_routes);
	struct rip_redistribute *redist;
	struct rip_system_cmd *cmd;
	struct rip_config new_rip;
	struct rip_neighbor *nbr;
	struct rip_network *net;
	struct route *r;
	int count = 0;

	DEBUG("Reloading configuration");

	rip_config_init(&new_rip);

	if (config_load(&new_routes, &new_rip)) {
		ERROR("Failed loading config, keeping current routes");
		route_list_free(&new_routes);
		rip_config_free(&new_rip);
		return;
	}

	TAILQ_FOREACH(r, &new_routes, entries)
		count++;
	DEBUG("Loaded %d routes from config", count);
	if (new_rip.enabled)
		DEBUG("RIP configuration loaded");

	/* Apply config via backend */
	if (backend_apply(&new_routes, &new_rip)) {
		ERROR("Failed applying config via backend, retry in 5s");
		route_list_free(&new_routes);
		rip_config_free(&new_rip);
		ev_timer_stop(loop, &retry_w);
		ev_timer_set(&retry_w, 5., 0.);
		ev_timer_start(loop, &retry_w);
		pidfile(NULL);
		return;
	}
	ev_timer_stop(loop, &retry_w);

	route_list_free(&active_routes);
	TAILQ_INIT(&active_routes);
	rip_config_free(&active_rip);
	rip_config_init(&active_rip);

	/* Move new_routes to active_routes */
	while ((r = TAILQ_FIRST(&new_routes)) != NULL) {
		TAILQ_REMOVE(&new_routes, r, entries);
		TAILQ_INSERT_TAIL(&active_routes, r, entries);
	}

	/* Move new_rip to active_rip - copy scalars and move lists */
	active_rip.enabled = new_rip.enabled;
	active_rip.default_metric = new_rip.default_metric;
	active_rip.distance = new_rip.distance;
	active_rip.default_route = new_rip.default_route;
	active_rip.debug_events = new_rip.debug_events;
	active_rip.debug_packet = new_rip.debug_packet;
	active_rip.debug_kernel = new_rip.debug_kernel;
	active_rip.timers = new_rip.timers;

	/* Move network list */
	while ((net = TAILQ_FIRST(&new_rip.networks)) != NULL) {
		TAILQ_REMOVE(&new_rip.networks, net, entries);
		TAILQ_INSERT_TAIL(&active_rip.networks, net, entries);
	}

	/* Move neighbor list */
	while ((nbr = TAILQ_FIRST(&new_rip.neighbors)) != NULL) {
		TAILQ_REMOVE(&new_rip.neighbors, nbr, entries);
		TAILQ_INSERT_TAIL(&active_rip.neighbors, nbr, entries);
	}

	/* Move redistribute list */
	while ((redist = TAILQ_FIRST(&new_rip.redistributes)) != NULL) {
		TAILQ_REMOVE(&new_rip.redistributes, redist, entries);
		TAILQ_INSERT_TAIL(&active_rip.redistributes, redist, entries);
	}

	/* Move system commands list */
	while ((cmd = TAILQ_FIRST(&new_rip.system_cmds)) != NULL) {
		TAILQ_REMOVE(&new_rip.system_cmds, cmd, entries);
		TAILQ_INSERT_TAIL(&active_rip.system_cmds, cmd, entries);
	}

	/* Execute system commands after config is applied.
	 * Run in background with retry since daemons may not be ready yet. */
	if (!TAILQ_EMPTY(&active_rip.system_cmds)) {
		TAILQ_FOREACH(cmd, &active_rip.system_cmds, entries) {
			char retry_cmd[512];

			snprintf(retry_cmd, sizeof(retry_cmd),
				"(for i in 1 2 3 4 5; do %s && break || sleep 1; done) &",
				cmd->command);
			DEBUG("Executing system command with retry: %s", cmd->command);
			if (system(retry_cmd) != 0)
				ERROR("Failed to launch system command: %s", cmd->command);
		}
	}

	pidfile(NULL);
}

static void inotify_cb(struct ev_loop *loop, ev_io *w, int revents)
{
	char buf[sizeof(struct inotify_event) + NAME_MAX + 1];

	(void)revents;
	while (read(w->fd, buf, sizeof(buf)) > 0)
		;
	DEBUG("conf.d changed, triggering reload");
	reload(loop);
}

static void sighup_cb(struct ev_loop *loop, ev_signal *w, int revents)
{
	(void)w; (void)revents;
	INFO("Got SIGHUP, reloading ...");
	reload(loop);
}

static void sigterm_cb(struct ev_loop *loop, ev_signal *w, int revents)
{
	(void)w; (void)revents;
	ev_break(loop, EVBREAK_ALL);
}

static void retry_cb(struct ev_loop *loop, ev_timer *w, int revents)
{
	(void)w; (void)revents;
	reload(loop);
}

static int usage(int rc)
{
	fprintf(stderr,
		"Usage: netd [-dhv]\n"
		"  -d  Enable debug (log to stderr)\n"
		"  -h  Show this help text\n"
		"  -v  Show version and exit\n");
	return rc;
}

int main(int argc, char *argv[])
{
	struct ev_loop *loop = EV_DEFAULT;
	ev_signal sighup_w, sigterm_w, sigint_w;
	ev_io inotify_w;
	int log_opts = LOG_PID | LOG_NDELAY;
	int ifd = -1;
	int c;

	while ((c = getopt(argc, argv, "dhv")) != -1) {
		switch (c) {
		case 'd':
			log_opts |= LOG_PERROR;
			debug = 1;
			break;
		case 'h':
			return usage(0);
		case 'v':
			puts("v" PACKAGE_VERSION);
			return 0;
		default:
			return usage(1);
		}
	}

	openlog("netd", log_opts, LOG_DAEMON);
	setlogmask(LOG_UPTO(LOG_INFO));
	INFO("v%s starting", PACKAGE_VERSION);

	if (backend_init()) {
		ERROR("Failed to initialize backend");
		closelog();
		return 1;
	}

	TAILQ_INIT(&active_routes);
	rip_config_init(&active_rip);

	/* Signal watchers */
	ev_signal_init(&sighup_w, sighup_cb, SIGHUP);
	ev_signal_start(loop, &sighup_w);

	ev_signal_init(&sigterm_w, sigterm_cb, SIGTERM);
	ev_signal_start(loop, &sigterm_w);

	ev_signal_init(&sigint_w, sigterm_cb, SIGINT);
	ev_signal_start(loop, &sigint_w);

	/* Retry timer — one-shot, started only on backend failure */
	ev_timer_init(&retry_w, retry_cb, 0., 0.);

	/* Watch conf.d for changes so we don't rely solely on signals */
	mkdir(CONF_DIR, 0755);
	ifd = inotify_init1(IN_CLOEXEC | IN_NONBLOCK);
	if (ifd < 0) {
		ERROR("inotify_init1: %s, falling back to signals only", strerror(errno));
	} else if (inotify_add_watch(ifd, CONF_DIR,
				     IN_CLOSE_WRITE | IN_DELETE |
				     IN_MOVED_TO | IN_MOVED_FROM) < 0) {
		ERROR("inotify_add_watch %s: %s", CONF_DIR, strerror(errno));
		close(ifd);
		ifd = -1;
	} else {
		ev_io_init(&inotify_w, inotify_cb, ifd, EV_READ);
		ev_io_start(loop, &inotify_w);
	}

	/* Initial load */
	reload(loop);

	ev_run(loop, 0);

	INFO("shutting down");

	if (ifd >= 0)
		close(ifd);
	route_list_free(&active_routes);
	rip_config_free(&active_rip);
	backend_cleanup();

	closelog();
	return 0;
}
