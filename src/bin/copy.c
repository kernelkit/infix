/* SPDX-License-Identifier: ISC */
#include "config.h"

#include <getopt.h>
#include <pwd.h>
#include <grp.h>
#include <stdio.h>
#include <stdarg.h>
#include <libgen.h>

#include <sys/stat.h>
#include <sys/types.h>
#include <sys/wait.h>

#include <sysrepo.h>
#include <sysrepo/netconf_acm.h>
#include <sysrepo/values.h>

#include "util.h"

#define err(rc, fmt, args...)      { fprintf(stderr, ERRMSG fmt ":%s\n", ##args, strerror(errno)); exit(rc); }
#define errx(rc, fmt, args...)     { fprintf(stderr, ERRMSG fmt "\n", ##args);                     exit(rc); }
#define warnx(fmt, args...)          fprintf(stderr, ERRMSG fmt "\n", ##args)
#define warn(fmt, args...)           fprintf(stderr, ERRMSG fmt ":%s\n", ##args, strerror(errno))
#define dbg(fmt, args...) if (debug) fprintf(stderr, DBGMSG fmt "\n", ##args)

struct infix_ds {
	char *name;		/* startup-config, etc.  */
	int   datastore;	/* sr_datastore_t and -1 */
	bool  rw;		/* read-write:1 or not:0 */
	char *path;		/* local path or NULL    */
};

const struct infix_ds infix_config[] = {
	{ "startup-config",    SR_DS_STARTUP,         true, "/cfg/startup-config.cfg" },
	{ "running-config",    SR_DS_RUNNING,         true, NULL },
	/* { "candidate-config",  SR_DS_CANDIDATE,       true, NULL }, */
	{ "operational-state", SR_DS_OPERATIONAL,     false, NULL },
	{ "factory-config",    SR_DS_FACTORY_DEFAULT, false, NULL }
};

static const char *prognm;
static const char *remote_user;
static char *xpath = "/*";
static int debug;
static int force;
static int timeout;
static int dry_run;
static int sanitize;

/*
 * Current system user, same as sysrepo user.  We use getuid() here
 * because `copy` is SUID root to work around sysrepo issues with a
 * /dev/shm that's moounted 01777.
 */
static const char *getuser(void)
{
	const struct passwd *pw;
	uid_t uid = getuid();

	pw = getpwuid(uid);
	if (!pw)
		err(1, "failed querying user info for uid %d", uid);

	return pw->pw_name;
}

/*
 * If UNIX user is in UNIX group of directory containing file,
 * return 1, otherwise 0.
 *
 * E.g., writing to /cfg/foo, where /cfg is owned by root:wheel,
 * should result in the file being owned by $LOGNAME:wheel with
 * 0660 perms for other users in same group.
 */
static int in_group(const char *user, const char *fn, gid_t *gid)
{
	char path[PATH_MAX];
	const struct passwd *pw;
	int i, num = 0, rc = 0;
	struct stat st;
	gid_t *groups;
	char *dir;

	pw = getpwnam(user);
	if (!pw)
		return 0;

	strlcpy(path, fn, sizeof(path));
	dir = dirname(path);

	if (stat(dir, &st))
		return 0;

	num = NGROUPS_MAX;
	groups = malloc(num * sizeof(gid_t));
	if (!groups) {
		warn("failed in_group()");
		return 0;
	}

	getgrouplist(user, pw->pw_gid, groups, &num);
	for (i = 0; i < num; i++) {
		if (groups[i] == st.st_gid) {
			*gid = st.st_gid;
			rc = 1;
			break;
		}
	}
	free(groups);

	return rc;
}

/*
 * Set group owner so other users with same directory permissions can
 * read/write the file as well.  E.g., an 'admin' level user in group
 * 'wheel' writing a new file to `/cfg` should be possible to read and
 * write to by other administrators.
 *
 * This function is called only when the file has been successfully
 * copied or created in a file system directory.  This is why we can
 * safely ignore any EPERM errors to chown(), below, because if the file
 * already existed, created by another user, we are not allowed to chgrp
 * it.  The sole purpose of this function is to allow other users in the
 * same group to access the file in the future.
 */
static void set_owner(const char *fn, const char *user)
{
	gid_t gid = 9999;

	if (!fn)
		return;	/* not an error, e.g., running-config is not a file */

	if (!in_group(user, fn, &gid))
		return;	/* user not in parent directory's group */

	if (chown(fn, -1, gid) && errno != EPERM) {
		const struct group *gr = getgrgid(gid);

		warn("setting group owner %s (%d) on %s",
		     gr ? gr->gr_name : "<unknown>", gid, fn);
	}
}

static const char *infix_ds(const char *text, const struct infix_ds **ds)
{
	size_t i, len = strlen(text);

	for (i = 0; i < NELEMS(infix_config); i++) {
		if (!strncmp(infix_config[i].name, text, len)) {
			*ds = &infix_config[i];
			return infix_config[i].name;
		}
	}

	*ds = NULL;
	return text;
}

static bool is_uri(const char *str)
{
	return strstr(str, "://") != NULL;
}

static bool is_stdout(const char *path)
{
	if (!path)
		return 1;

	return  !strcmp(path, "-") ||
		!strcmp(path, "/dev/stdout") ||
		!strcmp(path, "/dev/fd/1");
}

static char *mktmp(void)
{
	mode_t oldmask;
	char *path;
	int fd;

	path = strdup("/tmp/copy-XXXXXX");
	if (!path)
		goto err;

	oldmask = umask(0077);
	fd = mkstemp(path);
	umask(oldmask);

	if (fd < 0)
		goto err;

	if (chown(path, getuid(), -1))
		dbg("Failed to chown %s: %s", path, strerror(errno));

	close(fd);
	return path;
err:
	free(path);
	return NULL;
}

static void rmtmp(const char *path)
{
	if (remove(path)) {
		if (errno == ENOENT)
			return;

		warn("failed removing temporary file %s", path);
	}
}

static void sysrepo_print_error(sr_session_ctx_t *sess)
{
	const sr_error_info_t *erri = NULL;
	int err;

	if (!sess)
		return;

	err = sr_session_get_error(sess, &erri);
	if (err || !erri || !erri->err_count)
		return;

	warnx("%s (%d)", erri->err->message, erri->err->err_code);
}

/* Connect to sysrepo and create NACM-aware session on running datastore */
static int sysrepo_init(sr_conn_ctx_t **conn, sr_session_ctx_t **sess,
				    sr_subscription_ctx_t **sub)
{
	const char *user = getuser();
	int err;

	err = sr_connect(SR_CONN_DEFAULT, conn);
	if (err != SR_ERR_OK) {
		warnx("failed connecting to sysrepo: %s", sr_strerror(err));
		return err;
	}

	/* Always open running, because sr_nacm_init() does not work
	 * against the factory DS.
	 */
	err = sr_session_start(*conn, SR_DS_RUNNING, sess);
	if (err != SR_ERR_OK) {
		warnx("failed starting session: %s", sr_strerror(err));
		goto fail;
	}

	err = sr_nacm_init(*sess, 0, sub);
	if (err != SR_ERR_OK) {
		warnx("NACM init failed: %s", sr_strerror(err));
		goto fail;
	}

	dbg("Setting NACM user %s for session", user);
	err = sr_nacm_set_user(*sess, user);
	if (err != SR_ERR_OK) {
		warnx("NACM setup failed for user %s: %s", user, sr_strerror(err));
		goto fail;
	}

	return SR_ERR_OK;
fail:
	sysrepo_print_error(*sess);
	sr_session_stop(*sess);
	sr_disconnect(*conn);

	return err;
}

static sr_session_ctx_t *sysrepo_session(const struct infix_ds *ds)
{
	static sr_subscription_ctx_t *sub = NULL;
	static sr_session_ctx_t *sess;
	sr_conn_ctx_t *conn = NULL;
	int err;

	if (!ds) {
		if (!sess)
			return NULL;

		conn = sr_session_get_connection(sess);
		sr_session_stop(sess);
		sr_disconnect(conn);
		sess = NULL;
		sub = NULL;
		return NULL;
	}

	if (!sess) {
		err = sysrepo_init(&conn, &sess, &sub);
		if (err != SR_ERR_OK) {
			warnx("Failed to initialize session for %s", ds->name);
			return NULL;
		}
	}

	err = sr_session_switch_ds(sess, ds->datastore);
	if (err) {
		sysrepo_print_error(sess);
		warnx("%s activation failed", ds->name);
		return NULL;
	}

	return sess;
}

static int sysrepo_export(const struct infix_ds *ds, const char *path)
{
	sr_session_ctx_t *sess;
	sr_data_t *data = NULL;
	int err;

	sess = sysrepo_session(ds);
	if (!sess)
		return 1;

	err = sr_get_data(sess, xpath, 0, timeout * 1000, SR_OPER_DEFAULT, &data);
	if (err) {
		sysrepo_print_error(sess);
		warnx("failed retrieving %s data", ds->name);
		return err;
	}

	if (!data)
		return 0;

	err = lyd_print_path(path, data->tree, LYD_JSON, LYD_PRINT_SIBLINGS);
	sr_release_data(data);

	if (err) {
		sysrepo_print_error(sess);
		warnx("failed storing %s data", ds->name);
	}

	return err;
}

static int sysrepo_import(const struct infix_ds *ds, const char *path)
{
	const struct ly_ctx *ly;
	sr_session_ctx_t *sess;
	struct lyd_node *data;
	int err;

	sess = sysrepo_session(ds);
	if (!sess)
		return 1;

	ly = sr_acquire_context(sr_session_get_connection(sess));

	err = lyd_parse_data_path(ly, path, LYD_JSON,
				  LYD_PARSE_NO_STATE | LYD_PARSE_ONLY |
				  LYD_PARSE_STORE_ONLY | LYD_PARSE_STRICT, 0, &data);
	if (err) {
		warnx("failed parsing %s data", ds->name);
		goto out;
	}

	err = dry_run ? 0 : sr_replace_config(sess, NULL, data, timeout * 1000);
	if (err) {
		sysrepo_print_error(sess);
		warnx("failed importing %s data, error %d", ds->name, err);
	}

out:
	sr_release_context(sr_session_get_connection(sess));
	return err ? 1 : 0;
	/* return sysrepo_do(sysrepo_import_op, ds, path) ? 1 : 0; */
}

static int subprocess(char * const *argv)
{
	int pid, status;

	pid = fork();
	if (!pid) {
		execvp(argv[0], argv);
		exit(1);
	}

	if (pid < 0)
		return 1;

	if (waitpid(pid, &status, 0) < 0)
		return 1;

	if (!WIFEXITED(status))
		return 1;

	return WEXITSTATUS(status);
}

static int curl(char *op, const char *path, const char *uri)
{
	char *argv[] =  {
		"curl", "-L", op, NULL, NULL, NULL, NULL, NULL,
	};
	int err = 1;

	argv[3] = strdup(path);
	argv[4] = strdup(uri);
	if (!(argv[3] && argv[4]))
		goto out;

	if (remote_user) {
		argv[5] = strdup("-u");
		argv[6] = strdup(remote_user);
		if (!(argv[5] && argv[6]))
			goto out;
	}
	err = subprocess(argv);

out:
	free(argv[6]);
	free(argv[5]);
	free(argv[4]);
	free(argv[3]);
	return err;
}

static int curl_upload(const char *srcpath, const char *uri)
{
	char upload[] = "-T";

	if (curl(upload, srcpath, uri)) {
		warnx("upload to %s failed", uri);
		return 1;
	}

	return 0;
}

static int curl_download(const char *uri, const char *dstpath)
{
	char download[] = "-o";
	int err;

	if ((err = curl(download, dstpath, uri))) {
		warnx("download of %s failed, exit code %d", uri, err);
		return 1;
	}

	return 0;
}

static int cat(const char *srcpath)
{
	char *argv[] = { "cat", NULL, NULL };
	int err;

	argv[1] = strdup(srcpath);
	if (!argv[1])
		return 1;

	err = subprocess(argv);
	if (err)
		warnx("failed writing to stdout, exit code %d", err);

	free(argv[1]);
	return err;
}

static int cp(const char *srcpath, const char *dstpath)
{
	char *argv[] =  {
		"cp", NULL, NULL, NULL,
	};
	int err = 1;

	argv[1] = strdup(srcpath);
	argv[2] = strdup(dstpath);
	if (!(argv[1] && argv[2]))
		goto out;

	err = subprocess(argv);
	if (err)
		warnx("failed to save %s, exit code %d", dstpath, err);
out:
	free(argv[2]);
	free(argv[1]);
	return err;
}

static int put(const char *srcpath, const char *dst,
	       const struct infix_ds *ds, const char *path)
{
	int err = 0;

	if (ds)
		err = sysrepo_import(ds, srcpath);
	else if (is_stdout(dst))
		err = cat(srcpath);
	else if (is_uri(dst))
		err = curl_upload(srcpath, dst);

	if (err)
		return err;

	if (path) {
		err = cp(srcpath, path);
		if (!err)
			set_owner(path, getuser());
	}

	return 0;
}

static int get(const char *src, const struct infix_ds *ds, const char *path)
{
	int err = 0;

	if (ds)
		err = sysrepo_export(ds, path);
	else if (is_uri(src))
		err = curl_download(src, path);

	return err;
}

static int resolve_src(const char **src, const struct infix_ds **ds, char **path, bool *rm)
{
	*src = infix_ds(*src, ds);

	if (*ds || is_uri(*src)) {
		*path = mktmp();
		if (!*path)
			return 1;

		*rm = true;
		return 0;
	} else {
		*path = cfg_adjust(*src, NULL, sanitize);
	}

	if (!*path) {
		warn("no such file %s", *src);
		return 1;
	}

	*rm = false;
	return 0;
}

static int resolve_dst(const char **dst, const struct infix_ds **ds, char **path)
{
	if (is_stdout(*dst) || is_uri(*dst))
		return 0;

	*dst = infix_ds(*dst, ds);

	if (*ds) {
		if (!(*ds)->rw) {
			warn("%s is not writable", (*ds)->name);
			return 1;
		}

		if (!(*ds)->path)
			return 0;

		*path = strdup((*ds)->path);
	} else {
		*path = cfg_adjust(*dst, NULL, sanitize);
	}

	if (!*path) {
		warn("no such file: %s", *dst);
		return 1;
	}

	if (!force && !*ds && !access(*path, F_OK) && !yorn("Overwrite existing file %s", *path)) {
		warnx("OK, aborting.");
		return 1;
	}

	return 0;
}

static int copy(const char *src, const char *dst)
{
	const struct infix_ds *srcds = NULL, *dstds = NULL;
	char *srcpath = NULL, *dstpath = NULL;
	bool rmsrc = false;
	mode_t oldmask;
	int err = 1;

	/* rw for user and group only */
	oldmask = umask(0006);

	if (dst && !strcmp(src, dst)) {
		warn("source and destination are the same, aborting.");
		goto err;
	}

	err = resolve_src(&src, &srcds, &srcpath, &rmsrc);
	if (err)
		goto err;

	err = resolve_dst(&dst, &dstds, &dstpath);
	if (err)
		goto err;

	err = get(src, srcds, srcpath);
	if (err)
		goto err;

	err = put(srcpath, dst, dstds, dstpath);

err:
	/* If either src or dst came from sysrepo, close the session */
	sysrepo_session(NULL);

	if (rmsrc)
		rmtmp(srcpath);

	if (dstpath)
		free(dstpath);
	free(srcpath);

	sync();
	umask(oldmask);
	return err;
}

static int usage(int rc)
{
	printf("Usage: %s [OPTIONS] SRC [DST]\n"
	       "\n"
	       "Options:\n"
	       "  -d                 Enable debug mode, verbose output on stderr\n"
	       "  -f                 Force yes when copying to a file that exists already\n"
	       "  -h                 This help text\n"
	       "  -n                 Dry-run, validate configuration without applying\n"
	       "  -s                 Sanitize paths for CLI use (restrict path traversal)\n"
	       "  -t SEC             Timeout for the operation, or default %d sec\n"
	       "  -u USER            Username for remote commands, like scp\n"
	       "  -v                 Show version\n"
	       "  -x PATH            XPath to copy, default: all\n"
	       "\n"
	       "Files:\n"
	       "  SRC                JSON configuration file, or a datastore\n"
	       "  DST                Optiional file or datastore, except factory-config,\n"
	       "                     when omitted output goes to stdout\n"
	       "\n"
	       "Datastores (short forms possible):\n"
	       "  running-config     The running datastore, current active config\n"
	       "  startup-config     The non-volatile config used at startup\n"
	       "  factory-config     The device's factory default configuration\n"
	       "  operational-state  Operational status and state data"
	       "\n"
	       "Examples:\n"
	       "  %s operational -x /system-state/software/boot-order\n"
	       "\n", prognm, timeout, prognm);

	return rc;
}

static int usage_rpc(int rc)
{
	printf("Usage: %s [OPTIONS] <rpc-xpath> [key value ...]\n"
	       "\n"
	       "Execute a YANG RPC/action with NACM enforcement.\n"
	       "\n"
	       "Options:\n"
	       "  -d                 Enable debug mode, verbose output on stderr\n"
	       "  -h                 This help text\n"
	       "  -t SEC             Timeout for the operation, or default %d sec\n"
	       "  -v                 Show version\n"
	       "\n"
	       "Arguments:\n"
	       "  rpc-xpath          RPC XPath (e.g., /ietf-system:set-current-datetime)\n"
	       "  key value          Pairs of RPC argument names and values\n"
	       "                     Values can be comma-separated for lists/leaf-lists\n"
	       "\n"
	       "Examples:\n"
	       "  %s /ietf-system:set-current-datetime current-datetime \"2025-01-01T00:00:00Z\"\n"
	       "  %s /infix-system:set-boot-order boot-order primary boot-order secondary\n"
	       "  %s /infix-system:set-boot-order boot-order primary,secondary,net\n"
	       "\n", prognm, timeout, prognm, prognm, prognm);

	return rc;
}

/* Execute RPC from CLI arguments: xpath and key-value pairs */
static int rpc_exec(const char *rpc_xpath, int argc, char *argv[])
{
	sr_subscription_ctx_t *sub = NULL;
	sr_conn_ctx_t *conn = NULL;
	sr_session_ctx_t *sess = NULL;
	sr_val_t *input = NULL;
	sr_val_t *output = NULL;
	size_t icnt = 0, ocnt = 0;
	int rc = 1, err, i;

	dbg("Executing RPC %s with %d arguments", rpc_xpath, argc / 2);

	err = sysrepo_init(&conn, &sess, &sub);
	if (err != SR_ERR_OK)
		return 1;

	for (i = 0; i < argc - 1; i += 2) {
		const char *key = argv[i];
		const char *val = argv[i + 1];
		char *val_copy, *token, *saveptr;

		/* Check if value contains commas - split into multiple values */
		if (strchr(val, ',')) {
			val_copy = strdup(val);
			if (!val_copy) {
				warnx("Memory allocation failed");
				goto cleanup;
			}

			token = strtok_r(val_copy, ",", &saveptr);
			while (token) {
				sr_realloc_values(icnt, icnt + 1, &input);
				sr_val_build_xpath(&input[icnt], "%s/%s", rpc_xpath, key);
				sr_val_set_str_data(&input[icnt], SR_STRING_T, token);
				dbg("Adding RPC argument %zu: %s = %s", icnt, input[icnt].xpath, token);
				icnt++;
				token = strtok_r(NULL, ",", &saveptr);
			}
			free(val_copy);
		} else {
			/* Single value */
			sr_realloc_values(icnt, icnt + 1, &input);
			sr_val_build_xpath(&input[icnt], "%s/%s", rpc_xpath, key);
			sr_val_set_str_data(&input[icnt], SR_STRING_T, val);
			dbg("Adding RPC argument %zu: %s = %s", icnt, input[icnt].xpath, val);
			icnt++;
		}
	}

	dbg("Sending RPC %s (timeout: %d ms)", rpc_xpath, timeout * 1000);
	err = sr_rpc_send(sess, rpc_xpath, input, icnt, timeout * 1000, &output, &ocnt);
	if (err != SR_ERR_OK) {
		sysrepo_print_error(sess);
		warnx("RPC execution failed: %s", sr_strerror(err));
		goto cleanup;
	}

	/* Print output if any */
	for (i = 0; i < (int)ocnt; i++) {
		sr_print_val(&output[i]);
		puts("");
	}

	rc = 0;

cleanup:
	sr_free_values(input, icnt);
	sr_free_values(output, ocnt);
	if (sub)
		sr_nacm_destroy();
	if (sess)
		sr_session_stop(sess);
	if (conn)
		sr_disconnect(conn);

	return rc;
}

static int copy_main(int argc, char *argv[])
{
	const char *dst = "/dev/stdout";
	const char *src = NULL;
	int c;

	timeout = fgetint("/etc/default/confd", "=", "CONFD_TIMEOUT");

	while ((c = getopt(argc, argv, "dfhnst:u:vx:")) != EOF) {
		switch(c) {
		case 'd':
			debug = 1;
			break;
		case 'f':
			force = 1;
			break;
		case 'h':
			return usage(0);
		case 'n':
			dry_run = 1;
			break;
		case 's':
			sanitize = 1;
			break;
		case 't':
			timeout = atoi(optarg);
			break;
		case 'u':
			remote_user = optarg;
			break;
		case 'v':
			puts(PACKAGE_VERSION);
			return 0;
		case 'x':
			xpath = optarg;
			break;
		}
	}

	if (timeout < 0)
		timeout = 120;

	switch (argc - optind) {
	case 2:
		src = argv[optind++];
		dst = argv[optind++];
		break;
	case 1:
		src = argv[optind++];
		break;
	default:
		return usage(1);
	}

	return copy(src, dst);
}

static int rpc_main(int argc, char *argv[])
{
	int c;

	timeout = fgetint("/etc/default/confd", "=", "CONFD_TIMEOUT");

	while ((c = getopt(argc, argv, "dht:v")) != EOF) {
		switch(c) {
		case 'd':
			debug = 1;
			break;
		case 'h':
			return usage_rpc(0);
		case 't':
			timeout = atoi(optarg);
			break;
		case 'v':
			puts(PACKAGE_VERSION);
			return 0;
		}
	}

	if (timeout < 0)
		timeout = 120;

	/* Require at least RPC xpath */
	if (optind >= argc) {
		warnx("Missing RPC xpath");
		return usage_rpc(1);
	}

	/* Validate RPC xpath starts with '/' */
	if (argv[optind][0] != '/') {
		warnx("RPC xpath must start with '/'");
		return usage_rpc(1);
	}

	/* Validate argument count (must be key-value pairs) */
	argc -= optind + 1;
	if (argc % 2 != 0) {
		warnx("Arguments must be key-value pairs after RPC xpath");
		return usage_rpc(1);
	}

	return rpc_exec(argv[optind], argc, &argv[optind + 1]);
}

int main(int argc, char *argv[])
{
	prognm = basename(argv[0]);

	if (!strcmp(prognm, "rpc"))
		return rpc_main(argc, argv);

	return copy_main(argc, argv);
}
