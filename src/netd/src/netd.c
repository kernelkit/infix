/* SPDX-License-Identifier: BSD-3-Clause */

#include <errno.h>
#include <getopt.h>
#include <libite/lite.h>

#include "netd.h"
#include "config.h"

/* Backend selection at compile time */
#ifdef HAVE_FRR_GRPC
#include "grpc_backend.h"
static int backend_init(void)    { return grpc_backend_init(); }
static void backend_cleanup(void) { grpc_backend_cleanup(); }
static int backend_apply(struct route_head *routes, struct rip_config *rip) {
	return grpc_backend_apply(routes, rip);
}
#else
#include "linux_backend.h"
static int backend_init(void)    { return linux_backend_init(); }
static void backend_cleanup(void) { linux_backend_cleanup(); }
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

static void reload(void)
{
	struct route_head new_routes = TAILQ_HEAD_INITIALIZER(new_routes);
	struct rip_redistribute *redist;
	struct rip_system_cmd *cmd;
	struct rip_config new_rip;
	struct rip_neighbor *nbr;
	struct rip_network *net;
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
	INFO("Configuration reloaded");
}

static int usage(int rc)
{
	fprintf(stderr,
		"Usage: netd [-dh]\n"
		"  -d  Enable debug (log to stderr)\n"
		"  -h  Show this help text\n");
	return rc;
}

int main(int argc, char *argv[])
{
	int log_opts = LOG_PID | LOG_NDELAY;
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

	pidfile(NULL);
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
	backend_cleanup();

	closelog();
	return 0;
}
