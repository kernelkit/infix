/* SPDX-License-Identifier: BSD-3-Clause */

#include <getopt.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <sysrepo.h>
#include <sysrepo/version.h>
#include <ev.h>
#include <string.h>
#include <errno.h>
#include <time.h>
#include <sys/stat.h>

#include <asm/types.h>
#include <sys/socket.h>
#include <linux/netlink.h>
#include <linux/rtnetlink.h>

#include <libite/lite.h>
#include <jansson.h>
#include <ctype.h>
#include <linux/if.h>
#include <sys/queue.h>
#include <sys/mman.h>

#include <srx/common.h>
#include <srx/helpers.h>
#include <srx/lyx.h>
#include <srx/systemv.h>

#include "shared.h"
#include "journal.h"
#include "avahi.h"

/* New kernel feature, not in sys/mman.h yet */
#ifndef MFD_NOEXEC_SEAL
#define MFD_NOEXEC_SEAL 0x0008U
#endif

#define YANGER_BINPATH YANGER_DIR"/yanger"
#define XPATH_MAX PATH_MAX
#define XPATH_IFACE_BASE "/ietf-interfaces:interfaces"
#define XPATH_ROUTING_BASE "/ietf-routing:routing/control-plane-protocols/control-plane-protocol"
#define XPATH_ROUTING_TABLE "/ietf-routing:routing/ribs"
#define XPATH_HARDWARE_BASE "/ietf-hardware:hardware"
#define XPATH_SYSTEM_BASE "/ietf-system"
#define XPATH_ROUTING_OSPF XPATH_ROUTING_BASE "/ospf"
#define XPATH_ROUTING_RIP XPATH_ROUTING_BASE "/rip"
#define XPATH_ROUTING_BFD XPATH_ROUTING_BASE "/bfd"
#define XPATH_CONTAIN_BASE  "/infix-containers:containers"
#define XPATH_DHCP_SERVER_BASE  "/infix-dhcp-server:dhcp-server"
#define XPATH_LLDP_BASE "/ieee802-dot1ab-lldp:lldp"
#define XPATH_FIREWALL_BASE "/infix-firewall:firewall"
#define XPATH_NTP_BASE "/ietf-ntp:ntp"
#define XPATH_PTP_BASE "/ieee1588-ptp-tt:ptp"

TAILQ_HEAD(sub_head, sub);

struct sub {
	struct ev_io watcher;
	sr_subscription_ctx_t *sr_sub;

	TAILQ_ENTRY(sub)
	entries;
};

struct statd {
	struct sub_head subs;
	sr_session_ctx_t *sr_ses;        /* Provider session with callbacks */
	sr_conn_ctx_t *sr_conn;          /* Connection (owns YANG context) */
	struct ev_loop *ev_loop;
	struct journal_ctx journal;      /* Periodic operational snapshots */
	struct mdns_ctx mdns;            /* mDNS neighbor monitor */
};

static int ly_add_yanger_data(const struct ly_ctx *ctx, struct lyd_node **parent,
			      char *yanger_args[])
{
	FILE *stream;
	int err;
	int fd;

	fd = memfd_create("yanger_tmpfile", MFD_CLOEXEC | MFD_NOEXEC_SEAL);
	if (fd == -1) {
		ERROR("Error, unable to create memfd");
		return SR_ERR_SYS;
	}

	/* Wrap the file descriptor in a FILE stream for fwrite */
	stream = fdopen(fd, "w+");
	if (stream == NULL) {
		ERROR("Error, unable to fdopen memfd");
		close(fd);
		return SR_ERR_SYS;
	}

	err = fsystemv(yanger_args, NULL, stream, NULL);
	if (err) {
		ERROR("Error, running yanger");
		fclose(stream);
		return SR_ERR_SYS;
	}

	fflush(stream);

	if (lseek(fd, 0, SEEK_SET) == (off_t)-1) {
		ERROR("Error, unable reset stream (seek)");
		fclose(stream);
		return SR_ERR_SYS;
	}

	err = lyd_parse_data_fd(ctx, fd, LYD_JSON, LYD_PARSE_ONLY, 0, parent);
	if (err)
		ERROR("Error, parsing yanger data (%d): %s", err, ly_errmsg(ctx));

	fclose(stream);
	/* Note: fclose() already closes the underlying fd from fdopen() */

	return err;
}

static char *xpath_extract(const char *xpath, const char *key)
{
	char *res = NULL;
	const char *ptr;
	const char *end;

	/* (also checks if key exist) */
	ptr = strstr(xpath, key);
	if (!ptr)
		return NULL;

	ptr += strlen(key);

	end = strchr(ptr, '\'');
	if (!end) {
		ERROR("Can't find end quote for %s (sanity check)", key);
		return NULL;
	}

	if ((end - ptr) >= XPATH_MAX) {
		ERROR("Value for %s is to long (sanity check)", key);
		return NULL;
	}

	res = calloc((end - ptr) + 1, sizeof(char));
	if (!res)
		return NULL;

	strncpy(res, ptr, end - ptr);
	res[end - ptr] = '\0';

	return res;
}

static int sr_iface_cb(sr_session_ctx_t *session, uint32_t, const char *model,
			 const char *, const char *xpath, uint32_t,
			 struct lyd_node **parent, __attribute__((unused)) void *priv)
{
	char *yanger_args[5] = {
		YANGER_BINPATH,
		(char *)model,
		NULL,
		NULL,
		NULL
	};
	char *ifname = NULL;
	const struct ly_ctx *ctx;
	sr_conn_ctx_t *con;
	int err;

	DEBUG("Incoming interface query for xpath: %s", xpath);

	con = sr_session_get_connection(session);
	if (!con) {
		ERROR("Error, getting sr connection");
		return SR_ERR_INTERNAL;
	}

	ctx = sr_acquire_context(con);
	if (!ctx) {
		ERROR("Error, acquiring context");
		return SR_ERR_INTERNAL;
	}

	ifname = xpath_extract(xpath, "[name='");
	if (ifname) {
		yanger_args[2] = "-p";
		yanger_args[3] = ifname;
	}
	err = ly_add_yanger_data(ctx, parent, yanger_args);
	if (err)
		ERROR("Error adding interface yanger data");

	sr_release_context(con);

	return SR_ERR_OK;
}

static int sr_generic_cb(sr_session_ctx_t *session, uint32_t, const char *model,
			 const char *, const char *xpath, uint32_t,
			 struct lyd_node **parent, __attribute__((unused)) void *priv)
{
	char *yanger_args[5] = {
		YANGER_BINPATH,
		(char *)model,
		NULL
	};
	const struct ly_ctx *ctx;
	sr_conn_ctx_t *con;
	sr_error_t err;

	DEBUG("Incoming generic query for xpath: %s", xpath);

	con = sr_session_get_connection(session);
	if (!con) {
		ERROR("Error, getting sr connection");
		return SR_ERR_INTERNAL;
	}

	ctx = sr_acquire_context(con);
	if (!ctx) {
		ERROR("Error, acquiring context");
		return SR_ERR_INTERNAL;
	}

	err = ly_add_yanger_data(ctx, parent, yanger_args);
	if (err)
		ERROR("Error adding yanger data");

	sr_release_context(con);

	return err;
}

static int sr_ospf_cb(sr_session_ctx_t *session, uint32_t, const char *,
		      const char *, const char *xpath, uint32_t,
		      struct lyd_node **parent, __attribute__((unused)) void *priv)
{
	char *yanger_args[5] = {
		YANGER_BINPATH,
		"ietf-ospf",
		NULL
	};
	const struct ly_ctx *ctx;
	sr_conn_ctx_t *con;
	sr_error_t err;

	DEBUG("Incoming ospf query for xpath: %s", xpath);

	con = sr_session_get_connection(session);
	if (!con) {
		ERROR("Error, getting sr connection");
		return SR_ERR_INTERNAL;
	}

	ctx = sr_acquire_context(con);
	if (!ctx) {
		ERROR("Error, acquiring context");
		return SR_ERR_INTERNAL;
	}

	err = ly_add_yanger_data(ctx, parent, yanger_args);
	if (err)
		ERROR("Error adding yanger data");

	sr_release_context(con);

	return err;
}

static int sr_rip_cb(sr_session_ctx_t *session, uint32_t, const char *,
		     const char *, const char *xpath, uint32_t,
		     struct lyd_node **parent, __attribute__((unused)) void *priv)
{
	char *yanger_args[5] = {
		YANGER_BINPATH,
		"ietf-rip",
		NULL
	};
	const struct ly_ctx *ctx;
	sr_conn_ctx_t *con;
	sr_error_t err;

	DEBUG("Incoming rip query for xpath: %s", xpath);

	con = sr_session_get_connection(session);
	if (!con) {
		ERROR("Error, getting sr connection");
		return SR_ERR_INTERNAL;
	}

	ctx = sr_acquire_context(con);
	if (!ctx) {
		ERROR("Error, acquiring context");
		return SR_ERR_INTERNAL;
	}

	err = ly_add_yanger_data(ctx, parent, yanger_args);
	if (err)
		ERROR("Error adding yanger data");

	sr_release_context(con);

	return err;
}

static int sr_bfd_cb(sr_session_ctx_t *session, uint32_t, const char *,
		     const char *, const char *xpath, uint32_t,
		     struct lyd_node **parent, __attribute__((unused)) void *priv)
{
	char *yanger_args[5] = {
		YANGER_BINPATH,
		"ietf-bfd-ip-sh",
		NULL
	};
	const struct ly_ctx *ctx;
	sr_conn_ctx_t *con;
	sr_error_t err;

	DEBUG("Incoming BFD query for xpath: %s", xpath);

	con = sr_session_get_connection(session);
	if (!con) {
		ERROR("Error, getting sr connection");
		return SR_ERR_INTERNAL;
	}

	ctx = sr_acquire_context(con);
	if (!ctx) {
		ERROR("Error, acquiring context");
		return SR_ERR_INTERNAL;
	}

	err = ly_add_yanger_data(ctx, parent, yanger_args);
	if (err)
		ERROR("Error adding yanger data");

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

static void sighup_cb(struct ev_loop *, struct ev_signal *w, int)
{
	struct statd *statd = w->data;

	mdns_ctx_reconnect(&statd->mdns);
}


static void sr_event_cb(struct ev_loop *, struct ev_io *w, int)
{
	struct sub *sub = (struct sub *)w->data;

	sr_subscription_process_events(sub->sr_sub, NULL, NULL);
}

static int subscribe(struct statd *statd, char *model, char *xpath,
		     int (*cb)(sr_session_ctx_t *session, uint32_t, const char *, const char *,
		     const char *, uint32_t, struct lyd_node **parent, void *priv))
{
	struct sub *sub;
	int sr_ev_pipe;
	sr_error_t err;

	sub = malloc(sizeof(struct sub));
	memset(sub, 0, sizeof(struct sub));

	DEBUG("Subscribe to events for \"%s\"", xpath);
	err = sr_oper_get_subscribe(statd->sr_ses, model, xpath, cb, sub,
				    SR_SUBSCR_DEFAULT | SR_SUBSCR_NO_THREAD | SR_SUBSCR_DONE_ONLY,
				    &sub->sr_sub);
	if (err) {
		ERROR("Error, subscribing to path \"%s\": %s", xpath, sr_strerror(err));
		free(sub);
		return err;
	}

	err = sr_get_event_pipe(sub->sr_sub, &sr_ev_pipe);
	if (err) {
		ERROR("Error, getting sysrepo event pipe: %s", sr_strerror(err));
		sr_unsubscribe(sub->sr_sub);
		free(sub);
		return err;
	}

	TAILQ_INSERT_TAIL(&statd->subs, sub, entries);

	ev_io_init(&sub->watcher, sr_event_cb, sr_ev_pipe, EV_READ);
	sub->watcher.data = sub;
	ev_io_start(statd->ev_loop, &sub->watcher);

	return SR_ERR_OK;
}

static void sub_delete(struct ev_loop *loop, struct sub_head *subs, struct sub *sub)
{
	TAILQ_REMOVE(subs, sub, entries);
	ev_io_stop(loop, &sub->watcher);
	sr_unsubscribe(sub->sr_sub);
	free(sub);
}

static void unsub_to_all(struct statd *statd)
{
	struct sub *sub;

	while (!TAILQ_EMPTY(&statd->subs)) {
		sub = TAILQ_FIRST(&statd->subs);
		sub_delete(statd->ev_loop, &statd->subs, sub);
	}
}

static int subscribe_to_all(struct statd *statd)
{
	DEBUG("Attempting to subscribe to all");

	if (subscribe(statd, "ietf-routing", XPATH_ROUTING_TABLE, sr_generic_cb))
		return SR_ERR_INTERNAL;
	if (subscribe(statd, "ietf-interfaces", XPATH_IFACE_BASE, sr_iface_cb))
		return SR_ERR_INTERNAL;
	if (subscribe(statd, "ietf-routing", XPATH_ROUTING_OSPF, sr_ospf_cb))
		return SR_ERR_INTERNAL;
	if (subscribe(statd, "ietf-routing", XPATH_ROUTING_RIP, sr_rip_cb))
		return SR_ERR_INTERNAL;
	if (subscribe(statd, "ietf-routing", XPATH_ROUTING_BFD, sr_bfd_cb))
		return SR_ERR_INTERNAL;
	if (subscribe(statd, "ietf-hardware", XPATH_HARDWARE_BASE, sr_generic_cb))
		return SR_ERR_INTERNAL;
	if (subscribe(statd, "ietf-system", XPATH_SYSTEM_BASE":system", sr_generic_cb))
		return SR_ERR_INTERNAL;
	if (subscribe(statd, "ietf-system", XPATH_SYSTEM_BASE":system-state", sr_generic_cb))
		return SR_ERR_INTERNAL;
	if (subscribe(statd, "ieee802-dot1ab-lldp", XPATH_LLDP_BASE, sr_generic_cb))
		return SR_ERR_INTERNAL;
#ifdef CONTAINERS
	if (subscribe(statd, "infix-containers", XPATH_CONTAIN_BASE, sr_generic_cb))
		return SR_ERR_INTERNAL;
#endif
	if (subscribe(statd, "infix-dhcp-server", XPATH_DHCP_SERVER_BASE, sr_generic_cb))
		return SR_ERR_INTERNAL;
	if (subscribe(statd, "infix-firewall", XPATH_FIREWALL_BASE, sr_generic_cb))
		return SR_ERR_INTERNAL;
	if (subscribe(statd, "ietf-ntp", XPATH_NTP_BASE, sr_generic_cb))
		return SR_ERR_INTERNAL;
	if (subscribe(statd, "ieee1588-ptp-tt", XPATH_PTP_BASE, sr_generic_cb))
		return SR_ERR_INTERNAL;

	INFO("Successfully subscribed to all models");
	return SR_ERR_OK;
}

static void version_print(void)
{
	printf("statd - status daemon v%s, compiled with libsysrepo v%s\n\n",
	       STATD_VERSION, SR_VERSION);
}

static void help_print(void)
{
	printf("Usage:\n"
	       "  statd [-h] [-V] [-v <level>]\n"
	       "\n"
	       "Options:\n"
	       "  -h, --help           Prints usage help.\n"
	       "  -V, --version        Prints version information.\n"
	       "  -v, --verbosity <level>\n"
	       "                       Change verbosity to a level (none, error, warning, info, debug).\n"
	       "\n");
}

int main(int argc, char *argv[])
{
	struct ev_signal sigint_watcher, sigusr1_watcher, sighup_watcher;
	int log_opts = LOG_PID | LOG_NDELAY;
	int log_level = LOG_NOTICE;
	struct statd statd = {};
	int opt;
	int err;

	struct option options[] = {
		{"help",      no_argument,       NULL, 'h'},
		{"version",   no_argument,       NULL, 'V'},
		{"verbosity", required_argument, NULL, 'v'},
		{NULL,        0,                 NULL, 0},
	};

	opterr = 0;
	while ((opt = getopt_long(argc, argv, "hVv:", options, NULL)) != -1) {
		switch (opt) {
		case 'h':
			version_print();
			help_print();
			return EXIT_SUCCESS;
		case 'V':
			version_print();
			return EXIT_SUCCESS;
		case 'v':
			if (!strcmp(optarg, "none"))
				log_level = LOG_EMERG;
			else if (!strcmp(optarg, "error"))
				log_level = LOG_ERR;
			else if (!strcmp(optarg, "warning"))
				log_level = LOG_WARNING;
			else if (!strcmp(optarg, "info"))
				log_level = LOG_INFO;
			else if (!strcmp(optarg, "debug")) {
				log_level = LOG_DEBUG;
				debug = 1;
			} else {
				fprintf(stderr, "statd error: Invalid verbosity \"%s\"\n", optarg);
				return EXIT_FAILURE;
			}
			break;
		default:
			fprintf(stderr, "statd error: Invalid option or missing argument: -%c\n", optopt);
			return EXIT_FAILURE;
		}
	}

	if (optind < argc) {
		fprintf(stderr, "statd error: Redundant parameters\n");
		return EXIT_FAILURE;
	}

	if (getenv("DEBUG")) {
		log_opts |= LOG_PERROR;
		debug = 1;
	}

	openlog("statd", log_opts, LOG_DAEMON);
	setlogmask(LOG_UPTO(log_level));

	TAILQ_INIT(&statd.subs);
	statd.ev_loop = EV_DEFAULT;

	INFO("Status daemon starting");

	err = sr_connect(SR_CONN_DEFAULT, &statd.sr_conn);
	if (err) {
		ERROR("Error, connecting to sysrepo: %s", sr_strerror(err));
		return EXIT_FAILURE;
	}
	DEBUG("Connected to sysrepo");

	/* Provider session with operational callbacks */
	err = sr_session_start(statd.sr_conn, SR_DS_OPERATIONAL, &statd.sr_ses);
	if (err) {
		ERROR("Error, start provider session: %s", sr_strerror(err));
		sr_disconnect(statd.sr_conn);
		return EXIT_FAILURE;
	}
	DEBUG("Provider session started (%p)", statd.sr_ses);

	err = subscribe_to_all(&statd);
	if (err) {
		sr_session_stop(statd.sr_ses);
		sr_disconnect(statd.sr_conn);
		return EXIT_FAILURE;
	}

	ev_signal_init(&sigint_watcher, sigint_cb, SIGINT);
	sigint_watcher.data = &statd;
	ev_signal_start(statd.ev_loop, &sigint_watcher);

	ev_signal_init(&sigusr1_watcher, sigusr1_cb, SIGUSR1);
	sigusr1_watcher.data = &statd;
	ev_signal_start(statd.ev_loop, &sigusr1_watcher);

	ev_signal_init(&sighup_watcher, sighup_cb, SIGHUP);
	sighup_watcher.data = &statd;
	ev_signal_start(statd.ev_loop, &sighup_watcher);

	err = journal_start(&statd.journal, statd.ev_loop);
	if (err) {
		sr_session_stop(statd.sr_ses);
		sr_disconnect(statd.sr_conn);
		return EXIT_FAILURE;
	}

	if (mdns_ctx_init(&statd.mdns, statd.ev_loop, statd.sr_conn))
		INFO("mDNS neighbor monitoring not available");

	/* Signal readiness to Finit */
	pidfile(NULL);

	INFO("Status daemon entering main event loop");
	ev_run(statd.ev_loop, 0);

	/* We should never get here during normal operation */
	INFO("Status daemon shutting down");

	mdns_ctx_exit(&statd.mdns);
	journal_stop(&statd.journal);

	unsub_to_all(&statd);
	sr_session_stop(statd.sr_ses);
	sr_disconnect(statd.sr_conn);

	return EXIT_SUCCESS;
}
