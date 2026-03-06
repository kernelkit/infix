#include <errno.h>
#include <grp.h>
#include <pwd.h>
#include <stdio.h>
#include <stdarg.h>
#include <stdlib.h>
#include <string.h>
#include <fcntl.h>
#include <sys/stat.h>
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

/*
 * Safe subprocess execution without shell interpretation.
 * Use this instead of system()/systemf() when arguments come from user input.
 */
static int run(char *const argv[])
{
	pid_t pid;
	int status;

	pid = fork();
	if (pid == 0) {
		execvp(argv[0], argv);
		_exit(127);
	}

	if (pid < 0)
		return -1;

	if (waitpid(pid, &status, 0) < 0)
		return -1;

	if (WIFEXITED(status))
		return WEXITSTATUS(status);

	if (WIFSIGNALED(status)) {
		errno = EINTR;
		return -1;
	}

	return -1;
}

/*
 * Like run(), but drops privileges to the given user before exec.
 * Use for commands that must run as the CLI user, not root (klishd).
 */
static int run_as_user(const char *user, char *const argv[])
{
	struct passwd *pw;
	pid_t pid;
	int rc;

	pw = getpwnam(user);
	if (!pw) {
		fprintf(stderr, ERRMSG "unknown user: %s\n", user);
		return -1;
	}

	pid = fork();
	if (pid == -1)
		return -1;

	if (!pid) {
		if (initgroups(user, pw->pw_gid) || setgid(pw->pw_gid) || setuid(pw->pw_uid)) {
			fprintf(stderr, "Aborting, failed dropping privileges to "
				"(UID:%d GID:%d): %s\n",
				pw->pw_uid, pw->pw_gid, strerror(errno));
			_exit(1);
		}
		execvp(argv[0], argv);
		_exit(127);
	}

	while (waitpid(pid, &rc, 0) < 0) {
		if (errno != EINTR)
			return -1;
	}

	if (WIFEXITED(rc))
		return WEXITSTATUS(rc);

	if (WIFSIGNALED(rc)) {
		errno = EINTR;
		return -1;
	}

	return -1;
}

/*
 * Shell command execution - only use with hardcoded commands or when
 * shell features (pipes, redirects) are needed.  Never use with user input.
 */
static int shellf(const char *fmt, ...)
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
	char *argv[] = { "files", "/cfg", NULL };
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
	return run(argv);
}

int infix_erase(kcontext_t *ctx)
{
	kpargv_t *pargv = kcontext_pargv(ctx);
	const char *path;
	char *argv[4];

	path = kparg_value(kpargv_find(pargv, "file"));
	if (!path) {
		fprintf(stderr, ERRMSG "missing file argument to remove.\n");
		return -1;
	}

	cd_home(ctx);

	argv[0] = "erase";
	argv[1] = "-s";
	argv[2] = (char *)path;
	argv[3] = NULL;

	return run(argv);
}

int infix_files(kcontext_t *ctx)
{
	const char *path;
	char *argv[3];

	cd_home(ctx);
	path = kcontext_script(ctx);
	if (!path) {
		fprintf(stderr, ERRMSG "missing path argument to file search.\n");
		return -1;
	}

	argv[0] = "files";
	argv[1] = (char *)path;
	argv[2] = NULL;

	return run(argv);
}

int infix_ifaces(kcontext_t *ctx)
{
	(void)ctx;
	system("ip -j link | jq -r '.[] | select(.group != \"internal\") | .ifname'");
	return 0;
}

/* Note: uses shellf() for pipes, but all arguments are hardcoded by callers */
static int firewall_dbus_completion(const char *interface, const char *method, const char *parser)
{
	return shellf("gdbus call --system --dest org.fedoraproject.FirewallD1 "
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
/* Note: uses shellf() for pipes, but command is hardcoded */
int infix_firewall_services(kcontext_t *ctx)
{
	(void)ctx;
	return shellf("gdbus call --system --dest org.fedoraproject.FirewallD1 "
		      "--object-path /org/fedoraproject/FirewallD1 "
		      "--method org.fedoraproject.FirewallD1.listServices 2>/dev/null "
		      "| sed 's/^(//; s/,)$//' | tr \"'\" '\"' | jq -r '.[]' 2>/dev/null");
}

int infix_copy(kcontext_t *ctx)
{
	kpargv_t *pargv = kcontext_pargv(ctx);
	const char *src, *dst, *remote_user;
	char *argv[12];
	kparg_t *parg;
	int i = 0;

	src = kparg_value(kpargv_find(pargv, "src"));
	dst = kparg_value(kpargv_find(pargv, "dst"));
	if (!src || !dst)
		return -1;

	/* Ensure we run the copy command as the logged-in user, not root (klishd) */
	argv[i++] = "doas";
	argv[i++] = "-u";
	argv[i++] = (char *)cd_home(ctx);
	argv[i++] = "copy";
	argv[i++] = "-s";

	parg = kpargv_find(pargv, "validate");
	if (parg)
		argv[i++] = "-n";

	parg = kpargv_find(pargv, "user");
	if (parg) {
		remote_user = kparg_value(parg);
		argv[i++] = "-u";
		argv[i++] = (char *)remote_user;
	}

	argv[i++] = (char *)src;
	argv[i++] = (char *)dst;
	argv[i] = NULL;

	return run(argv);
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
		if (initgroups(user, pw->pw_gid) || setgid(pw->pw_gid) || setuid(pw->pw_uid)) {
			fprintf(stderr, "Aborting, failed dropping privileges to (UID:%d GID:%d): %s\n",
				pw->pw_uid, pw->pw_gid, strerror(errno));
			_exit(1);
		}

		_exit(execvp(args[0], args));
	}

	while (waitpid(pid, &rc, 0) < 0) {
		if (errno != EINTR)
			return -1;
	}

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

static int infix_set_boot_order(kcontext_t *ctx)
{
	kpargv_t *pargv = kcontext_pargv(ctx);
	const char *targets[3];
	char *argv[16];
	int i = 0;

	targets[0] = valid_boot_target(kpargv_find(pargv, "first"));
	targets[1] = valid_boot_target(kpargv_find(pargv, "second"));
	targets[2] = valid_boot_target(kpargv_find(pargv, "third"));

	if (!targets[0]) {
		fprintf(stderr, ERRMSG "missing boot target\n");
		return -1;
	}

	argv[i++] = "doas";
	argv[i++] = "-u";
	argv[i++] = (char *)cd_home(ctx);
	argv[i++] = "rpc";
	argv[i++] = "/infix-system:set-boot-order";

	if (targets[0]) {
		argv[i++] = "boot-order";
		argv[i++] = (char *)targets[0];
	}
	if (targets[1]) {
		argv[i++] = "boot-order";
		argv[i++] = (char *)targets[1];
	}
	if (targets[2]) {
		argv[i++] = "boot-order";
		argv[i++] = (char *)targets[2];
	}
	argv[i] = NULL;

	return run(argv);
}

int infix_users(kcontext_t *ctx)
{
	(void)ctx;
	return shellf("copy oper -x /system/authentication/user/name "
		      "| jq -r '.\"ietf-system:system\".authentication.user[].name'");
}

int infix_groups(kcontext_t *ctx)
{
	(void)ctx;
	return shellf("copy oper -x /nacm/groups "
		      "| jq -r '.\"ietf-netconf-acm:nacm\".groups.group[].name'");
}

int infix_sym_keys(kcontext_t *ctx)
{
	(void)ctx;
	return shellf("copy running -x /ietf-keystore:keystore/symmetric-keys "
		      "| jq -r '.\"ietf-keystore:keystore\".\"symmetric-keys\".\"symmetric-key\"[].name'");
}

int infix_asym_keys(kcontext_t *ctx)
{
	(void)ctx;
	return shellf("copy running -x /ietf-keystore:keystore/asymmetric-keys "
		      "| jq -r '.\"ietf-keystore:keystore\".\"asymmetric-keys\".\"asymmetric-key\"[].name'");
}

/*
 * Create ~/.ssh/known_hosts with correct ownership if it doesn't exist.
 * Must be called as root before dropping privileges, since ~/.ssh/ is
 * owned by root on Infix (the system manages authorized_keys via YANG).
 * Once the file exists with the user's ownership, ssh(1) can update it.
 */
static void ensure_known_hosts(const struct passwd *pw)
{
	char path[512];
	int fd;

	snprintf(path, sizeof(path), "%s/.ssh/known_hosts", pw->pw_dir);
	fd = open(path, O_CREAT | O_EXCL | O_WRONLY, 0600);
	if (fd < 0)
		return; /* Already exists, or unrecoverable error */

	fchown(fd, pw->pw_uid, pw->pw_gid);
	close(fd);
}

int infix_ssh_connect(kcontext_t *ctx)
{
	kpargv_t *pargv = kcontext_pargv(ctx);
	const char *host, *ruser, *port, *user;
	struct passwd *pw;
	kparg_t *parg;
	char *argv[8];
	int i = 0;

	host  = kparg_value(kpargv_find(pargv, "host"));

	parg  = kpargv_find(pargv, "user");
	ruser = parg ? kparg_value(parg) : NULL;

	parg  = kpargv_find(pargv, "port");
	port  = parg ? kparg_value(parg) : NULL;

	if (!host) {
		fprintf(stderr, ERRMSG "missing host argument.\n");
		return -1;
	}

	user = cd_home(ctx);
	pw   = getpwnam(user);
	if (pw)
		ensure_known_hosts(pw);

	argv[i++] = "ssh";
	if (ruser) { argv[i++] = "-l"; argv[i++] = (char *)ruser; }
	if (port)  { argv[i++] = "-p"; argv[i++] = (char *)port;  }
	argv[i++] = (char *)host;
	argv[i]   = NULL;

	return run_as_user(user, argv);
}

/*
 * Completion: list hostnames from the current user's ~/.ssh/known_hosts.
 */
int infix_ssh_known_hosts(kcontext_t *ctx)
{
	char path[512], line[4096];
	const char *user;
	struct passwd *pw;
	FILE *f;

	user = cd_home(ctx);
	pw   = getpwnam(user);
	if (!pw)
		return 0;

	snprintf(path, sizeof(path), "%s/.ssh/known_hosts", pw->pw_dir);
	f = fopen(path, "r");
	if (!f)
		return 0;

	while (fgets(line, sizeof(line), f)) {
		char *sp;

		/* Skip comments, blank lines, and hashed entries */
		if (line[0] == '#' || line[0] == '\n' || line[0] == '|')
			continue;
		sp = strchr(line, ' ');
		if (!sp)
			continue;
		*sp = '\0';
		/* Entries may be comma-separated: "host,ip algo key" */
		for (char *tok = strtok(line, ","); tok; tok = strtok(NULL, ","))
			puts(tok);
	}

	fclose(f);
	return 0;
}

/*
 * Pre-enroll a host public key received out-of-band into ~/.ssh/known_hosts.
 * Runs as the CLI user to ensure correct file ownership.
 */
int infix_ssh_add_known_host(kcontext_t *ctx)
{
	kpargv_t *pargv = kcontext_pargv(ctx);
	const char *host, *keytype, *pubkey, *user;
	char path[512];
	struct passwd *pw;
	pid_t pid;
	int rc;

	host    = kparg_value(kpargv_find(pargv, "host"));
	keytype = kparg_value(kpargv_find(pargv, "keytype"));
	pubkey  = kparg_value(kpargv_find(pargv, "pubkey"));
	if (!host || !keytype || !pubkey) {
		fprintf(stderr, ERRMSG "missing arguments.\n");
		return -1;
	}

	user = cd_home(ctx);
	pw   = getpwnam(user);
	if (!pw) {
		fprintf(stderr, ERRMSG "unknown user: %s\n", user);
		return -1;
	}

	snprintf(path, sizeof(path), "%s/.ssh/known_hosts", pw->pw_dir);

	ensure_known_hosts(pw);

	pid = fork();
	if (pid == -1)
		return -1;

	if (!pid) {
		FILE *f;

		if (setgid(pw->pw_gid) || setuid(pw->pw_uid)) {
			fprintf(stderr, "Aborting, failed dropping privileges: %s\n",
				strerror(errno));
			_exit(1);
		}

		f = fopen(path, "a");
		if (!f) {
			fprintf(stderr, ERRMSG "cannot open %s: %s\n", path, strerror(errno));
			_exit(1);
		}

		fprintf(f, "%s %s %s\n", host, keytype, pubkey);
		fclose(f);
		printf("Host %s added to %s\n", host, path);
		_exit(0);
	}

	while (waitpid(pid, &rc, 0) != pid)
		;

	if (WIFEXITED(rc))
		return WEXITSTATUS(rc);

	if (WIFSIGNALED(rc)) {
		errno = EINTR;
		return -1;
	}

	return -1;
}

int infix_ssh_remove_known_host(kcontext_t *ctx)
{
	kpargv_t *pargv = kcontext_pargv(ctx);
	const char *host;
	char *argv[4];

	host = kparg_value(kpargv_find(pargv, "host"));
	if (!host) {
		fprintf(stderr, ERRMSG "missing host argument.\n");
		return -1;
	}

	argv[0] = "ssh-keygen";
	argv[1] = "-R";
	argv[2] = (char *)host;
	argv[3] = NULL;

	return run_as_user(cd_home(ctx), argv);
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
	kplugin_add_syms(plugin, ksym_new("users", infix_users));
	kplugin_add_syms(plugin, ksym_new("groups", infix_groups));
	kplugin_add_syms(plugin, ksym_new("sym_keys", infix_sym_keys));
	kplugin_add_syms(plugin, ksym_new("asym_keys", infix_asym_keys));
	kplugin_add_syms(plugin, ksym_new("firewall_zones", infix_firewall_zones));
	kplugin_add_syms(plugin, ksym_new("firewall_policies", infix_firewall_policies));
	kplugin_add_syms(plugin, ksym_new("firewall_services", infix_firewall_services));
	kplugin_add_syms(plugin, ksym_new("set_boot_order", infix_set_boot_order));
	kplugin_add_syms(plugin, ksym_new("shell", infix_shell));
	kplugin_add_syms(plugin, ksym_new("ssh_connect",           infix_ssh_connect));
	kplugin_add_syms(plugin, ksym_new("ssh_known_hosts",       infix_ssh_known_hosts));
	kplugin_add_syms(plugin, ksym_new("ssh_add_known_host",    infix_ssh_add_known_host));
	kplugin_add_syms(plugin, ksym_new("ssh_remove_known_host", infix_ssh_remove_known_host));

	return 0;
}
