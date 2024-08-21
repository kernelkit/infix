#include <assert.h>
#include <dirent.h>
#include <errno.h>
#include <pwd.h>
#include <stdio.h>
#include <stdarg.h>
#include <stdlib.h>
#include <string.h>
#include <termios.h>
#include <sys/types.h>
#include <sys/wait.h>

#include <sysrepo.h>
#include <sysrepo_types.h>
#include <sysrepo/values.h>
#include <sysrepo/netconf_acm.h>

#include <klish/kplugin.h>
#include <klish/ksession.h>
#include <klish/kcontext.h>

#include <libyang/libyang.h>

#define ERRMSG "Error: "
#define INFMSG "Note: "

#ifndef NELEMS
#define NELEMS(v) (sizeof(v) / sizeof(v[0]))
#endif

const uint8_t kplugin_infix_major = 1;
const uint8_t kplugin_infix_minor = 0;

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

static void cd_home(kcontext_t *ctx)
{
	const char *user = "root";
	ksession_t *session;
	struct passwd *pw;

	session = kcontext_session(ctx);
	if (session) {
		user = ksession_user(session);
		if (!user)
			user = "root";
	}

	pw = getpwnam(user);
	chdir(pw->pw_dir);
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

static int has_ext(const char *fn, const char *ext)
{
	size_t pos = strlen(fn);
	size_t len = strlen(ext);

	if (len < pos && !strcmp(&fn[pos - len], ext))
		return pos - len;
	return 0;
}

static const char *basenm(const char *fn)
{
	const char *ptr;

	if (!fn)
		return "";

	ptr = strrchr(fn, '/');
	if (!ptr)
		ptr = fn;

	return ptr;
}

static char *cfg_adjust(const char *fn, const char *tmpl, char *buf, size_t len)
{
	if (strstr(fn, "../"))
		return NULL;	/* relative paths not allowed */

	if (fn[0] == '/') {
		strncpy(buf, fn, len);
		return buf;	/* allow absolute paths */
	}

	/* Files in /cfg must end in .cfg */
	if (!strncmp(fn, "/cfg/", 5)) {
		snprintf(buf, len, "%s", fn);
		if (!has_ext(fn, ".cfg"))
			strcat(buf, ".cfg");

		return buf;
	}

	/* Files ending with .cfg belong in /cfg */
	if (has_ext(fn, ".cfg")) {
		snprintf(buf, len, "/cfg/%s", fn);
		return buf;
	}

	if (strlen(fn) > 0 && fn[0] == '.' && tmpl) {
		if (fn[1] == '/' && fn[1] != 0)
			strncpy(buf, fn, len);
		else
			snprintf(buf, len, "%s", basenm(tmpl));
	} else
		strncpy(buf, fn, len);

	return buf;
}

static char rawgetch(void)
{
	struct termios saved, c;
	char key;

	if (tcgetattr(fileno(stdin), &saved) < 0)
		return -1;

	c = saved;
	c.c_lflag &= ~ICANON;
	c.c_lflag &= ~ECHO;
	c.c_cc[VMIN] = 1;
	c.c_cc[VTIME] = 0;

	if (tcsetattr(fileno(stdin), TCSANOW, &c) < 0) {
		tcsetattr(fileno(stdin), TCSANOW, &saved);
		return -1;
	}

	key = getchar();
	tcsetattr(fileno(stdin), TCSANOW, &saved);

	return key;
}

static int yorn(const char *fmt, ...)
{
	va_list ap;
	char ch;

	va_start(ap, fmt);
	vfprintf(stderr, fmt, ap);
	va_end(ap);

	fprintf(stderr, " (y/N)? ");
	ch = rawgetch();
	fprintf(stderr, "%c\n", ch);
	if (ch != 'y' && ch != 'Y')
		return 0;

	return 1;
}

static int systemf(const char *fmt, ...)
{
	va_list ap;
	char *cmd;
	int len;
	int rc;

	va_start(ap, fmt);
	len = vsnprintf(NULL, 0, fmt, ap);
	va_end(ap);

	cmd = alloca(++len);
	if (!cmd) {
		errno = ENOMEM;
		return -1;
	}

	va_start(ap, fmt);
	vsnprintf(cmd, len, fmt, ap);
	va_end(ap);

//	fprintf(stderr, INFMSG "CMD: '%s'\n", cmd);

	rc = system(cmd);
	if (rc == -1)
		return -1;

	if (WIFEXITED(rc)) {
		errno = 0;
		rc = WEXITSTATUS(rc);
	} else if (WIFSIGNALED(rc)) {
		errno = EINTR;
		rc = -1;
	}

	return rc;
}

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

static int files(const char *path, const char *stripext)
{
	struct dirent *d;
	DIR *dir;

	dir = opendir(path);
	if (!dir) {
		fprintf(stderr, ERRMSG "%s", strerror(errno));
		return -1;
	}

	while ((d = readdir(dir))) {
		char name[sizeof(d->d_name) + 1];

		/* only list regular files, skip dirs and dotfiles */
		if (d->d_type != DT_REG || d->d_name[0] == '.')
			continue;

		strncpy(name, d->d_name, sizeof(name));
		if (stripext) {
			size_t pos = has_ext(name, stripext);

			if (pos)
				name[pos] = 0;
		}

		printf("%s\n", name);
	}

	return closedir(dir);
}

int infix_datastore(kcontext_t *ctx)
{
	const char *ds;

	ds = kcontext_script(ctx);
	if (!ds)
		goto done;

	if (!strcmp(ds, "src")) {
		puts("factory-config");
		puts("running-config");
		puts("startup-config");
	}
	if (!strcmp(ds, "dst")) {
		puts("running-config");
		puts("startup-config");
	}

done:
	return files("/cfg", ".cfg");
}

int infix_erase(kcontext_t *ctx)
{
	kpargv_t *pargv = kcontext_pargv(ctx);
	const char *path;
	char *fn;

	path = kparg_value(kpargv_find(pargv, "file"));
	if (!path) {
		fprintf(stderr, ERRMSG "missing file argument to remove.\n");
		return -1;
	}

	cd_home(ctx);
	if (access(path, F_OK)) {
		size_t len = strlen(path) + 10;

		fn = alloca(len);
		if (!fn) {
			fprintf(stderr, ERRMSG "failed allocating memory.\n");
			return -1;
		}

		cfg_adjust(path, NULL, fn, len);
		if (access(fn, F_OK)) {
			fprintf(stderr, "No such file: %s\n", fn);
			return -1;
		}
	} else
		fn = (char *)path;

	if (!yorn("Remove %s, are you sure", fn))
		return 0;

	if (remove(fn)) {
		fprintf(stderr, ERRMSG "failed removing %s: %s\n", fn, strerror(errno));
		return -1;
	}

	return 0;
}

int infix_files(kcontext_t *ctx)
{
	const char *path;

	cd_home(ctx);
	path = kcontext_script(ctx);
	if (!path) {
		fprintf(stderr, ERRMSG "missing path argument to file search.\n");
		return -1;
	}

	return files(path, NULL);
}

int infix_ifaces(kcontext_t *ctx)
{
	(void)ctx;
	system("ls /sys/class/net");
	return 0;
}

static const char *infix_ds(const char *text, const char *type, struct infix_ds **ds)
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

int infix_copy(kcontext_t *ctx)
{
	struct infix_ds *srcds = NULL, *dstds = NULL;
	char temp_file[20] = "/tmp/copy.XXXXXX";
	kpargv_t *pargv = kcontext_pargv(ctx);
	const char *tmpfn = NULL;
	sr_session_ctx_t *sess;
	const char *fn = NULL;
	const char *src, *dst;
	const char *username;
	sr_conn_ctx_t *conn;
	char user[256] = "";
	char adjust[256];
	kparg_t *parg;
	int rc = 0;

	src = kparg_value(kpargv_find(pargv, "src"));
	dst = kparg_value(kpargv_find(pargv, "dst"));
	if (!src || !dst)
		goto err;

	parg = kpargv_find(pargv, "user");
	if (parg)
		snprintf(user, sizeof(user), "-u %s", kparg_value(parg));

	src = infix_ds(src, "source", &srcds);
	if (!src)
		goto err;
	dst = infix_ds(dst, "destination", &dstds);
	if (!dst)
		goto err;

	if (!strcmp(src, dst)) {
		fprintf(stderr, ERRMSG "source and destination are the same, aborting.");
		goto err;
	}

	cd_home(ctx);
	username = ksession_user(kcontext_session(ctx));
	umask(0660);

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

	return rc;
}

int infix_shell(kcontext_t *ctx)
{
	const char *user = "root";
	ksession_t *session;
	pid_t pid;
	int rc;

	session = kcontext_session(ctx);
	if (session) {
		user = ksession_user(session);
		if (!user)
			user = "root";
	}

	pid = fork();
	if (pid == -1)
		return -1;

	if (!pid) {
		struct passwd *pw;
		char *args[] = {
			"env", "CLISH=yes",
			SHELL, "-il",
			NULL
		};

		pw = getpwnam(user);
		if (setgid(pw->pw_gid) || setuid(pw->pw_uid)) {
			fprintf(stderr, "Aborting, failed dropping privileges to (UID:%d GID:%d): %s\n",
				pw->pw_uid, pw->pw_gid, strerror(errno));
			_exit(1);
		}

		_exit(execvp(args[0], args));
	}

	while (waitpid(pid, &rc, 0) != pid)
		;

	if (WIFEXITED(rc))
		rc = WEXITSTATUS(rc);
	else if (WIFSIGNALED(rc))
		rc = -2;

	return rc;
}

int kplugin_infix_fini(kcontext_t *ctx)
{
	(void)ctx;

	return 0;
}

int kplugin_infix_init(kcontext_t *ctx)
{
	kplugin_t *plugin = kcontext_plugin(ctx);

	kplugin_add_syms(plugin, ksym_new("copy", infix_copy));
	kplugin_add_syms(plugin, ksym_new("datastore", infix_datastore));
	kplugin_add_syms(plugin, ksym_new("erase", infix_erase));
	kplugin_add_syms(plugin, ksym_new("files", infix_files));
	kplugin_add_syms(plugin, ksym_new("ifaces", infix_ifaces));
	kplugin_add_syms(plugin, ksym_new("shell", infix_shell));

	return 0;
}
