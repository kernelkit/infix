#include <errno.h>
#include <pwd.h>
#include <stdio.h>
#include <stdarg.h>
#include <stdlib.h>
#include <string.h>
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
	return systemf("files /cfg .cfg");
}

int infix_erase(kcontext_t *ctx)
{
	kpargv_t *pargv = kcontext_pargv(ctx);
	const char *path;

	path = kparg_value(kpargv_find(pargv, "file"));
	if (!path) {
		fprintf(stderr, ERRMSG "missing file argument to remove.\n");
		return -1;
	}

	cd_home(ctx);

	return systemf("erase %s", path);
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

	return systemf("files %s", path);
}

int infix_ifaces(kcontext_t *ctx)
{
	(void)ctx;
	system("ip -j link | jq -r '.[] | select(.group != \"internal\") | .ifname'");
	return 0;
}

int infix_copy(kcontext_t *ctx)
{
	kpargv_t *pargv = kcontext_pargv(ctx);
	const char *src, *dst;
	const char *username;
	char user[256] = "";
	kparg_t *parg;

	src = kparg_value(kpargv_find(pargv, "src"));
	dst = kparg_value(kpargv_find(pargv, "dst"));
	if (!src || !dst)
		return -1;

	parg = kpargv_find(pargv, "user");
	if (parg)
		snprintf(user, sizeof(user), "-u %s", kparg_value(parg));

	cd_home(ctx);
	username = ksession_user(kcontext_session(ctx));

	return systemf("sudo -u %s copy %s %s %s", username, user, src, dst);
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
