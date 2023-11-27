#include <assert.h>
#include <dirent.h>
#include <errno.h>
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

#include <klish/kplugin.h>
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
	{ "factory-config",     NULL,          SR_DS_FACTORY_DEFAULT, 0, "/cfg/factory-config.cfg" },
};

static int has_ext(const char *fn, const char *ext)
{
	size_t pos = strlen(fn);
	size_t len = strlen(ext);

	if (len < pos && !strcmp(&fn[pos - len], ext))
		return pos - len;
	return 0;
}

static char *cfg_adjust(const char *fn, char *buf, size_t len)
{
	if (strncmp(fn, "/cfg/", 5)) {
		if (fn[0] == '.' || fn[0] == '/')
			return NULL;

		snprintf(buf, len, "/cfg/%s", fn);
	}

	if (!has_ext(buf, ".cfg"))
		strcat(buf, ".cfg");

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

		if (d->d_type != DT_REG)
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
	}
	if (!strcmp(ds, "dst")) {
		puts("running-config");
	}

done:
	return files("/cfg", ".cfg");
}

int infix_dir(kcontext_t *ctx)
{
	return systemf("ls --color=always --group-directories-first -A /cfg");
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

	if (access(path, F_OK)) {
		size_t len = strlen(path) + 10;

		fn = alloca(len);
		if (!fn) {
			fprintf(stderr, ERRMSG "failed allocating memory.\n");
			return -1;
		}

		cfg_adjust(path, fn, len);
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

	if (strchr(text, '/') && !strstr(text, "://")) {
		fprintf(stderr, ERRMSG "invalid %s argument: %s\n", type, text);
		return NULL;
	}

	return text;
}

int infix_copy(kcontext_t *ctx)
{
	struct infix_ds *srcds = NULL, *dstds = NULL;
	kpargv_t *pargv = kcontext_pargv(ctx);
	sr_session_ctx_t *sess;
	const char *src, *dst;
	sr_conn_ctx_t *conn;
	int rc = 0;

	src = kparg_value(kpargv_find(pargv, "src"));
	dst = kparg_value(kpargv_find(pargv, "dst"));
	if (!src || !dst)
		goto err;

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

	/* 1. Regular ds copy */
	if (srcds && dstds) {
		/* Ensure the dst ds is writable */
		if (!dstds->rw) {
		invalid:
			fprintf(stderr, ERRMSG "\"%s\" is not a valid file or datastore\n", dst);
			goto err;
		}

		if (sr_connect(SR_CONN_DEFAULT, &conn)) {
			fprintf(stderr, ERRMSG "connection to datastore failed\n");
			goto err;
		}
		if (sr_session_start(conn, dstds->datastore, &sess)) {
			fprintf(stderr, ERRMSG "unable to open transaction to %s\n", dst);
		} else {
			rc = sr_copy_config(sess, NULL, srcds->datastore, 0);
			if (rc)
				emsg(sess, ERRMSG "unable to copy configuration (%d)\n", rc);
		}
		rc = sr_disconnect(conn);
	} else {
		char temp_file[20] = "/tmp/copy.XXXXXX";
		const char *tmpfn = NULL;
		const char *fn = NULL;
		char adjust[256];

		if (srcds) {
			/* 2. Export from a datastore somewhere else */
			if (strstr(dst, "://")) {
				snprintf(adjust, sizeof(adjust), "/tmp/%s.cfg", srcds->name);
				tmpfn = adjust;
				fn = tmpfn;
			} else {
				fn = cfg_adjust(dst, adjust, sizeof(adjust));
				if (!fn) {
					fprintf(stderr, "2.3\n");
					goto invalid;
				}

				if (!access(fn, F_OK) && !yorn("Overwrite existing file %s", fn)) {
					fprintf(stderr, "OK, aborting.\n");
					return 0;
				}
			}

			if (srcds->path)
				fn = srcds->path; /* user directly => correct name */
			else
				rc = systemf("sysrepocfg -d %s -X%s -f json", srcds->sysrepocfg, fn);

			if (rc)
				fprintf(stderr, ERRMSG "failed exporting %s\n", src);
			else if (tmpfn) {
				rc = systemf("curl -T %s %s", fn, dst);
				if (rc)
					fprintf(stderr, ERRMSG "failed uploading %s to %s", src, dst);
			}
		} else if (dstds) {
			if (!dstds->sysrepocfg)
				goto invalid;
			if (!dstds->rw) {
				fprintf(stderr, ERRMSG "not possible to write to %s", dst);
				goto err;
			}

			/* 3. Import from somewhere to a datastore */
			if (strstr(src, "://")) {
				tmpfn = mktemp(temp_file);
				fn = tmpfn;
			} else {
				fn = cfg_adjust(src, adjust, sizeof(adjust));
				if (!fn)
					goto invalid;
			}

			if (tmpfn)
				rc = systemf("curl -o %s %s", fn, src);
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
				fn = cfg_adjust(dst, adjust, sizeof(adjust));
				if (!fn)
					goto invalid;

				if (!access(fn, F_OK)) {
					if (!yorn("Overwrite existing file %s", fn)) {
						fprintf(stderr, "OK, aborting.\n");
						return 0;
					}
				}

				rc = systemf("curl -o %s %s", fn, src);
			} else if (strstr(dst, "://")) {
				fn = cfg_adjust(src, adjust, sizeof(adjust));
				if (!fn)
					goto invalid;

				if (access(fn, F_OK))
					fprintf(stderr, ERRMSG "no such file %s, aborting.", fn);
				else
					rc = systemf("curl -T %s %s", fn, dst);
			}
		}

		if (tmpfn)
			rc = remove(tmpfn);
	}

err:
	return rc;
}

int infix_commit(kcontext_t *ctx)
{
	sr_session_ctx_t *sess;
	sr_conn_ctx_t *conn;
	int err;

	(void)ctx;

	if (sr_connect(SR_CONN_DEFAULT, &conn)) {
		fprintf(stderr, ERRMSG "connection to datastore failed\n");
		goto err;
	}

	if (sr_session_start(conn, SR_DS_RUNNING, &sess)) {
		fprintf(stderr, ERRMSG "unable to open transaction to running-config\n");
		goto err_disconnect;
	}

	err = sr_copy_config(sess, NULL, SR_DS_CANDIDATE, 0);
	if (err) {
		if (err == SR_ERR_CALLBACK_FAILED)
			emsg(sess, "Please check your changes, try 'diff' and 'do show interfaces'.\n");
		else
			emsg(sess, "Failed committing candidate to running: %s\n", sr_strerror(err));
		goto err_disconnect;
	}

	sr_disconnect(conn);
	return 0;

err_disconnect:
	sr_disconnect(conn);
err:
	return -1;
}

int infix_rpc(kcontext_t *ctx)
{
	kpargv_pargs_node_t *iter;
	size_t icnt = 0, ocnt = 0;
	sr_val_t *input = NULL;
	sr_session_ctx_t *sess;
	sr_conn_ctx_t *conn;
	const char *xpath;
	sr_val_t *output;
	kparg_t *parg;
	int err;

	xpath = kcontext_script(ctx);
	if (!xpath) {
		fprintf(stderr, ERRMSG "cannot find rpc xpath\n");
		goto err;
	}

	iter = kpargv_pargs_iter(kcontext_pargv(ctx));
	while ((parg = kpargv_pargs_each(&iter))) {
		const char *key = kentry_name(kparg_entry(parg));
		const char *val = kparg_value(parg);

		/* skip leading part of command line: 'set datetime' */
//		fprintf(stderr, "%s(): got key %s val %s\n", __func__, key, val ?: "<NIL>");
		if (!val || !strcmp(key, val))
			continue;

		sr_realloc_values(icnt, icnt + 1, &input);
		/* e.g. /ietf-system:set-current-datetime/current-datetime */
		sr_val_build_xpath(&input[icnt], "%s/%s", xpath, key);
		sr_val_set_str_data(&input[icnt++], SR_STRING_T, val);
	}

	if (sr_connect(SR_CONN_DEFAULT, &conn)) {
		fprintf(stderr, ERRMSG "connection to datastore failed\n");
		goto err;
	}

	if (sr_session_start(conn, SR_DS_OPERATIONAL, &sess)) {
		emsg(sess, ERRMSG "unable to open transaction to running-config\n");
		goto err_disconnect;
	}

//	fprintf(stderr, "%s(): sending RPC %s, icnt %zu\n", __func__, xpath, icnt);
	if ((err = sr_rpc_send(sess, xpath, input, icnt, 0, &output, &ocnt))) {
		emsg(sess, ERRMSG "failed sending RPC %s: %s\n", xpath, sr_strerror(err));
		goto err_disconnect;
	}

	for (size_t i = 0; i < ocnt; i++) {
		sr_print_val(&output[i]);
		puts("");
	}

	sr_free_values(input, icnt);
	sr_free_values(output, ocnt);
	sr_disconnect(conn);
	return 0;

err_disconnect:
	sr_disconnect(conn);
err:
	sr_free_values(input, icnt);
	return -1;
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
	kplugin_add_syms(plugin, ksym_new("commit", infix_commit));
	kplugin_add_syms(plugin, ksym_new("datastore", infix_datastore));
	kplugin_add_syms(plugin, ksym_new("dir", infix_dir));
	kplugin_add_syms(plugin, ksym_new("erase", infix_erase));
	kplugin_add_syms(plugin, ksym_new("files", infix_files));
	kplugin_add_syms(plugin, ksym_new("ifaces", infix_ifaces));
	kplugin_add_syms(plugin, ksym_new("rpc", infix_rpc));

	return 0;
}
