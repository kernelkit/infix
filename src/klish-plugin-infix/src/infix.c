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

static const char *cd_home(kcontext_t *ctx)
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

	return user;
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
	return systemf("files /cfg");
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

	return systemf("erase -s %s", path);
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

static int firewall_dbus_completion(const char *interface, const char *method, const char *parser)
{
	return systemf("gdbus call --system --dest org.fedoraproject.FirewallD1 "
		       "--object-path /org/fedoraproject/FirewallD1 "
		       "--method org.fedoraproject.FirewallD1.%s.%s 2>/dev/null "
		       "| %s", interface, method, parser);
}

/*
 * Completion function for firewall zones.
 * D-Bus returns variant format: ({'zone1': {...}},)
 * Pipeline:
 *   - sed removes wrapper parentheses
 *   - tr converts single to double quotes
 *   - jq extracts keys
 */
int infix_firewall_zones(kcontext_t *ctx)
{
	(void)ctx;
	return firewall_dbus_completion("zone", "getActiveZones",
		"sed 's/^(//; s/,)$//' | sed 's/@as \\[\\]/[]/g' | tr \"'\" '\"' | jq -r 'keys[]' 2>/dev/null");
}

/*
 * Completion function for firewall policies.
 * D-Bus returns variant format: (['policy1', 'policy2'],)
 * Pipeline:
 *   - sed removes wrapper parentheses
 *   - tr converts single to double quotes
 *   - jq extracts array items
 */
int infix_firewall_policies(kcontext_t *ctx)
{
	(void)ctx;
	return firewall_dbus_completion("policy", "getPolicies",
		"sed 's/^(//; s/,)$//' | tr \"'\" '\"' | jq -r '.[]' 2>/dev/null");
}

/*
 * Completion function for firewall services.
 * D-Bus returns variant format: (['dhcp', 'dns', 'ssh'],)
 * Pipeline:
 *   - sed removes wrapper parentheses
 *   - tr converts single to double quotes
 *   - jq extracts array items
 */
int infix_firewall_services(kcontext_t *ctx)
{
	(void)ctx;
	return systemf("gdbus call --system --dest org.fedoraproject.FirewallD1 "
		       "--object-path /org/fedoraproject/FirewallD1 "
		       "--method org.fedoraproject.FirewallD1.listServices 2>/dev/null "
		       "| sed 's/^(//; s/,)$//' | tr \"'\" '\"' | jq -r '.[]' 2>/dev/null");
}

int infix_copy(kcontext_t *ctx)
{
	kpargv_t *pargv = kcontext_pargv(ctx);
	const char *src, *dst;
	char user[256] = "";
	char validate[8] = "";
	kparg_t *parg;

	src = kparg_value(kpargv_find(pargv, "src"));
	dst = kparg_value(kpargv_find(pargv, "dst"));
	if (!src || !dst)
		return -1;

	parg = kpargv_find(pargv, "user");
	if (parg)
		snprintf(user, sizeof(user), "-u %s", kparg_value(parg));

	parg = kpargv_find(pargv, "validate");
	if (parg)
		strlcpy(validate, "-n", sizeof(validate));

	/* Ensure we run the copy command as the logged-in user, not root (klishd) */
	return systemf("doas -u %s copy -s %s %s %s %s", cd_home(ctx), validate, user, src, dst);
}

int infix_shell(kcontext_t *ctx)
{
	const char *user = cd_home(ctx);
	pid_t pid;
	int rc;

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

static const char *boot_targets[] = {
	"primary",
	"secondary",
	"net",
	NULL
};

int infix_boot_targets(kcontext_t *ctx)
{
	size_t i;

	(void)ctx;

	for (i = 0; boot_targets[i]; i++)
		puts(boot_targets[i]);

	return 0;
}

static const char *valid_boot_target(const kparg_t *parg)
{
	const char *target;
	size_t i;

	if (!parg)
		return NULL;

	target = kparg_value(parg);
	for (i = 0; boot_targets[i]; i++) {
		if (!strcmp(target, boot_targets[i]))
			return target;
	}

	return NULL;
}

int infix_set_boot_order(kcontext_t *ctx)
{
	char tmpfile[] = "/tmp/boot-order-XXXXXX";
	kpargv_t *pargv = kcontext_pargv(ctx);
	const char *targets[3];
	int fd, rc = 0;
	FILE *fp;

	targets[0] = valid_boot_target(kpargv_find(pargv, "first"));
	targets[1] = valid_boot_target(kpargv_find(pargv, "second"));
	targets[2] = valid_boot_target(kpargv_find(pargv, "third"));

	if (!targets[0]) {
		fprintf(stderr, ERRMSG "missing boot target\n");
		return -1;
	}

	fd = mkstemp(tmpfile);
	if (fd == -1)
		goto fail;

	fp = fdopen(fd, "w");
	if (!fp) {
		close(fd);
		unlink(tmpfile);
	fail:
		fprintf(stderr, ERRMSG "failed creating temporary file\n");
		return -1;
	}

	fputs("{\"infix-system:set-boot-order\":{\"boot-order\":[", fp);
	for (size_t i = 0; i < NELEMS(targets); i++) {
		if (!targets[i])
			continue;

		fprintf(fp, "%s\"%s\"", i > 0 ? "," : "", targets[i]);
	}
	fputs("]}}", fp);

	fclose(fp);

	rc = systemf("sysrepocfg -R %s -fjson 2>&1", tmpfile);
	unlink(tmpfile);

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

	kplugin_add_syms(plugin, ksym_new("boot_targets", infix_boot_targets));
	kplugin_add_syms(plugin, ksym_new("copy", infix_copy));
	kplugin_add_syms(plugin, ksym_new("datastore", infix_datastore));
	kplugin_add_syms(plugin, ksym_new("erase", infix_erase));
	kplugin_add_syms(plugin, ksym_new("files", infix_files));
	kplugin_add_syms(plugin, ksym_new("ifaces", infix_ifaces));
	kplugin_add_syms(plugin, ksym_new("firewall_zones", infix_firewall_zones));
	kplugin_add_syms(plugin, ksym_new("firewall_policies", infix_firewall_policies));
	kplugin_add_syms(plugin, ksym_new("firewall_services", infix_firewall_services));
	kplugin_add_syms(plugin, ksym_new("set_boot_order", infix_set_boot_order));
	kplugin_add_syms(plugin, ksym_new("shell", infix_shell));

	return 0;
}
