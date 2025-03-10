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

#include <sysrepo.h>
#include <sysrepo/netconf_acm.h>

#include "util.h"

struct infix_ds {
	char *name;		/* startup-config, etc.  */
	char *sysrepocfg;	/* ds name in sysrepocfg */
	int   datastore;	/* sr_datastore_t and -1 */
	int   rw;		/* read-write:1 or not:0 */
	char *path;		/* local path or NULL    */
};

struct infix_ds infix_config[] = {
	{ "startup-config",     "startup",          SR_DS_STARTUP,         1, "/cfg/startup-config.cfg" },
	{ "running-config",     "running",          SR_DS_RUNNING,         1, NULL },
	{ "candidate-config",   "candidate",        SR_DS_CANDIDATE,       1, NULL },
	{ "operational-config", "operational",      SR_DS_OPERATIONAL,     1, NULL },
	{ "factory-config",     "factory-default",  SR_DS_FACTORY_DEFAULT, 0, NULL }
};

static const char *prognm = "copy";
static int timeout;


/*
 * Print sysrepo session errors followed by an optional string.
 */
static void emsg(sr_session_ctx_t *sess, const char *fmt, ...)
{
	const sr_error_info_t *err = NULL;
	va_list ap;
	size_t i;
	int rc;

	if (!sess)
		goto end;

	rc = sr_session_get_error(sess, &err);
	if ((rc != SR_ERR_OK) || !err)
		goto end;

	// Show the first error only. Because probably next errors are
	// originated from internal sysrepo code but is not from subscribers.
//	for (i = 0; i < err->err_count; i++)
	for (i = 0; i < (err->err_count < 1 ? err->err_count : 1); i++)
		fprintf(stderr, ERRMSG "%s\n", err->err[i].message);
end:
	if (fmt) {
		va_start(ap, fmt);
		vfprintf(stderr, fmt, ap);
		va_end(ap);
	}
}

/*
 * Current system user, same as sysrepo user
 */
static char *getuser(void)
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
static gid_t in_group(const char *user, const char *fn, gid_t *gid)
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
		const struct group *gr = getgrgid(gid);

		fprintf(stderr, ERRMSG "setting group owner %s (%d) on %s: %s\n",
			gr ? gr->gr_name : "<unknown>", gid, fn, strerror(errno));
	}
}

static const char *infix_ds(const char *text, struct infix_ds **ds)
{
	size_t i, len = strlen(text);

	for (i = 0; i < NELEMS(infix_config); i++) {
		if (!strncmp(infix_config[i].name, text, len)) {
			*ds = &infix_config[i];
			return infix_config[i].name;
		}
	}

	return text;
}


static int copy(const char *src, const char *dst, const char *remote_user)
{
	struct infix_ds *srcds = NULL, *dstds = NULL;
	char temp_file[20] = "/tmp/copy.XXXXXX";
	const char *tmpfn = NULL;
	sr_session_ctx_t *sess;
	const char *fn = NULL;
	sr_conn_ctx_t *conn;
	const char *user;
	char adjust[256];
	mode_t oldmask;
	int rc = 0;

	/* rw for user and group only */
	oldmask = umask(0006);

	src = infix_ds(src, &srcds);
	if (!src)
		goto err;
	dst = infix_ds(dst, &dstds);
	if (!dst)
		goto err;

	if (!strcmp(src, dst)) {
		fprintf(stderr, ERRMSG "source and destination are the same, aborting.");
		goto err;
	}

	user = getuser();

	/* 1. Regular ds copy */
	if (srcds && dstds) {
		/* Ensure the dst ds is writable */
		if (!dstds->rw) {
			fprintf(stderr, ERRMSG "not possible to write to \"%s\", skipping.\n", dst);
			rc = 1;
			goto err;
		}

		if (sr_connect(SR_CONN_DEFAULT, &conn)) {
			fprintf(stderr, ERRMSG "connection to datastore failed\n");
			rc = 1;
			goto err;
		}

		sr_log_syslog("klishd", SR_LL_WRN);

		if (sr_session_start(conn, dstds->datastore, &sess)) {
			fprintf(stderr, ERRMSG "unable to open transaction to %s\n", dst);
		} else {
			sr_nacm_set_user(sess, user);
			rc = sr_copy_config(sess, NULL, srcds->datastore, timeout * 1000);
			if (rc)
				emsg(sess, ERRMSG "unable to copy configuration, err %d: %s\n",
				     rc, sr_strerror(rc));
			else
				set_owner(dstds->path, user);
		}
		rc = sr_disconnect(conn);

		if (!srcds->path || !dstds->path)
			goto err; /* done, not an error */

		/* allow copy factory startup */
	}

	if (srcds) {
		/* 2. Export from a datastore somewhere else */
		if (strstr(dst, "://")) {
			if (srcds->path)
				fn = srcds->path;
			else {
				snprintf(adjust, sizeof(adjust), "/tmp/%s.cfg", srcds->name);
				fn = tmpfn = adjust;
				rc = systemf("sysrepocfg -d %s -X%s -f json", srcds->sysrepocfg, fn);
			}

			if (rc)
				fprintf(stderr, ERRMSG "failed exporting %s to %s\n", src, fn);
			else {
				rc = systemf("curl %s -LT %s %s", remote_user, fn, dst);
				if (rc)
					fprintf(stderr, ERRMSG "failed uploading %s to %s\n", src, dst);
				else
					set_owner(dst, user);
			}
			goto err;
		}

		if (dstds && dstds->path)
			fn = dstds->path;
		else
			fn = cfg_adjust(dst, src, adjust, sizeof(adjust));

		if (!fn) {
			fprintf(stderr, ERRMSG "invalid destination path.\n");
			rc = -1;
			goto err;
		}

		if (!access(fn, F_OK) && !yorn("Overwrite existing file %s", fn)) {
			fprintf(stderr, "OK, aborting.\n");
			return 0;
		}

		if (srcds->path)
			rc = systemf("cp %s %s", srcds->path, fn);
		else
			rc = systemf("sysrepocfg -d %s -X%s -f json", srcds->sysrepocfg, fn);
		if (rc)
			fprintf(stderr, ERRMSG "failed copy %s to %s\n", src, fn);
		else
			set_owner(fn, user);
	} else if (dstds) {
		if (!dstds->sysrepocfg) {
			fprintf(stderr, ERRMSG "not possible to import to this datastore.\n");
			rc = 1;
			goto err;
		}
		if (!dstds->rw) {
			fprintf(stderr, ERRMSG "not possible to write to %s", dst);
			goto err;
		}

		/* 3. Import from somewhere to a datastore */
		if (strstr(src, "://")) {
			tmpfn = mktemp(temp_file);
			fn = tmpfn;
		} else {
			fn = cfg_adjust(src, NULL, adjust, sizeof(adjust));
			if (!fn) {
				fprintf(stderr, ERRMSG "invalid source file location.\n");
				rc = 1;
				goto err;
			}
		}

		if (tmpfn)
			rc = systemf("curl %s -Lo %s %s", remote_user, fn, src);
		if (rc) {
			fprintf(stderr, ERRMSG "failed downloading %s", src);
		} else {
			rc = systemf("sysrepocfg -d %s -I%s -f json", dstds->sysrepocfg, fn);
			if (rc)
				fprintf(stderr, ERRMSG "failed loading %s from %s", dst, src);
		}
	} else {
		if (strstr(src, "://") && strstr(dst, "://")) {
			fprintf(stderr, ERRMSG "copy from remote to remote is not supported.\n");
			goto err;
		}

		if (strstr(src, "://")) {
			fn = cfg_adjust(dst, src, adjust, sizeof(adjust));
			if (!fn) {
				fprintf(stderr, ERRMSG "invalid destination file location.\n");
				rc = 1;
				goto err;
			}

			if (!access(fn, F_OK)) {
				if (!yorn("Overwrite existing file %s", fn)) {
					fprintf(stderr, "OK, aborting.\n");
					return 0;
				}
			}

			rc = systemf("curl %s -Lo %s %s", remote_user, fn, src);
		} else if (strstr(dst, "://")) {
			fn = cfg_adjust(src, NULL, adjust, sizeof(adjust));
			if (!fn) {
				fprintf(stderr, ERRMSG "invalid source file location.\n");
				rc = 1;
				goto err;
			}

			if (access(fn, F_OK))
				fprintf(stderr, ERRMSG "no such file %s, aborting.", fn);
			else
				rc = systemf("curl %s -LT %s %s", remote_user, fn, dst);
		} else {
			if (!access(dst, F_OK)) {
				if (!yorn("Overwrite existing file %s", dst)) {
					fprintf(stderr, "OK, aborting.\n");
					return 0;
				}
			}
			rc = systemf("cp %s %s", src, dst);
		}
	}

err:
	if (tmpfn)
		rc = remove(tmpfn);

	sync();			/* ensure command is flushed to disk */
	umask(oldmask);

	return rc;
}

static int usage(int rc)
{
	printf("Usage: %s [OPTIONS] SRC DST\n"
	       "\n"
	       "Options:\n"
	       "  -h         This help text\n"
	       "  -u USER    Username for remote commands, like scp\n"
	       "  -t SEEC    Timeout for the operation, or default %d sec\n"
	       "  -v         Show version\n", prognm, timeout);

	return rc;
}

int main(int argc, char *argv[])
{
	const char *user = NULL, *src = NULL, *dst = NULL;
	int c;

	timeout = fgetint("/etc/default/confd", "=", "CONFD_TIMEOUT");

	while ((c = getopt(argc, argv, "ht:u:v")) != EOF) {
		switch(c) {
		case 'h':
			return usage(0);
		case 't':
			timeout = atoi(optarg);
			break;
		case 'u':
			user = optarg;
			break;
		case 'v':
			puts(PACKAGE_VERSION);
			return 0;
		}
	}

	if (timeout < 0)
		timeout = 120;

	if (optind >= argc)
		return usage(1);

	src = argv[optind++];
	dst = argv[optind++];

	return copy(src, dst, user);
}
