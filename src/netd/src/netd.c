/* SPDX-License-Identifier: BSD-3-Clause */

#include <errno.h>
#include <getopt.h>
#include <libite/lite.h>

#include "netd.h"
#include "config.h"
#include "route.h"
#include "rip.h"

/* Backend selection at compile time */
#ifdef HAVE_FRR_GRPC
#include "grpc_backend.h"
static int backend_init(void)    { return grpc_backend_init(); }
static void backend_fini(void)   { grpc_backend_fini(); }
static int backend_apply(struct route_head *routes, struct rip_config *rip) {
	return grpc_backend_apply(routes, rip);
}
#else
#include "linux_backend.h"
static int backend_init(void)    { return linux_backend_init(); }
static void backend_fini(void)   { linux_backend_fini(); }
static int backend_apply(struct route_head *routes, struct rip_config *rip) {
	return linux_backend_apply(routes, rip);
}
#endif

int debug;

static volatile sig_atomic_t do_reload;
static volatile sig_atomic_t do_shutdown;

static struct route_head active_routes = TAILQ_HEAD_INITIALIZER(active_routes);
static struct rip_config active_rip;

static void sighup_handler(int sig)
{
	(void)sig;
	do_reload = 1;
}

static void sigterm_handler(int sig)
{
	(void)sig;
	do_shutdown = 1;
}

static void reload(void)
{
	struct route_head new_routes = TAILQ_HEAD_INITIALIZER(new_routes);
	struct rip_config new_rip;
	struct route *r;
	int count = 0;

	INFO("Reloading configuration");

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
		ERROR("Failed applying config via backend");
		route_list_free(&new_routes);
		rip_config_free(&new_rip);
		return;
	}

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
	struct rip_network *net;
	while ((net = TAILQ_FIRST(&new_rip.networks)) != NULL) {
		TAILQ_REMOVE(&new_rip.networks, net, entries);
		TAILQ_INSERT_TAIL(&active_rip.networks, net, entries);
	}

	/* Move neighbor list */
	struct rip_neighbor *nbr;
	while ((nbr = TAILQ_FIRST(&new_rip.neighbors)) != NULL) {
		TAILQ_REMOVE(&new_rip.neighbors, nbr, entries);
		TAILQ_INSERT_TAIL(&active_rip.neighbors, nbr, entries);
	}

	/* Move redistribute list */
	struct rip_redistribute *redist;
	while ((redist = TAILQ_FIRST(&new_rip.redistributes)) != NULL) {
		TAILQ_REMOVE(&new_rip.redistributes, redist, entries);
		TAILQ_INSERT_TAIL(&active_rip.redistributes, redist, entries);
	}

	/* Move system commands list */
	struct rip_system_cmd *cmd;
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

	INFO("Configuration reloaded");
}

static int usage(int rc)
{
	fprintf(stderr,
		"Usage: netd [-dh] [-p pidfile]\n"
		"  -d  Enable debug (log to stderr)\n"
		"  -h  Show this help text\n"
		"  -p  Set pidfile path (default: from program name)\n");
	return rc;
}

int main(int argc, char *argv[])
{
	int log_opts = LOG_PID | LOG_NDELAY;
	const char *pidfn = NULL;
	struct sigaction sa;
	int c;

	while ((c = getopt(argc, argv, "dhp:")) != -1) {
		switch (c) {
		case 'd':
			log_opts |= LOG_PERROR;
			debug = 1;
			break;
		case 'h':
			return usage(0);
		case 'p':
			pidfn = optarg;
			break;
		default:
			return usage(1);
		}
	}

	openlog("netd", log_opts, LOG_DAEMON);
	INFO("netd starting");

	/* Set up signal handlers */
	memset(&sa, 0, sizeof(sa));
	sa.sa_handler = sighup_handler;
	sigaction(SIGHUP, &sa, NULL);

	sa.sa_handler = sigterm_handler;
	sigaction(SIGTERM, &sa, NULL);
	sigaction(SIGINT, &sa, NULL);

	if (backend_init()) {
		ERROR("Failed to initialize backend");
		closelog();
		return 1;
	}

	TAILQ_INIT(&active_routes);
	rip_config_init(&active_rip);

	/* Initial load */
	do_reload = 1;

	/* Signal readiness to Finit */
	pidfile(pidfn);

	while (!do_shutdown) {
		if (do_reload) {
			do_reload = 0;
			reload();
		}
		pause();
	}

	INFO("netd shutting down");

	route_list_free(&active_routes);
	rip_config_free(&active_rip);
	backend_fini();

	closelog();
	return 0;
}
