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

#include "util.h"

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

static const char *prognm = "copy";
static const char *remote_user;
static int timeout;
static int dry_run;
static int sanitize;

/*
 * Current system user, same as sysrepo user
 */
static const char *getuser(void)
{
	const struct passwd *pw;
	uid_t uid;

	uid = getuid();
	pw = getpwuid(uid);
	if (!pw) {
		perror("getpwuid");
		exit(1);
	}

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
		perror("in_group() malloc");
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
		int _errno = errno;
		const struct group *gr = getgrgid(gid);

		fprintf(stderr, ERRMSG "setting group owner %s (%d) on %s: %s\n",
			gr ? gr->gr_name : "<unknown>", gid, fn, strerror(_errno));
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

		fprintf(stderr, ERRMSG "removal of temporary file %s failed\n", path);
	}
}


static void sysrepo_print_error(sr_session_ctx_t *sess)
{
	const sr_error_info_t *erri = NULL;
	int err;

	err = sr_session_get_error(sess, &erri);
	if (err || !erri || !erri->err_count)
		return;

	fprintf(stderr, ERRMSG "%s (%d)\n", erri->err->message, erri->err->err_code);
}

static sr_session_ctx_t *sysrepo_session(const struct infix_ds *ds)
{
	static sr_session_ctx_t *sess;

	sr_subscription_ctx_t *sub = NULL;
	const char *user = getuser();
	sr_conn_ctx_t *conn = NULL;
	int err;

	if (!ds) {
		if (!sess)
			return NULL;

		conn = sr_session_get_connection(sess);
		sr_session_stop(sess);
		sr_disconnect(conn);
		return NULL;
	}

	if (!sess) {
		err = sr_connect(0, &conn);
		if (err != SR_ERR_OK) {
			sysrepo_print_error(sess);
			fprintf(stderr, ERRMSG "could not connect to %s\n", ds->name);
			goto err;
		}

		/* Always open running, because sr_nacm_init() does not work
		 * against the factory DS.
		 */
		err = sr_session_start(conn, SR_DS_RUNNING, &sess);
		if (err != SR_ERR_OK) {
			sysrepo_print_error(sess);
			fprintf(stderr, ERRMSG "%s session setup failed\n", ds->name);
			goto err_disconnect;
		}

		err = sr_nacm_init(sess, 0, &sub);
		if (err != SR_ERR_OK) {
			sysrepo_print_error(sess);
			fprintf(stderr, ERRMSG "%s NACM setup failed\n", ds->name);
			goto err_stop;
		}

		err = sr_nacm_set_user(sess, user);
		if (err != SR_ERR_OK) {
			sysrepo_print_error(sess);
			fprintf(stderr, ERRMSG "%s NACM setup for %s failed\n", ds->name, user);
			goto err_nacm_destroy;
		}
	}

	err = sr_session_switch_ds(sess, ds->datastore);
	if (err) {
		sysrepo_print_error(sess);
		fprintf(stderr, ERRMSG "%s activation failed\n", ds->name);
		return NULL;
	}

	return sess;

err_nacm_destroy:
	sr_nacm_destroy();
err_stop:
	sr_session_stop(sess);
err_disconnect:
	sr_disconnect(conn);
err:
	sess = NULL;
	return NULL;
}

static int sysrepo_export(const struct infix_ds *ds, const char *path)
{
	sr_session_ctx_t *sess;
	sr_data_t *data;
	int err;

	sess = sysrepo_session(ds);
	if (!sess)
		return 1;

	err = sr_get_data(sess, "/*", 0, timeout * 1000, SR_OPER_DEFAULT, &data);
	if (err) {
		sysrepo_print_error(sess);
		fprintf(stderr, ERRMSG "retrieval of %s data failed\n", ds->name);
		return err;
	}

	err = lyd_print_path(path, data->tree, LYD_JSON, LYD_PRINT_SIBLINGS);
	sr_release_data(data);
	if (err) {
		sysrepo_print_error(sess);
		fprintf(stderr, ERRMSG "failed to store %s data\n", ds->name);
		return err;
	}

	return 0;
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
		fprintf(stderr, ERRMSG "failed to parse %s data\n", ds->name);
		goto out;
	}

	err = dry_run ? 0 : sr_replace_config(sess, NULL, data, timeout * 1000);
	if (err) {
		sysrepo_print_error(sess);
		fprintf(stderr, ERRMSG "failed import %s data\n", ds->name);
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
		fprintf(stderr, ERRMSG "upload to %s failed\n", uri);
		return 1;
	}

	return 0;
}

static int curl_download(const char *uri, const char *dstpath)
{
	char download[] = "-o";

	if (curl(download, dstpath, uri)) {
		fprintf(stderr, ERRMSG "download of %s failed\n", uri);
		return 1;
	}

	return 0;
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
		fprintf(stderr, ERRMSG "failed to save %s\n", dstpath);
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
		fprintf(stderr, ERRMSG "no such file %s.", *src);
		return 1;
	}

	*rm = false;
	return 0;
}

static int resolve_dst(const char **dst, const struct infix_ds **ds, char **path)
{
	*dst = infix_ds(*dst, ds);

	if (*ds) {
		if (!(*ds)->rw) {
			fprintf(stderr, ERRMSG "%s is not writable", (*ds)->name);
			return 1;
		}

		if (!(*ds)->path)
			return 0;

		*path = strdup((*ds)->path);
	} else if (is_uri(*dst)) {
		return 0;
	} else {
		*path = cfg_adjust(*dst, NULL, sanitize);
	}

	if (!*path) {
		fprintf(stderr, ERRMSG "no such file: %s", *dst);
		return 1;
	}

	if (!*ds && !access(*path, F_OK) && !yorn("Overwrite existing file %s", *path)) {
		fprintf(stderr, "OK, aborting.\n");
		return 1;
	}

	return 0;
}

static int copy(const char *src, const char *dst)
{
	char *srcpath = NULL, *dstpath = NULL;
	const struct infix_ds *srcds, *dstds;
	bool rmsrc = false;
	mode_t oldmask;
	int err = 1;

	/* rw for user and group only */
	oldmask = umask(0006);

	if (!strcmp(src, dst)) {
		fprintf(stderr, ERRMSG "source and destination are the same, aborting.\n");
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

	free(dstpath);
	free(srcpath);

	sync();
	umask(oldmask);
	return err;
}

static int usage(int rc)
{
	printf("Usage: %s [OPTIONS] SRC DST\n"
	       "\n"
	       "Options:\n"
	       "  -h              This help text\n"
	       "  -n              Dry-run, validate configuration without applying\n"
	       "  -s              Sanitize paths for CLI use (restrict path traversal)\n"
	       "  -t SEC          Timeout for the operation, or default %d sec\n"
	       "  -u USER         Username for remote commands, like scp\n"
	       "  -v              Show version\n"
	       "\n"
	       "Files:\n"
	       "  SRC             JSON configuration file, or a datastore\n"
	       "  DST             A file or datastore, except factory-config\n"
	       "\n"
	       "Datastores:\n"
	       "  running-config  The running datastore, current active config\n"
	       "  startup-config  The non-volatile config used at startup\n"
	       "  factory-config  The device's factory default configuration\n"
	       "\n", prognm, timeout);

	return rc;
}

int main(int argc, char *argv[])
{
	const char *src = NULL, *dst = NULL;
	int c;

	timeout = fgetint("/etc/default/confd", "=", "CONFD_TIMEOUT");

	while ((c = getopt(argc, argv, "hnst:u:v")) != EOF) {
		switch(c) {
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
		}
	}

	if (timeout < 0)
		timeout = 120;

	if (argc - optind != 2)
		return usage(1);

	src = argv[optind++];
	dst = argv[optind++];

	return copy(src, dst);
}
