/* SPDX-License-Identifier: ISC */
#include "config.h"

#include <getopt.h>
#include <pwd.h>
#include <stdio.h>
#include <stdarg.h>

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
	{ "startup-config",     "startup",     SR_DS_STARTUP,         1, "/cfg/startup-config.cfg" },
	{ "running-config",     "running",     SR_DS_RUNNING,         1, NULL },
	{ "candidate-config",   "candidate",   SR_DS_CANDIDATE,       1, NULL },
	{ "operational-config", "operational", SR_DS_OPERATIONAL,     1, NULL },
	{ "factory-config",     "factory-default",          SR_DS_FACTORY_DEFAULT, 0, NULL }
};

static const char *prognm = "copy";


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

static void set_owner(const char *fn, const char *user)
{
	struct passwd *pw;

	pw = getpwnam(user);
	if (!pw) {
		fprintf(stderr, ERRMSG "setting owner %s on %s: %s\n", fn, user, strerror(errno));
		return;
	}

	chmod(fn, 0660);
	chown(fn, pw->pw_uid, pw->pw_gid);
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


static int copy(const char *src, const char *dst, const char *user)
{
	struct infix_ds *srcds = NULL, *dstds = NULL;
	char temp_file[20] = "/tmp/copy.XXXXXX";
	const char *tmpfn = NULL;
	sr_session_ctx_t *sess;
	const char *fn = NULL;
	const char *username;
	sr_conn_ctx_t *conn;
	char adjust[256];
	mode_t oldmask;
	int rc = 0;

	oldmask = umask(0660);

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

	username = getuser();

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
			sr_nacm_set_user(sess, username);
			rc = sr_copy_config(sess, NULL, srcds->datastore, 0);
			if (rc)
				emsg(sess, ERRMSG "unable to copy configuration, err %d: %s\n",
				     rc, sr_strerror(rc));
			else
				set_owner(dstds->path, username);
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
				rc = systemf("curl %s -LT %s %s", user, fn, dst);
				if (rc)
					fprintf(stderr, ERRMSG "failed uploading %s to %s\n", src, dst);
				else
					set_owner(dst, username);
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
			set_owner(fn, username);
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
			rc = systemf("curl %s -Lo %s %s", user, fn, src);
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

			rc = systemf("curl %s -Lo %s %s", user, fn, src);
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
				rc = systemf("curl %s -LT %s %s", user, fn, dst);
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
	       "  -v         Show version\n", prognm);

	return rc;
}

int main(int argc, char *argv[])
{
	const char *user = NULL, *src = NULL, *dst = NULL;
	int c;

	while ((c = getopt(argc, argv, "hu:v")) != EOF) {
		switch(c) {
		case 'h':
			return usage(0);
		case 'u':
			user = optarg;
			break;
		case 'v':
			puts(PACKAGE_VERSION);
			return 0;
		}
	}

	if (optind >= argc)
		return usage(1);

	src = argv[optind++];
	dst = argv[optind++];

	return copy(src, dst, user);
}
