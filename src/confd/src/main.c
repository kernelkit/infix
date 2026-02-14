/* SPDX-License-Identifier: BSD-3-Clause */
/*
 * confd - Infix configuration daemon
 *
 * Replaces sysrepo-plugind + bootstrap + load with a single binary.
 * One sr_connect(), all datastore operations in-process, then load
 * plugins and enter the event loop.
 *
 * Copyright (c) 2018 - 2021 Deutsche Telekom AG.
 * Copyright (c) 2018 - 2021 CESNET, z.s.p.o.
 */

#include <dirent.h>
#include <dlfcn.h>
#include <errno.h>
#include <ev.h>
#include <fcntl.h>
#include <getopt.h>
#include <glob.h>
#include <poll.h>
#include <pthread.h>
#include <stdint.h>
#include <signal.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

#include <jansson.h>
#include <libyang/libyang.h>
#include <libite/lite.h>
#include <sysrepo.h>
#include <sysrepo/version.h>

/* Callback type names from sysrepo plugin API */
#define SRP_INIT_CB    "sr_plugin_init_cb"
#define SRP_CLEANUP_CB "sr_plugin_cleanup_cb"

#ifndef CONFD_VERSION
#define CONFD_VERSION PACKAGE_VERSION
#endif

#ifndef SRPD_PLUGINS_PATH
#define SRPD_PLUGINS_PATH "/usr/lib/sysrepo-plugind/plugins"
#endif

struct plugin {
	void *handle;
	char *name;
	int (*init_cb)(sr_session_ctx_t *session, void **private_data);
	void (*cleanup_cb)(sr_session_ctx_t *session, void *private_data);
	void (*get_subs)(void *priv, sr_subscription_ctx_t **sub, sr_subscription_ctx_t **fsub);
	void *private_data;
	sr_subscription_ctx_t *sub;
	sr_subscription_ctx_t *fsub;
	int initialized;
};

/* Maximum number of sysrepo event pipe file descriptors across all plugins */
#define MAX_EVENT_FDS 64

static void error_print(int sr_error, const char *format, ...)
{
	va_list ap;
	char msg[2048];

	if (!sr_error)
		snprintf(msg, sizeof(msg), "confd error: %s\n", format);
	else
		snprintf(msg, sizeof(msg), "confd error: %s (%s)\n", format, sr_strerror(sr_error));

	va_start(ap, format);
	vfprintf(stderr, msg, ap);
	va_end(ap);
}

/* Finit style progress output on console */
static void conout(int rc, const char *fmt, ...)
{
	const char *sta = "%s\e[1m[\e[1;%dm%s\e[0m\e[1m]\e[0m %s";
	const char *msg[] = { " OK ", "FAIL", "WARN", " ⋯  " };
	const int col[] = { 32, 31, 33, 33 };
	const char *cr = rc == 3 ? "" : "\r";
	char buf[80];
	va_list ap;

	snprintf(buf, sizeof(buf), sta, cr, col[rc], msg[rc], fmt);
	va_start(ap, fmt);
	vfprintf(stderr, buf, ap);
	va_end(ap);
}

static void version_print(void)
{
	printf("confd - Infix configuration daemon v%s, compiled with libsysrepo v%s\n\n",
	       CONFD_VERSION, SR_VERSION);
}

static void help_print(void)
{
	printf(
		"Usage:\n"
		"  confd [-h] [-V] [-v <level>] [-d] [-n] [-f] [-p pidfile]\n"
		"        [-F factory-config] [-S startup-config] [-E failure-config]\n"
		"        [-t timeout]\n"
		"\n"
		"Options:\n"
		"  -h, --help           Prints usage help.\n"
		"  -V, --version        Prints version information.\n"
		"  -v, --verbosity <level>\n"
		"                       Change verbosity to a level (none, error, warning, info, debug) or\n"
		"                       number (0, 1, 2, 3, 4).\n"
		"  -d, --debug          Debug mode - not daemonized and logs to stderr.\n"
		"  -n, --foreground     Run in foreground and log to syslog.\n"
		"  -f, --fatal-plugin-fail\n"
		"                       Terminate if any plugin initialization fails.\n"
		"  -p, --pid-file <path>\n"
		"                       Create a PID file at the specified path.\n"
		"  -F, --factory-config <path>\n"
		"                       Factory default config file (default: /etc/factory-config.cfg).\n"
		"  -S, --startup-config <path>\n"
		"                       Startup config file (default: /cfg/startup-config.cfg).\n"
		"  -E, --failure-config <path>\n"
		"                       Failure fallback config file (default: /etc/failure-config.cfg).\n"
		"  -t, --timeout <ms>   Sysrepo operation timeout in seconds (default: 60).\n"
		"\n"
		"Environment variable $SRPD_PLUGINS_PATH overwrites the default plugins directory.\n"
		"\n");
}

static void ignore_signals(void)
{
	struct sigaction action;

	memset(&action, 0, sizeof(action));
	action.sa_handler = SIG_IGN;
	sigaction(SIGPIPE, &action, NULL);
	sigaction(SIGTSTP, &action, NULL);
	sigaction(SIGTTIN, &action, NULL);
	sigaction(SIGTTOU, &action, NULL);
}

/* libev callbacks for steady-state operation */
static void signal_cb(struct ev_loop *loop, struct ev_signal *w, int revents)
{
	(void)revents;
	(void)w;
	ev_break(loop, EVBREAK_ALL);
}

static void sr_event_cb(struct ev_loop *loop, struct ev_io *w, int revents)
{
	(void)loop;
	(void)revents;
	sr_subscription_process_events(w->data, NULL, NULL);
}

/*
 * Temporary event pump thread for bootstrap.
 *
 * With SR_SUBSCR_NO_THREAD, sysrepo writes events to a pipe and waits
 * for the application to call sr_subscription_process_events().  During
 * bootstrap, sr_replace_config() blocks waiting for callbacks — this
 * thread ensures those callbacks get dispatched.
 */
struct event_pump {
	struct plugin *plugins;
	int            plugin_count;
	int            running;
};

static void *event_pump_thread(void *arg)
{
	struct event_pump *pump = arg;
	struct pollfd fds[MAX_EVENT_FDS];
	sr_subscription_ctx_t *subs[MAX_EVENT_FDS];
	int nfds = 0;

	for (int i = 0; i < pump->plugin_count; i++) {
		struct plugin *p = &pump->plugins[i];

		if (p->sub && sr_get_event_pipe(p->sub, &fds[nfds].fd) == SR_ERR_OK) {
			fds[nfds].events = POLLIN;
			subs[nfds] = p->sub;
			nfds++;
		}
		if (p->fsub && sr_get_event_pipe(p->fsub, &fds[nfds].fd) == SR_ERR_OK) {
			fds[nfds].events = POLLIN;
			subs[nfds] = p->fsub;
			nfds++;
		}
	}

	while (pump->running) {
		if (poll(fds, nfds, 100) > 0) {
			for (int i = 0; i < nfds; i++)
				if (fds[i].revents & POLLIN)
					sr_subscription_process_events(subs[i], NULL, NULL);
		}
	}

	return NULL;
}

static void quiet_now(void)
{
	int fd = -1;

	fd = open("/dev/null", O_RDWR, 0);
	if (fd != -1) {
		dup2(fd, STDIN_FILENO);
		dup2(fd, STDOUT_FILENO);
		dup2(fd, STDERR_FILENO);
		close(fd);
	}

	nice(0);
}

static void daemon_init(int debug, sr_log_level_t log_level)
{
	pid_t pid = 0, sid = 0;

	nice(-20);

	ignore_signals();

	if (debug) {
		if (debug < 0)
			goto done;
		sr_log_stderr(log_level);
		return;
	}

	pid = fork();
	if (pid < 0) {
		error_print(0, "fork() failed (%s).", strerror(errno));
		exit(EXIT_FAILURE);
	}
	if (pid > 0)
		exit(EXIT_SUCCESS);

	sid = setsid();
	if (sid < 0) {
		error_print(0, "setsid() failed (%s).", strerror(errno));
		exit(EXIT_FAILURE);
	}

	if (chdir("/") < 0) {
		error_print(0, "chdir() failed (%s).", strerror(errno));
		exit(EXIT_FAILURE);
	}

	quiet_now();
done:
	sr_log_syslog("confd", log_level);
}

static int open_pidfile(const char *pidfile)
{
	int pidfd;

	pidfd = open(pidfile, O_RDWR | O_CREAT, 0640);
	if (pidfd < 0) {
		error_print(0, "Unable to open the PID file \"%s\" (%s).", pidfile, strerror(errno));
		return -1;
	}

	if (lockf(pidfd, F_TLOCK, 0) < 0) {
		if (errno == EACCES || errno == EAGAIN)
			error_print(0, "Another instance of confd is running.");
		else
			error_print(0, "Unable to lock the PID file \"%s\" (%s).", pidfile, strerror(errno));
		close(pidfd);
		return -1;
	}

	return pidfd;
}

static int write_pidfile(int pidfd)
{
	char pid[30] = {0};
	int pid_len;

	if (ftruncate(pidfd, 0)) {
		error_print(0, "Failed to truncate pid file (%s).", strerror(errno));
		return -1;
	}

	snprintf(pid, sizeof(pid) - 1, "%ld\n", (long)getpid());
	pid_len = strlen(pid);
	if (write(pidfd, pid, pid_len) < pid_len) {
		error_print(0, "Failed to write PID into pid file (%s).", strerror(errno));
		return -1;
	}

	return 0;
}

/*
 * Plugin loading -- external .so files only (no internal plugins)
 */
static size_t path_len_no_ext(const char *path)
{
	const char *dot;

	dot = strrchr(path, '.');
	if (!dot || dot == path)
		return 0;

	return dot - path;
}

static int load_plugins(struct plugin **plugins, int *plugin_count)
{
	void *mem, *handle;
	struct plugin *plugin;
	DIR *dir;
	struct dirent *ent;
	const char *plugins_dir;
	char *path;
	size_t name_len;
	int rc = 0;

	*plugins = NULL;
	*plugin_count = 0;

	plugins_dir = getenv("SRPD_PLUGINS_PATH");
	if (!plugins_dir)
		plugins_dir = SRPD_PLUGINS_PATH;

	dir = opendir(plugins_dir);
	if (!dir) {
		error_print(0, "Opening \"%s\" directory failed (%s).", plugins_dir, strerror(errno));
		return -1;
	}

	while ((ent = readdir(dir))) {
		if (!strcmp(ent->d_name, ".") || !strcmp(ent->d_name, ".."))
			continue;

		if (asprintf(&path, "%s/%s", plugins_dir, ent->d_name) == -1) {
			error_print(0, "asprintf() failed (%s).", strerror(errno));
			rc = -1;
			break;
		}
		handle = dlopen(path, RTLD_LAZY);
		if (!handle) {
			error_print(0, "Opening plugin \"%s\" failed (%s).", path, dlerror());
			free(path);
			rc = -1;
			break;
		}
		free(path);

		mem = realloc(*plugins, (*plugin_count + 1) * sizeof(**plugins));
		if (!mem) {
			error_print(0, "realloc() failed (%s).", strerror(errno));
			dlclose(handle);
			rc = -1;
			break;
		}
		*plugins = mem;
		plugin = &(*plugins)[*plugin_count];
		memset(plugin, 0, sizeof(*plugin));

		*(void **)&plugin->init_cb = dlsym(handle, SRP_INIT_CB);
		if (!plugin->init_cb) {
			error_print(0, "Failed to find function \"%s\" in plugin \"%s\".",
				    SRP_INIT_CB, ent->d_name);
			dlclose(handle);
			rc = -1;
			break;
		}

		*(void **)&plugin->cleanup_cb = dlsym(handle, SRP_CLEANUP_CB);
		if (!plugin->cleanup_cb) {
			error_print(0, "Failed to find function \"%s\" in plugin \"%s\".",
				    SRP_CLEANUP_CB, ent->d_name);
			dlclose(handle);
			rc = -1;
			break;
		}

		/* Optional: allows main to collect subscription contexts */
		*(void **)&plugin->get_subs = dlsym(handle, "confd_get_subscriptions");

		plugin->handle = handle;

		name_len = path_len_no_ext(ent->d_name);
		if (name_len == 0) {
			error_print(0, "Wrong filename \"%s\".", ent->d_name);
			dlclose(handle);
			rc = -1;
			break;
		}

		plugin->name = strndup(ent->d_name, name_len);
		if (!plugin->name) {
			error_print(0, "strndup() failed.");
			dlclose(handle);
			rc = -1;
			break;
		}

		++(*plugin_count);
	}

	closedir(dir);
	return rc;
}

/*
 * Wipe stale sysrepo SHM files for a clean slate every boot.
 */
static void wipe_sysrepo_shm(void)
{
	glob_t gl;

	if (glob("/dev/shm/sr_*", 0, NULL, &gl) == 0) {
		for (size_t i = 0; i < gl.gl_pathc; i++)
			unlink(gl.gl_pathv[i]);
		globfree(&gl);
	}
}

const char *basenm(const char *path)
{
	const char *slash;

	if (!path)
		return NULL;

	slash = strrchr(path, '/');
	if (slash)
		return slash[1] ? slash + 1 : NULL;

	return path;
}

/*
 * Append error message to login banners.
 */
static void banner_append(const char *msg)
{
	const char *files[] = {
		"/etc/banner",
		"/etc/issue",
		"/etc/issue.net",
	};

	for (size_t i = 0; i < sizeof(files) / sizeof(files[0]); i++) {
		FILE *fp = fopen(files[i], "a");

		if (fp) {
			fprintf(fp, "\n%s\n", msg);
			fclose(fp);
		}
	}
}

/*
 * Smart migration: only fork+exec the migrate script if the version
 * in the config file doesn't match the current confd version.
 */
static int maybe_migrate(const char *path)
{
	const char *backup_dir = "/cfg/backup";
	json_t *root, *meta, *ver;
	const char *file_ver;
	char backup[256];
	int rc;

	root = json_load_file(path, 0, NULL);
	if (!root)
		return -1;

	meta     = json_object_get(root, "infix-meta:meta");
	ver      = meta ? json_object_get(meta, "version") : NULL;
	file_ver = ver ? json_string_value(ver) : "0.0";

	if (!strcmp(file_ver, CONFD_VERSION)) {
		json_decref(root);
		return 0;
	}
	json_decref(root);

	SRPLG_LOG_INF("confd", "%s config version %s vs confd %s, migrating ...",
		      path, file_ver, CONFD_VERSION);

	mkpath(backup_dir, 0770);
	chown(backup_dir, 0, 10); /* root:wheel */

	snprintf(backup, sizeof(backup), "%s/%s", backup_dir, basenm(path));
	rc = systemf("migrate -i -b \"%s\" \"%s\"", backup, path);
	if (rc)
		SRPLG_LOG_ERR("confd", "Migration of %s failed (rc=%d)", path, rc);

	return rc;
}

static int file_exists(const char *path)
{
	return access(path, F_OK) == 0;
}

/*
 * Load a JSON config file into the running datastore.
 * Mirrors what sysrepocfg -I does: lyd_parse_data() + sr_replace_config().
 */
static int load_config(sr_conn_ctx_t *conn, sr_session_ctx_t *sess,
		       const char *path, uint32_t timeout_ms)
{
	const struct ly_ctx *ly_ctx;
	struct lyd_node *data = NULL;
	struct ly_in *in = NULL;
	LY_ERR lyrc;
	int r;

	ly_ctx = sr_acquire_context(conn);

	lyrc = ly_in_new_filepath(path, 0, &in);
	if (lyrc == LY_EINVAL) {
		/* empty file */
		char *empty = strdup("");

		ly_in_new_memory(empty, &in);
	} else if (lyrc) {
		error_print(0, "Failed to open \"%s\" for reading", path);
		sr_release_context(conn);
		return -1;
	}

	lyrc = lyd_parse_data(ly_ctx, NULL, in, LYD_JSON,
			      LYD_PARSE_NO_STATE | LYD_PARSE_ONLY | LYD_PARSE_STRICT,
			      0, &data);
	ly_in_free(in, 1);

	if (lyrc) {
		SRPLG_LOG_ERR("confd", "Parsing %s failed", path);
		sr_release_context(conn);
		return -1;
	}

	sr_release_context(conn);

	r = sr_replace_config(sess, NULL, data, timeout_ms);
	if (r != SR_ERR_OK) {
		SRPLG_LOG_ERR("confd", "sr_replace_config failed: %s", sr_strerror(r));
		return -1;
	}

	return 0;
}

/*
 * Export running datastore to a JSON file.
 */
static int export_running(sr_session_ctx_t *sess, const char *path, uint32_t timeout_ms)
{
	sr_data_t *data = NULL;
	FILE *fp;
	int r;

	r = sr_get_data(sess, "/*", 0, timeout_ms, 0, &data);
	if (r != SR_ERR_OK) {
		SRPLG_LOG_ERR("confd", "sr_get_data failed: %s", sr_strerror(r));
		return -1;
	}

	umask(0006);
	fp = fopen(path, "w");
	if (!fp) {
		SRPLG_LOG_ERR("confd", "Failed to open %s for writing: %s", path, strerror(errno));
		sr_release_data(data);
		return -1;
	}

	lyd_print_file(fp, data ? data->tree : NULL, LYD_JSON, LYD_PRINT_SIBLINGS);
	fclose(fp);
	sr_release_data(data);

	chown(path, 0, 10);    /* root:wheel for admin group access */

	return 0;
}

/*
 * Handle startup-config load failure: revert to factory-default,
 * then load failure-config, set error banners.
 */
static void handle_startup_failure(sr_session_ctx_t *sess, const char *failure_path,
				   sr_conn_ctx_t *conn, uint32_t timeout_ms)
{
	int r;

	SRPLG_LOG_ERR("confd", "Failed loading startup-config, reverting to Fail Secure mode!");

	/* Reset to factory-default */
	r = sr_copy_config(sess, NULL, SR_DS_FACTORY_DEFAULT, timeout_ms);
	if (r != SR_ERR_OK) {
		SRPLG_LOG_ERR("confd", "sr_copy_config(factory-default) failed: %s", sr_strerror(r));
		/* Nuclear option: wipe everything */
		systemf("rm -f /etc/sysrepo/data/*startup* /etc/sysrepo/data/*running* /dev/shm/sr_*");
		return;
	}

	/* Load failure-config on top */
	if (file_exists(failure_path)) {
		if (load_config(conn, sess, failure_path, timeout_ms)) {
			SRPLG_LOG_ERR("confd", "Failed loading failure-config, aborting!");
			banner_append("CRITICAL ERROR: Logins are disabled, no credentials available");
			systemf("initctl -nbq runlevel 9");
			return;
		}
	}

	banner_append("ERROR: Corrupt startup-config, system has reverted to default login credentials");
}

/*
 * Enable test-mode if the test-mode marker exists.
 */
static void maybe_enable_test_mode(void)
{
	if (file_exists("/mnt/aux/test-mode")) {
		int rc;

		conout(3, "Enbling test mode");
		rc = systemf("sysrepoctl -c infix-test -e test-mode-enable");
		conout(rc ? 1 : 0, "\n");
	}
}

/*
 * Determine which config to load:
 * - test-mode (unless override exists)
 * - startup-config
 * - first-boot from factory
 */
static int bootstrap_config(sr_conn_ctx_t *conn, sr_session_ctx_t *sess,
			    const char *factory_path, const char *startup_path,
			    const char *failure_path, const char *test_path,
			    uint32_t timeout_ms)
{
	const char *config_path;
	int r;

	/* Test mode support */
	if (file_exists("/mnt/aux/test-mode")) {
		if (file_exists("/mnt/aux/test-override-startup")) {
			unlink("/mnt/aux/test-override-startup");
			config_path = startup_path;
		} else {
			SRPLG_LOG_INF("confd", "Test mode detected, switching to test-config");
			config_path = test_path;
		}
	} else {
		config_path = startup_path;
	}

	if (file_exists(config_path)) {
		/* Run migration if needed */
		maybe_migrate(config_path);

		/* Load startup (or test) config */
		SRPLG_LOG_INF("confd", "Loading %s ...", config_path);
		if (load_config(conn, sess, config_path, timeout_ms)) {
			handle_startup_failure(sess, failure_path, conn, timeout_ms);
			return 0; /* continue running even in fail-secure */
		}

		SRPLG_LOG_INF("confd", "Loaded %s successfully, syncing startup datastore.", config_path);
		sr_session_switch_ds(sess, SR_DS_STARTUP);
		r = sr_copy_config(sess, NULL, SR_DS_RUNNING, timeout_ms);
		sr_session_switch_ds(sess, SR_DS_RUNNING);
		if (r != SR_ERR_OK)
			SRPLG_LOG_WRN("confd", "Failed to sync startup datastore: %s", sr_strerror(r));

		return 0;
	}

	/* First boot: no startup-config, initialize from factory */
	SRPLG_LOG_INF("confd", "startup-config missing, initializing from factory-config");

	r = sr_copy_config(sess, NULL, SR_DS_FACTORY_DEFAULT, timeout_ms);
	if (r != SR_ERR_OK) {
		SRPLG_LOG_ERR("confd", "sr_copy_config(factory-default) failed: %s", sr_strerror(r));
		return -1;
	}

	/* Export running → startup file */
	if (export_running(sess, startup_path, timeout_ms))
		SRPLG_LOG_WRN("confd", "Failed to export running to %s", startup_path);

	return 0;
}

static void *gen_config_thread(void *arg)
{
	(void)arg;
	return (void *)(intptr_t)systemf("/usr/libexec/confd/gen-config");
}

int main(int argc, char **argv)
{
	struct plugin *plugins = NULL;
	sr_conn_ctx_t *conn = NULL;
	sr_session_ctx_t *sess = NULL;
	sr_log_level_t log_level = SR_LL_ERR;
	int plugin_count = 0, i, r, rc = EXIT_FAILURE, opt, debug = 0;
	int pidfd = -1, fatal_fail = 0;
	pthread_t tid;
	void *tret;
	const char *pidfile = NULL;
	const char *factory_path = "/etc/factory-config.cfg";
	const char *startup_path = "/cfg/startup-config.cfg";
	const char *failure_path = "/etc/failure-config.cfg";
	const char *test_path = "/etc/test-config.cfg";
	uint32_t timeout_s = 60;
	uint32_t timeout_ms;

	struct option options[] = {
		{"help",              no_argument,       NULL, 'h'},
		{"version",           no_argument,       NULL, 'V'},
		{"verbosity",         required_argument, NULL, 'v'},
		{"debug",             no_argument,       NULL, 'd'},
		{"foreground",        no_argument,       NULL, 'n'},
		{"pid-file",          required_argument, NULL, 'p'},
		{"fatal-plugin-fail", no_argument,       NULL, 'f'},
		{"factory-config",    required_argument, NULL, 'F'},
		{"startup-config",    required_argument, NULL, 'S'},
		{"failure-config",    required_argument, NULL, 'E'},
		{"timeout",           required_argument, NULL, 't'},
		{NULL,                0,                 NULL, 0},
	};

	opterr = 0;
	while ((opt = getopt_long(argc, argv, "hVv:dnp:fF:S:E:t:", options, NULL)) != -1) {
		switch (opt) {
		case 'h':
			version_print();
			help_print();
			rc = EXIT_SUCCESS;
			goto cleanup;
		case 'V':
			version_print();
			rc = EXIT_SUCCESS;
			goto cleanup;
		case 'v':
			if (!strcmp(optarg, "none"))
				log_level = SR_LL_NONE;
			else if (!strcmp(optarg, "error"))
				log_level = SR_LL_ERR;
			else if (!strcmp(optarg, "warning"))
				log_level = SR_LL_WRN;
			else if (!strcmp(optarg, "info"))
				log_level = SR_LL_INF;
			else if (!strcmp(optarg, "debug"))
				log_level = SR_LL_DBG;
			else if (strlen(optarg) == 1 && optarg[0] >= '0' && optarg[0] <= '4')
				log_level = atoi(optarg);
			else {
				error_print(0, "Invalid verbosity \"%s\"", optarg);
				goto cleanup;
			}
			break;
		case 'd':
			debug = 1;
			break;
		case 'n':
			debug = -1;
			break;
		case 'p':
			pidfile = optarg;
			break;
		case 'f':
			fatal_fail = 1;
			break;
		case 'F':
			factory_path = optarg;
			break;
		case 'S':
			startup_path = optarg;
			break;
		case 'E':
			failure_path = optarg;
			break;
		case 't':
			timeout_s = (uint32_t)atoi(optarg);
			break;
		default:
			error_print(0, "Invalid option or missing argument: -%c", optopt);
			goto cleanup;
		}
	}

	if (optind < argc) {
		error_print(0, "Redundant parameters");
		goto cleanup;
	}

	timeout_ms = timeout_s * 1000;

	if (pidfile && (pidfd = open_pidfile(pidfile)) < 0)
		goto cleanup;

	/* Load plugins from disk (dlopen) before daemonizing */
	if (load_plugins(&plugins, &plugin_count))
		error_print(0, "load_plugins failed (continuing)");

	/* Daemonize -- after this point, confd no longer logs to stderr */
	daemon_init(debug, log_level);

	/* Start gen-config in parallel -- thread is joined before we need the result */
	conout(3, "Generating factory-config and failure-config");
	if (pthread_create(&tid, NULL, gen_config_thread, NULL)) {
		SRPLG_LOG_ERR("confd", "Failed to create gen-config thread: %s", strerror(errno));
		conout(1, "\n");
		goto cleanup;
	}

	/* Phase 1: Wipe stale SHM for a clean slate */
	wipe_sysrepo_shm();

	/* Phase 2: Connect to sysrepo (rebuilds SHM from installed YANG modules) */
	r = sr_connect(0, &conn);
	if (r != SR_ERR_OK) {
		error_print(r, "Failed to connect");
		goto cleanup;
	}

	/* Phase 3: Wait for gen-config thread to finish */
	pthread_join(tid, &tret);
	if ((intptr_t)tret != 0) {
		SRPLG_LOG_ERR("confd", "gen-config failed (rc=%d)", (int)(intptr_t)tret);
		conout(1, "\n");
		goto cleanup;
	}
	conout(0, "\n");

	/* Phase 4: Install factory defaults into all datastores */
	SRPLG_LOG_INF("confd", "Loading factory-default datastore from %s ...", factory_path);
	conout(3, "Loading factory-default datastore");
	r = sr_install_factory_config(conn, factory_path);
	if (r != SR_ERR_OK) {
		SRPLG_LOG_ERR("confd", "sr_install_factory_config failed: %s", sr_strerror(r));
		conout(1, "\n");
		goto cleanup;
	}
	conout(0, "\n");

	/* Phase 5: Start running-datastore session */
	r = sr_session_start(conn, SR_DS_RUNNING, &sess);
	if (r != SR_ERR_OK) {
		error_print(r, "Failed to start new session");
		goto cleanup;
	}

	/* Phase 6: Clear running datastore so plugin init sees an empty
	 * tree.  This matches the original bootstrap flow where running
	 * was cleared with '{}' before sysrepo-plugind started.  When we
	 * later load startup-config, the diff will be all-create which is
	 * what the plugin callbacks expect. */
	r = sr_replace_config(sess, NULL, NULL, timeout_ms);
	if (r != SR_ERR_OK) {
		SRPLG_LOG_ERR("confd", "Failed to clear running datastore: %s", sr_strerror(r));
		goto cleanup;
	}

	/* Enable test-mode YANG feature if needed */
	maybe_enable_test_mode();

	/* Phase 7: Initialize plugins (subscribe to YANG module changes) */
	conout(3, "Loading confd plugins");
	for (i = 0; i < plugin_count; i++) {
		r = plugins[i].init_cb(sess, &plugins[i].private_data);
		if (r) {
			SRPLG_LOG_ERR("confd", "Plugin \"%s\" initialization failed (%s).",
				      plugins[i].name, sr_strerror(r));
			if (fatal_fail) {
				conout(1, "\n");
				goto cleanup;
			}
		} else {
			SRPLG_LOG_INF("confd", "Plugin \"%s\" initialized.", plugins[i].name);
			plugins[i].initialized = 1;
		}
	}
	conout(0, "\n");

	/* Phase 8: Collect subscription contexts from plugins */
	for (i = 0; i < plugin_count; i++) {
		if (plugins[i].initialized && plugins[i].get_subs)
			plugins[i].get_subs(plugins[i].private_data,
					    &plugins[i].sub, &plugins[i].fsub);
	}

	/* Phase 9: Start event pump thread for bootstrap.
	 * With SR_SUBSCR_NO_THREAD, sr_replace_config() blocks waiting
	 * for callbacks.  The pump thread processes those events. */
	struct event_pump pump = {
		.plugins      = plugins,
		.plugin_count = plugin_count,
		.running      = 1,
	};
	pthread_t pump_tid;

	if (pthread_create(&pump_tid, NULL, event_pump_thread, &pump)) {
		SRPLG_LOG_ERR("confd", "Failed to create event pump thread: %s", strerror(errno));
		goto cleanup;
	}

	/*
	 * Phase 10: Load startup config -- plugins are now subscribed, so
	 * sr_replace_config() will trigger their change callbacks.
	 * The event pump thread processes those callbacks.
	 */
	conout(3, "Loading startup-config");
	if (bootstrap_config(conn, sess, factory_path, startup_path,
			     failure_path, test_path, timeout_ms)) {
		pump.running = 0;
		pthread_join(pump_tid, NULL);
		conout(1, "\n");
		goto cleanup;
	}
	conout(0, "\n");

	/* Phase 11: Stop event pump — bootstrap is done */
	pump.running = 0;
	pthread_join(pump_tid, NULL);

	/* No more progress to show, go to quiet daemon mode */
	quiet_now();

	/* Signal that bootstrap is complete (dbus, resolvconf depend on this) */
	symlink("/run/finit/cond/reconf", "/run/finit/cond/usr/bootstrap");

	/* Write PID file after everything is ready */
	if (pidfile && write_pidfile(pidfd) < 0)
		goto cleanup;

	/* Phase 12: Steady-state — libev event loop replaces pthread_cond_wait */
	{
		struct ev_loop *loop = EV_DEFAULT;
		struct ev_signal sigterm_w, sigint_w, sighup_w, sigquit_w;
		struct ev_io io_watchers[MAX_EVENT_FDS];
		int nio = 0;

		ev_signal_init(&sigterm_w, signal_cb, SIGTERM);
		ev_signal_init(&sigint_w,  signal_cb, SIGINT);
		ev_signal_init(&sighup_w,  signal_cb, SIGHUP);
		ev_signal_init(&sigquit_w, signal_cb, SIGQUIT);
		ev_signal_start(loop, &sigterm_w);
		ev_signal_start(loop, &sigint_w);
		ev_signal_start(loop, &sighup_w);
		ev_signal_start(loop, &sigquit_w);

		for (i = 0; i < plugin_count; i++) {
			int fd;

			if (plugins[i].sub && sr_get_event_pipe(plugins[i].sub, &fd) == SR_ERR_OK) {
				ev_io_init(&io_watchers[nio], sr_event_cb, fd, EV_READ);
				io_watchers[nio].data = plugins[i].sub;
				ev_io_start(loop, &io_watchers[nio]);
				nio++;
			}
			if (plugins[i].fsub && sr_get_event_pipe(plugins[i].fsub, &fd) == SR_ERR_OK) {
				ev_io_init(&io_watchers[nio], sr_event_cb, fd, EV_READ);
				io_watchers[nio].data = plugins[i].fsub;
				ev_io_start(loop, &io_watchers[nio]);
				nio++;
			}
		}

		ev_run(loop, 0);
		ev_loop_destroy(loop);
	}

	rc = EXIT_SUCCESS;

cleanup:
	while (plugin_count > 0) {
		if (plugins[plugin_count - 1].initialized)
			plugins[plugin_count - 1].cleanup_cb(sess, plugins[plugin_count - 1].private_data);
		if (plugins[plugin_count - 1].handle)
			dlclose(plugins[plugin_count - 1].handle);
		free(plugins[plugin_count - 1].name);
		--plugin_count;
	}
	free(plugins);

	if (pidfd >= 0) {
		close(pidfd);
		unlink(pidfile);
	}

	sr_disconnect(conn);
	return rc;
}
