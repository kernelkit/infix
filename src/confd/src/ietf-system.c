/* SPDX-License-Identifier: BSD-3-Clause */

#include <assert.h>
#include <crypt.h>
#include <ctype.h>
#include <paths.h>
#include <pwd.h>
#include <grp.h>
#include <shadow.h>
#include <jansson.h>

#include <sys/utsname.h>
#include <sys/sysinfo.h>
#include <sys/types.h>

#include <srx/common.h>
#include <srx/lyx.h>

#include <srx/srx_val.h>

#include "base64.h"
#include "core.h"

#define NACM_BASE_     "/ietf-netconf-acm:nacm"
#define XPATH_BASE_    "/ietf-system:system"
#define XPATH_AUTH_    XPATH_BASE_"/authentication"
#define CLOCK_PATH_    "/ietf-system:system-state/clock"
#define PLATFORM_PATH_ "/ietf-system:system-state/platform"
#define PASSWORD_PATH  "/ietf-system:system/authentication/user/password"

#define _PATH_PASSWD   "/etc/passwd"
#define _PATH_HOSTNAME "/etc/hostname"
#define _PATH_HOSTS    "/etc/hosts"

struct sr_change {
	sr_change_oper_t op;
	sr_val_t *old;
	sr_val_t *new;
};

static char   *ver = NULL;
static char   *rel = NULL;
static char   *sys = NULL;
static char   *os  = NULL;
static char   *nm  = NULL;
static char   *id  = NULL;

static struct { char *name, *shell; } shells[] = {
	{ "infix-system:sh",    "/bin/sh"    },
	{ "infix-system:bash",  "/bin/bash"  },
	{ "infix-system:clish", "/bin/clish" },
	{ "infix-system:false", "/bin/false" }
};

static char *strip_quotes(char *str)
{
	char *ptr;

	while (*str && (isspace(*str) || *str == '"'))
		str++;

	for (ptr = str + strlen(str); ptr > str; ptr--) {
		if (*ptr != '"')
			continue;

		*ptr = 0;
		break;
	}

	return str;
}

static void setvar(const char *line, const char *key, char **var)
{
	char *ptr;

	if (!strncmp(line, key, strlen(key)) && (ptr = strchr(line, '='))) {
		if (*var)
			free(*var);
		*var = strdup(strip_quotes(++ptr));
	}
}

static void os_init(void)
{
	struct utsname uts;
	char line[80];
	FILE *fp;

	if (!uname(&uts)) {
		os  = strdup(uts.sysname);
		ver = strdup(uts.release);
		rel = strdup(uts.release);
		sys = strdup(uts.machine);
	}

	fp = fopen("/etc/os-release", "r");
	if (!fp) {
		fp = fopen("/usr/lib/os-release", "r");
		if (!fp)
			return;
	}

	while (fgets(line, sizeof(line), fp)) {
		line[strlen(line) - 1] = 0; /* drop \n */
		setvar(line, "NAME", &os);
		setvar(line, "VERSION_ID", &ver);
		setvar(line, "BUILD_ID", &rel);
		setvar(line, "ARCHITECTURE", &sys);
		setvar(line, "DEFAULT_HOSTNAME", &nm);
		setvar(line, "ID", &id);
	}
	fclose(fp);
}

static char *fmtime(time_t t, char *buf, size_t len)
{
        const char *isofmt = "%FT%T%z";
        struct tm tm;
        size_t i, n;

	tzset();
        localtime_r(&t, &tm);
        n = strftime(buf, len, isofmt, &tm);
        i = n - 5;
        if (buf[i] == '+' || buf[i] == '-') {
                buf[i + 6] = buf[i + 5];
                buf[i + 5] = buf[i + 4];
                buf[i + 4] = buf[i + 3];
                buf[i + 3] = ':';
        }

        return buf;
}

static sr_error_t _sr_change_iter(sr_session_ctx_t *session, struct confd *confd, char *xpath,
				  sr_error_t cb(sr_session_ctx_t *, struct confd *, struct sr_change *))
{
	struct sr_change change = {};
	sr_change_iter_t *iter;
	sr_error_t err;

	err = sr_dup_changes_iter(session, xpath, &iter);
	if (err)
		return err;

	while (sr_get_change_next(session, iter, &change.op, &change.old, &change.new) == SR_ERR_OK) {
		err = cb(session, confd, &change);
		sr_free_val(change.old);
		sr_free_val(change.new);
		if (err) {
			sr_free_change_iter(iter);
			return err;
		}
	}
	sr_free_change_iter(iter);

	return SR_ERR_OK;
}

static int clock_cb(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		    const char *path, const char *request_path, uint32_t request_id,
		    struct lyd_node **parent, void *priv)
{
	static char boottime[64] = { 0 };
	const struct ly_ctx *ctx;
	char curtime[64];
	time_t now, boot;
	int rc;

	ctx = sr_acquire_context(sr_session_get_connection(session));

	now = time(NULL);
	if (!boottime[0]) {
		struct sysinfo si;

		sysinfo(&si);
		boot = now - si.uptime;
		fmtime(boot, boottime, sizeof(boottime));
	}
	fmtime(now, curtime, sizeof(curtime));

	if ((rc = lydx_new_path(ctx, parent, CLOCK_PATH_, "boot-datetime", "%s", boottime)))
		goto fail;
	if ((rc = lydx_new_path(ctx, parent, CLOCK_PATH_, "current-datetime", "%s", curtime)))
		goto fail;

	if (rc) {
	fail:
		ERROR("Failed building data tree, libyang error %d", rc);
		rc = SR_ERR_INTERNAL;
	}

	sr_release_context(sr_session_get_connection(session));
	return rc;
}

static int platform_cb(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		       const char *path, const char *request_path, uint32_t request_id,
		       struct lyd_node **parent, void *priv)
{
	const struct ly_ctx *ctx;
	int rc;

	ctx = sr_acquire_context(sr_session_get_connection(session));

	if ((rc = lydx_new_path(ctx, parent, PLATFORM_PATH_, "os-name", "%s", os)))
		goto fail;
	if ((rc = lydx_new_path(ctx, parent, PLATFORM_PATH_, "os-release", "%s", rel)))
		goto fail;
	if ((rc = lydx_new_path(ctx, parent, PLATFORM_PATH_, "os-version", "%s", ver)))
		goto fail;
	if ((rc = lydx_new_path(ctx, parent, PLATFORM_PATH_, "machine", "%s", sys)))
		goto fail;

	if (rc) {
	fail:
		ERROR("Failed building data tree, libyang error %d", rc);
		rc = SR_ERR_INTERNAL;
	}

	sr_release_context(sr_session_get_connection(session));
	return rc;
}

static int rpc_exec(sr_session_ctx_t *session, uint32_t sub_id, const char *path,
		    const sr_val_t *input, const size_t input_cnt,
		    sr_event_t event, unsigned request_id,
		    sr_val_t **output, size_t *output_cnt,
		    void *priv)
{
	char *args[] = { (char *)priv, NULL };

	DEBUG("%s", path);

	if (runbg(args, 500) == -1)
		return SR_ERR_INTERNAL;

	return SR_ERR_OK;
}

/* '/ietf-system:set-current-date-time' */
static int rpc_set_datetime(sr_session_ctx_t *session, uint32_t sub_id,
			    const char *path, const sr_val_t *input,
			    const size_t input_cnt, sr_event_t event,
			    unsigned request_id, sr_val_t **output,
			    size_t *output_cnt, void *priv)
{
	const char *isofmt = "%FT%T%z";
	static int rc = SR_ERR_SYS;
        struct timeval tv;
        struct tm tm;
	char tz[24];
	char *buf;
	size_t n;

	buf = strdup(input->data.string_val);
	if (!buf)
		return SR_ERR_NO_MEMORY;

	n = strlen(buf);
	if (buf[n - 3] == ':' && (buf[n - 6] == '+' || buf[n - 6] == '-')) {
                buf[n - 3] = buf[n - 2];
                buf[n - 2] = buf[n - 1];
                buf[n - 1] = 0;
        } else
                isofmt = "%FT%TZ";

        memset(&tm, 0, sizeof(tm));
        if (!strptime(buf, isofmt, &tm)) {
                ERRNO("ietf-system:failed strptime: %s", strerror(errno));
		goto done;
	}

	snprintf(tz, sizeof(tz), "UTC%s%ld", tm.tm_gmtoff > 0 ? "+" : "", tm.tm_gmtoff / 3600);
	setenv("TZ", tz, 1);

        tv.tv_sec = mktime(&tm);
        tv.tv_usec = 0;
	if (tv.tv_sec == (time_t)-1 || settimeofday(&tv, NULL)) {
		ERRNO("ietf-system:settimeofday() failed");
		goto done;
	}

	/* Ensure the RTC is updated as well, in case of unclean shutdowns */
	if (systemf("hwclock -uw"))
		ERROR("failed saving new system date/time to RTC.");

	rc = SR_ERR_OK;
done:
	unsetenv("TZ");
        free(buf);

	return rc;
}

static int sys_reload_services(void)
{
	return systemf("initctl -nbq touch sysklogd");
}


#define TIMEZONE_CONF "/etc/timezone"
#define TIMEZONE_PREV TIMEZONE_CONF "-"
#define TIMEZONE_NEXT TIMEZONE_CONF "+"

static int change_clock(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	const char *tz_utc_offset;
	char tz_name[14];
	char *timezone;

	switch (event) {
	case SR_EV_ENABLED:	/* first time, on register. */
	case SR_EV_CHANGE:	/* regular change (copy cand running) */
		/* Set up next timezone */
		break;

	case SR_EV_ABORT:	/* User abort, or other plugin failed */
		(void)remove(TIMEZONE_NEXT);
		return SR_ERR_OK;

	case SR_EV_DONE:
		/* Check if passed validation in previous event */
		if (access(TIMEZONE_NEXT, F_OK))
			return SR_ERR_OK;

		(void)remove(TIMEZONE_PREV);
		(void)rename(TIMEZONE_CONF, TIMEZONE_PREV);
		(void)rename(TIMEZONE_NEXT, TIMEZONE_CONF);

		(void)remove("/etc/localtime-");
		(void)rename("/etc/localtime",  "/etc/localtime-");
		(void)rename("/etc/localtime+", "/etc/localtime");

		return SR_ERR_OK;

	default:
		return SR_ERR_OK;

	}
	tz_utc_offset = srx_get_str(session, XPATH_BASE_"/clock/timezone-utc-offset");
	timezone = srx_get_str(session, XPATH_BASE_"/clock/timezone-name");
	if (!timezone && !tz_utc_offset) {
		snprintf(tz_name,sizeof(tz_name),"Etc/UTC");
		timezone = tz_name;
	}

	if (tz_utc_offset) {
		int8_t offset = atol(tz_utc_offset);
		/* When using Etc/GMT offsets, the +/- is inverted in tzdata. */
		snprintf(tz_name,sizeof(tz_name), "Etc/GMT%s%.2d", offset>-1?"":"+", -offset);
		timezone = tz_name;
	}

	(void)remove("/etc/localtime+");
	if (systemf("ln -sf /usr/share/zoneinfo/%s /etc/localtime+", timezone)) {
		ERROR("No such timezone %s", timezone);
		return SR_ERR_VALIDATION_FAILED;
	}

	if (writesf(timezone, "w", TIMEZONE_NEXT)) {
		ERRNO("Failed preparing %s", TIMEZONE_NEXT);
		return SR_ERR_SYS;
	}

	return SR_ERR_OK;
}

static int change_ntp(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		      const char *_, sr_event_t event, unsigned request_id, void *priv)
{
	sr_change_iter_t *iter = NULL;
	int rc, err = SR_ERR_OK;
	int changes = 0;
	sr_val_t *val;
	size_t cnt;

	switch (event) {
	case SR_EV_ENABLED:	/* first time, on register. */
	case SR_EV_CHANGE:	/* regular change (copy cand running) */
		/* Generate next config */
		break;

	case SR_EV_ABORT:	/* User abort, or other plugin failed */
		return SR_ERR_OK;

	case SR_EV_DONE:
		if (!srx_enabled(session, XPATH_BASE_"/ntp/enabled")) {
			systemf("rm -rf /etc/chrony/conf.d/* /etc/chrony/sources.d/*");
			systemf("initctl -nbq disable chronyd");
			return SR_ERR_OK;
		}

		if (fexist("/run/chrony/.changes")) {
			systemf("chronyc reload sources >/dev/null");
			erase("/run/chrony/.changes");
		}

		systemf("initctl -nbq enable chronyd");
		return SR_ERR_OK;

	default:
		return SR_ERR_OK;
	}

	/*
	 * First remove .sources files for all deleted servers
	 */
	sr_get_changes_iter(session, XPATH_BASE_"/ntp/server[name=*]/name", &iter);
	if (iter) {
		sr_change_oper_t op;
		sr_val_t *old, *new;

		while (!sr_get_change_next(session, iter, &op, &old, &new)) {
			char *name;

			if (op != SR_OP_DELETED)
				continue;

			name = sr_val_to_str(old);
			DEBUG("Removing NTP server %s", name);
			erasef("/etc/chrony/sources.d/%s.sources", name);
			free(name);
			changes++;
		}

		sr_free_change_iter(iter);
	}

	/*
	 * Then add or recreate any new or modified sources
	 */
	rc = sr_get_items(session, XPATH_BASE_"/ntp/server", 0, 0, &val, &cnt);
	if (rc) {
		return SR_ERR_OK;
	}

	for (size_t i = 0; i < cnt; i++) {
		const char *xpath = val[i].xpath;
		char *ptr, *name;
		int server = 0;
		FILE *fp;

		name = srx_get_str(session, "%s/name", xpath);
		if (!name) {
			ERROR("no name for xpath %s", xpath);
			continue;
		}

		fp = fopenf("w", "/etc/chrony/sources.d/%s.sources", name);
		if (!fp) {
			ERROR("failed saving /etc/chrony/sources.d/%s.sources: %s",
			      name, strerror(errno));
			free(name);
			continue;
		}

		/* Get /ietf-system:system/ntp/server[name='foo'] */
		ptr = srx_get_str(session, "%s/udp/address", xpath);
		if (ptr) {
			char *type = srx_get_str(session, "%s/association-type", xpath);

			fprintf(fp, "%s %s", type ?: "server", ptr);
			server++;
			if (type)
				free(type);
			free(ptr);

			ptr = srx_get_str(session, "%s/udp/port", xpath);
			if (ptr) {
				fprintf(fp, " port %s", ptr);
				free(ptr);
			}
		}

		if (server) {
			if (srx_enabled(session, "%s/iburst", xpath) > 0)
				fprintf(fp, " iburst");
			if (srx_enabled(session, "%s/prefer", xpath) > 0)
				fprintf(fp, " prefer");
		}
		fprintf(fp, "\n");
		fclose(fp);
		changes++;
	}
	sr_free_values(val, cnt);

	if (changes) {
		if (touch("/run/chrony/.changes"))
			ERRNO("Failed recording changes to NTP client");
	}

	return err;
}

#define RESOLV_CONF "/etc/resolv.conf.head"
#define RESOLV_PREV RESOLV_CONF "-"
#define RESOLV_NEXT RESOLV_CONF "+"

static int change_dns(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	const char *fn = RESOLV_NEXT;
	int timeout = 0, attempts = 0;
	int rc = SR_ERR_SYS;
	sr_val_t *val;
	size_t cnt;
	FILE *fp;

	switch (event) {
	case SR_EV_ENABLED:	/* first time, on register. */
	case SR_EV_CHANGE:	/* regular change (copy cand running) */
		/* Generate next config */
		break;

	case SR_EV_ABORT:	/* User abort, or other plugin failed */
		(void)remove(RESOLV_NEXT);
		return SR_ERR_OK;

	case SR_EV_DONE:
		/* Check if passed validation in previous event */
		if (!access(RESOLV_NEXT, F_OK)) {
			(void)remove(RESOLV_PREV);
			(void)rename(RESOLV_CONF, RESOLV_PREV);
			(void)rename(RESOLV_NEXT, RESOLV_CONF);
		}

		/* in bootstrap, another resolvconf will soon take your call */
		if (systemf("initctl -bq cond get hook/sys/up"))
			return 0;

		systemf("resolvconf -u");
		return SR_ERR_OK;

	default:
		return SR_ERR_OK;
	}

	fp = fopen(fn, "w");
	if (!fp) {
		ERROR("failed updating %s: %s", fn, strerror(errno));
		return SR_ERR_SYS;
	}

	SRX_GET_UINT8(session, timeout, XPATH_BASE_"/dns-resolver/options/timeout");
	SRX_GET_UINT8(session, attempts, XPATH_BASE_"/dns-resolver/options/attempts");
	if (timeout || attempts) {
		fprintf(fp, "options");
		if (timeout)
			fprintf(fp, " timeout:%d", timeout);
		if (attempts)
			fprintf(fp, " attempts:%d ", attempts);
		fprintf(fp, "\n");
	}

	rc = sr_get_items(session, XPATH_BASE_"/dns-resolver/search", 0, 0, &val, &cnt);
	if (rc)
		goto fail;

	if (cnt) {
		fprintf(fp, "search ");
		for (size_t i = 0; i < cnt; i++)
			fprintf(fp, "%s ", val[i].data.string_val);
		fprintf(fp, "\n");
	}
	sr_free_values(val, cnt);

	rc = sr_get_items(session, XPATH_BASE_"/dns-resolver/server", 0, 0, &val, &cnt);
	if (rc)
		goto fail;

	for (size_t i = 0; i < cnt; i++) {
		char *ptr;

		/* Get /ietf-system:system/dns-resolver/server[name='foo'] */
		ptr = srx_get_str(session, "%s/udp-and-tcp/address", val[i].xpath);
		if (ptr)
			/* XXX: add support also for udp-and-tcp/port */
			fprintf(fp, "nameserver %s\n", ptr);
	}
	sr_free_values(val, cnt);

	rc = SR_ERR_OK;
fail:
	fclose(fp);
	if (rc)
		(void)remove(fn);

	return rc;
}

static bool is_group_member(const char *user, const char *group)
{
	/* Check if user is already in group */
	if (!systemf("grep %s /etc/group |grep -q %s", group, user))
		return true;

	return false;
}

static void add_group(const char *user, const char *group)
{
	bool is_already = is_group_member(user, group);

	if (is_already)
		return; /* already group member */

	if (systemf("adduser %s %s", user, group))
		AUDIT("Failed giving user \"%s\" UNIX %s permissions.", user, group);
	else
		AUDIT("User \"%s\" added to UNIX \"%s\" group.", user, group);
}

static void del_group(const char *user, const char *group)
{
	bool is_already = is_group_member(user, group);

	if (!is_already)
		return; /* not member of group */

	if (systemf("delgroup %s %s", user, group))
		AUDIT("Failed removing user \"%s\" from UNIX \"%s\" group.", user, group);
	else
		AUDIT("User \"%s\" removed from UNIX \"%s\" group.", user, group);
}

/* Users with a valid shell are also allowed CLI access */
static void adjust_access(const char *user, const char *shell)
{
	if (strcmp(shell, "/bin/false"))
		add_group(user, "sys-cli");
	else
		del_group(user, "sys-cli");
}

/* XXX: Currently Infix only has admin and non-admins as a group */
static bool is_admin_user(sr_session_ctx_t *session, const char *user)
{
	sr_val_t *groups = NULL, *rules = NULL;
	size_t group_count = 0, rule_count = 0;
	bool is_admin = false;
	char xpath[256];
	int rc;

	/* Fetch groups for each user */
	snprintf(xpath, sizeof(xpath), NACM_BASE_"/groups/group[user-name='%s']/name", user);
	rc = sr_get_items(session, xpath, 0, 0, &groups, &group_count);
	if (rc)
		return false;	/* safe default */

	for (size_t j = 0; j < group_count; j++) {
		/* Fetch and check rules for each group */
		snprintf(xpath, sizeof(xpath), NACM_BASE_"/rule-list[group='%s']/rule"
			 "[module-name='*'][access-operations='*'][action='permit']",
			 groups[j].data.string_val);
		rc = sr_get_items(session, xpath, 0, 0, &rules, &rule_count);
		if (rc)
			continue; /* not found, this is OK */

		/* At least one group grants full administrator permissions */
		if (rule_count > 0)
			is_admin = true;

		sr_free_values(rules, rule_count);
	}

	sr_free_values(groups, group_count);

	return is_admin;
}

static int is_valid_username(const char *user)
{
	size_t i;

	/* The username should not start with a digit or hyphen */
	if (!isalpha(user[0]) && (user[0] != '_'))
		return 0;

	for (i = 1; i < strlen(user); i++) {
		if (!isalnum(user[i]) && (user[i] != '_') && (user[i] != '-') && (user[i] != '.'))
			return 0;
	}

	return 1;
}

static char *sys_find_usable_shell(sr_session_ctx_t *sess, char *name)
{
	const char *conf = NULL;
	char *shell = NULL;
	char xpath[256];
	sr_data_t *cfg;

	snprintf(xpath, sizeof(xpath), XPATH_AUTH_"/user[name='%s']/infix-system:shell", name);
	if (!sr_get_data(sess, xpath, 0, 0, 0, &cfg) && cfg) {
		struct lyd_node *node;

		if (!lyd_find_path(cfg->tree, xpath, 0, &node))
			conf = (char *)lyd_get_value(node);
	}

	/* Verify the configured shell exists (and is a login shell) */
	if (conf) {
		for (size_t i = 0; i < NELEMS(shells); i++) {
			if (strcmp(shells[i].name, conf))
				continue;

			shell = shells[i].shell;
			break;
		}
	}

	if (!shell || !whichp(shell))
		shell = LOGIN_SHELL;

	shell = strdup(shell);
	if (cfg)
		sr_release_data(cfg);

	return shell;
}

/*
 * Used both when deleting a user from the system configuration, and
 * when cleaning up stale users while adding a new user to the system.
 * In the latter case we don't need to log failure/success.
 */
static int sys_del_user(char *user, bool silent)
{
	char *args[] = {
		"deluser", "--remove-home", user, NULL
	};
	int err;

	erasef("/var/run/sshd/%s.keys", user);
	err = systemv_silent(args);
	if (err) {
		if (!silent)
			ERROR("Error deleting user \"%s\"", user);

		/* Ensure $HOME is removed at least. */
		systemf("rm -rf /home/%s", user);

		return SR_ERR_SYS;
	}

	if (!silent)
		NOTE("User \"%s\" deleted", user);

	return SR_ERR_OK;
}

/*
 * Helper for sys_add_user(), notice how uid:gid must never be 0:0, it
 * is reserved for the system root user which is never created/deleted.
 *
 * This function balances both /etc/passwd and /etc/group, so to make
 * things a little bit easier, we simplify the world and reserve the
 * same group name as the username.  This means we can always remove
 * any stale group entries first.
 *
 * When uid:gid is non-zero we create the user and re-attach them with
 * their $HOME.  When uid:gid *is* zero, however, we must make sure to
 * clean up any stale user + $HOME before calling adduser to allocate
 * a new uid:gid pair for the new user.
 *
 * If this function fails, it is up to the callee to retry the operation
 * with a zero uid:gid pair, which is the most iron clad form.
 */
static int sys_call_adduser(sr_session_ctx_t *sess, char *name, uid_t uid, gid_t gid)
{
	char *shell = sys_find_usable_shell(sess, name);
	char *eargs[] = {
		"adduser", "-d", "-s", shell, "-u", NULL, "-G", NULL, "-H", name, NULL
	};
	char *nargs[] = {
		"adduser", "-d", "-s", shell, name, NULL
	};
	char uid_str[10], grp_str[strlen(name) + 1];
	char **args;
	int err;

	DEBUG("Adding new user \"%s\", cleaning up any stale group.", name);
	systemf("delgroup %s 2>/dev/null", name);

	/* reusing existing uid:gid from $HOME */
	if (uid && gid) {
		/* recreate group first */
		systemf("addgroup -g %d %s", gid, name);

		snprintf(uid_str, sizeof(uid_str), "%d", uid);
		snprintf(grp_str, sizeof(grp_str), "%s", name);
		eargs[5] = uid_str;
		eargs[7] = grp_str;

		args = eargs;
	} else {
		DEBUG("Cleaning up any stale user %s and /home/%s", name, name);
		sys_del_user(name, true);

		args = nargs;
	}

	/**
	 * The Busybox implementation of 'adduser -d' sets the password
	 * to "*", which prevents new users from logging in until we
	 * conclude the session by setting their shadow password and/or
	 * any SSH public keys have been installed.
	 */
	err = systemv_silent(args);
	if (!err)
		adjust_access(name, shell);
	free(shell);

	return err;
}

/*
 * Create a new (locked) user, if they have a matching $HOME we use the
 * uid:gid from that, if they are free.  In all cases of conflict with
 * any (type of) user we remove the existing $HOME and start with a new
 * uid:gid pair.
 */
static int sys_add_user(sr_session_ctx_t *sess, char *name)
{
	char home[strlen(name) + 10];
	bool reused = false;
	struct stat st;
	int err;

	/* Map users to their existing $HOME */
	snprintf(home, sizeof(home), "/home/%s", name);
	if (!stat(home, &st)) {
		/* Verify IDs aren't already used, like BusyBox adduser */
		if (getpwuid(st.st_uid) || getgrgid(st.st_uid) || getgrgid(st.st_gid)) {
			/* Exists but owned by someone else. */
			AUDIT("Failed mapping user \"%s\" to /home/%s, uid:gid (%d:%d) already exists.",
			      name, name, st.st_uid, st.st_gid);
			err = sys_call_adduser(sess, name, 0, 0);
		} else {
			AUDIT("Reusing uid:gid %d:%d and /home/%s for new user \"%s\"",
				 st.st_uid, st.st_gid, name, name);
			err = sys_call_adduser(sess, name, st.st_uid, st.st_gid);
			if (err) {
				AUDIT("Failed reusing uid:gid from /home/%s, retrying create user ...", name);
				err = sys_call_adduser(sess, name, 0, 0);
			} else
				reused = true;
		}
	} else
		err = sys_call_adduser(sess, name, 0, 0);

	if (err) {
		AUDIT("Failed creating new user \"%s\"", name);
		return SR_ERR_SYS;
	}

	AUDIT("User \"%s\" created%s.", name, reused ? ", mapped to existing home directory" : "");

	/*
	 * OpenSSH in Infix has been set up to use /var/run/sshd/%s.keys
	 * but libSSH used by netopeer2-server still reads the classic
	 * /home/%s/.ssh/authorized_keys file.  This creates a both the
	 * directory and the symlink owned by root to prevent tampering.
	 */
	DEBUG("Adding secure /home/%s/.ssh directory.", name);
	fmkpath(0750, "/home/%s/.ssh", name);
	systemf("ln -sf /var/run/sshd/%s.keys /home/%s/.ssh/authorized_keys", name, name);

	return SR_ERR_OK;
}

static char *change_get_user(struct sr_change *change)
{
	sr_xpath_ctx_t state;
	sr_val_t *val;
	char *user;

	val = change->old ? : change->new;
	assert(val);

	user = sr_xpath_key_value(val->xpath, "user", "name", &state);
	if (!user) {
		sr_xpath_recover(&state);
		return NULL;
	}

	user = strdup(user);
	sr_xpath_recover(&state);
	if (user) {
		const struct passwd *pw = getpwnam(user);

		if (!pw) {
			/* Skipping, user probably deleted. */
			free(user);
			user = NULL;
		}
	}

	return user;
}

static int set_shell(const char *user, const char *shell)
{
	struct passwd *pw;
	FILE *fp = NULL;
	int fd = -1;

	adjust_access(user, shell);

	setpwent();

	fp = fopen(_PATH_PASSWD "+", "w");
	if (!fp) {
		ERRNO("Failed opening %s+ for %s", _PATH_PASSWD, user);
		goto fail;
	}
	fd = fileno(fp);
	if (fd == -1 || fchown(fd, 0, 0) || fchmod(fd, 0644))
		goto fail;

	while ((pw = getpwent())) {
		struct passwd upw;

		if (!strcmp(pw->pw_name, user)) {
			if (strcmp(pw->pw_shell, shell))
				AUDIT("Updating login shell for user \"%s\" to %s", user, shell);

			upw = *pw;
			upw.pw_shell = (char *)shell;
			pw = &upw;
		}

		if (putpwent(pw, fp))
			goto fail;
	}

	/* Ensure all calls are made, if either fails we bail */
	if (fclose(fp) || rename(_PATH_PASSWD "+", _PATH_PASSWD)) {
		fp = NULL;
		goto fail;
	}
	endpwent();

	return 0;
fail:
	if (fp)
		fclose(fp);
	endpwent();
	ERRNO("Failed setting user \"%s\" shell %s", user, shell);

	return -1;
}

static int set_password(const char *user, const char *hash, bool lock)
{
	struct spwd *sp;
	FILE *fp = NULL;
	int fd = -1;

	if (lckpwdf())
		goto exit;

	setspent();

	fp = fopen(_PATH_SHADOW "+", "w");
	if (!fp) {
		ERRNO("Failed opening %s+ for user \"%s\"", _PATH_SHADOW, user);
		goto fail;
	}
	fd = fileno(fp);
	if (fd == -1 || fchown(fd, 0, 0) || fchmod(fd, 0600))
		goto fail;

	while ((sp = getspent())) {
		struct spwd usp;

		if (!strcmp(sp->sp_namp, user)) {
			usp = *sp;
			usp.sp_pwdp = lock ? "!" : (char *)hash;
			/*
			 * Disable password aging & C:o, we cannot rely
			 * on time being correct.
			 */
			usp.sp_lstchg = -1;
			usp.sp_min    = -1;
			usp.sp_max    = -1;
			usp.sp_warn   = -1;
			usp.sp_inact  = -1;
			usp.sp_expire = -1;
			sp = &usp;
		}

		if (putspent(sp, fp))
			goto fail;
	}

	/* Ensure all calls are made, if either fails we bail */
	if (fclose(fp) || rename(_PATH_SHADOW "+", _PATH_SHADOW)) {
		fp = NULL;
		goto fail;
	}
	endspent();
	ulckpwdf();

	return 0;
fail:
	if (fp)
		fclose(fp);
	endspent();
	ulckpwdf();
exit:
	AUDIT("Failed setting password for user \"%s\"", user);

	return -1;
}

/*
 * The iana-crypt-hash yang model is used to validate password hashes.
 * This function traps empty passwords and the $factory$ keyword.
 *
 * Empty passwords are not allowed, but instead of locking ("!") the
 * account, we disable password login with "*", allowing the user to
 * log in with SSH keys.
 *
 * The $factory$ keyword hash is converted to device's default password
 * hash, or NULL if it's not available, in which case the account will
 * be locked.
 */
static const char *is_valid_hash(struct confd *confd, const char *user, const char *hash)
{
	const char *factory = "$factory$";

	if (!hash || !strlen(hash))
		return "*";

	if (!strncmp(hash, factory, strlen(factory))) {
		struct json_t *pwd;

		pwd = json_object_get(confd->root, "factory-password-hash");
		if (!json_is_string(pwd)) {
			EMERG("Cannot find factory-default password hash for user \"%s\"!", user);
			return NULL;
		}

		hash = json_string_value(pwd);
	}

	return hash;
}

static sr_error_t handle_sr_passwd_update(sr_session_ctx_t *, struct confd *confd, struct sr_change *change)
{
	sr_error_t err = SR_ERR_OK;
	const char *hash;
	char *user;
	bool lock;

	user = change_get_user(change);
	if (!user)
		return SR_ERR_OK;

	switch (change->op) {
	case SR_OP_CREATED:
	case SR_OP_MODIFIED:
		assert(change->new);

		if (change->new->type != SR_STRING_T) {
			AUDIT("Internal error, expected user \"%s\" password to be string type.", user);
			err = SR_ERR_INTERNAL;
			break;
		}

		hash = is_valid_hash(confd, user, change->new->data.string_val);
		if (!hash) {
			/*
			 * Do not fail, lock account instead.  This way developers can
			 * enable root account login at build-time to diagnose the system.
			 */
			lock = true;
		} else
			lock = false;

		if (set_password(user, hash, lock))
			err = SR_ERR_SYS;
		else if (lock)
			NOTE("User account \"%s\" locked.", user);
		else if (!strcmp(hash, "*"))
			NOTE("Password login disabled for user \"%s\"", user);
		else
			AUDIT("Password updated for user \"%s\"", user);
		break;
	case SR_OP_DELETED:
		if (set_password(user, "*", false))
			err = SR_ERR_SYS;
		else
			NOTE("Password login disabled for user \"%s\"", user);
		break;
	case SR_OP_MOVED:
		break;
	}

	free(user);
	return err;
}

static sr_error_t handle_sr_shell_update(sr_session_ctx_t *sess, struct confd *confd, struct sr_change *change)
{
	char *shell = NULL;
	char *user;
	int err;

	if (change->op == SR_OP_DELETED)
		return SR_ERR_OK;

	user = change_get_user(change);
	if (!user)
		return SR_ERR_OK;

	shell = sys_find_usable_shell(sess, (char *)user);
	if (set_shell(user, shell)) {
		AUDIT("Failed updating shell to %s for user \"%s\"", shell, user);
		err = SR_ERR_SYS;
	} else {
		AUDIT("Login shell updated for user \"%s\"", user);
		err = SR_ERR_OK;
	}
	free(shell);
	free(user);

	return err;
}

static sr_error_t check_sr_user_update(sr_session_ctx_t *, struct confd *, struct sr_change *change)
{
	sr_xpath_ctx_t state;
	sr_val_t *val;
	char *name;

	val = change->old ? : change->new;
	assert(val);

	name = sr_xpath_key_value(val->xpath, "user", "name", &state);
	if (!is_valid_username(name)) {
		AUDIT("Invalid username \"%s\"", name);
		return SR_ERR_VALIDATION_FAILED;
	}
	NOTE("Username \"%s\" is valid", name);

	return SR_ERR_OK;
}

static sr_error_t handle_sr_user_update(sr_session_ctx_t *sess, struct confd *, struct sr_change *change)
{
	sr_xpath_ctx_t state;
	char *name;
	sr_error_t err;

	switch (change->op) {
	case SR_OP_CREATED:
		assert(change->new);

		name = sr_xpath_key_value(change->new->xpath, "user", "name", &state);
		err = sys_add_user(sess, name);
		if (err) {
			sr_xpath_recover(&state);
			return err;
		}
		sr_xpath_recover(&state);
		break;
	case SR_OP_DELETED:
		assert(change->old);

		name = sr_xpath_key_value(change->old->xpath, "user", "name", &state);
		err = sys_del_user(name, false);
		if (err) {
			sr_xpath_recover(&state);
			return err;
		}
		sr_xpath_recover(&state);
		break;
	case SR_OP_MOVED:
	case SR_OP_MODIFIED:
		return SR_ERR_OK;
	}

	return SR_ERR_OK;
}

static int lyx_list_is_empty(struct lyd_node *parent, const char *list)
{
	struct lyd_node *elem;
	int num = 0;

	LYX_LIST_FOR_EACH(parent, elem, list) {
		num++;
	}

	return num == 0;
}

static sr_error_t generate_auth_keys(sr_session_ctx_t *session, const char *xpath)
{
	struct lyd_node *auth, *user, *key;
	sr_error_t err = 0;
	sr_data_t *cfg;

	/* err may be OK and cfg NULL if 'no system' */
	err = sr_get_data(session, xpath, 0, 0, 0, &cfg);
	if (err || !cfg)
		return err;

	auth = lydx_get_descendant(cfg->tree, "system", "authentication", NULL);
	if (!auth) {
		ERROR("cannot find 'ietf-system:authentication'");
		goto err_release_data;
	}

	LYX_LIST_FOR_EACH(lyd_child(auth), user, "user") {
		const char *username = lydx_get_cattr(user, "name");
		FILE *fp;

		if (lyx_list_is_empty(lyd_child(user), "authorized-key")) {
			erasef("/var/run/sshd/%s.keys", username);
			continue;
		}

		fp = fopenf("w", "/var/run/sshd/%s.keys", username);
		if (!fp) {
			ERROR("failed opening user \"%s\" authorized_keys file: %s", username, strerror(errno));
			continue;
		}

		LYX_LIST_FOR_EACH(lyd_child(user), key, "authorized-key") {
			fprintf(fp, "%s %s %s\n",
				lydx_get_cattr(key, "algorithm"),
				lydx_get_cattr(key, "key-data"),
				lydx_get_cattr(key, "name") ?: username);
		}
		fclose(fp);
	}

err_release_data:
	sr_release_data(cfg);

	return err;
}

static const char *ctp_crypt(void)
{
	struct {
		const char *crypt;
		const char *prefix;
	} list[] = {
		{ "md5crypt",    "$1$" },
		{ "sha256crypt", "$5$" },
		{ "sha512crypt", "$6$" },
		{ "yescrypt",    "$y$" },
	};
	size_t i;

	for (i = 0; i < NELEMS(list); i++) {
		if (strcmp(list[i].crypt, DEFAULT_CRYPT))
			continue;

		return list[i].prefix;
	}

	return "$y$";		/* fallback */
}

static int ctp_hash_is_cleartext(const char *hash)
{
    if (strlen(hash) < 3)
        return 0;

    if (hash[0] == '$' && hash[1] == '0' && hash[2] == '$')
        return 1;

    return 0;
}

static sr_error_t check_user_ctp(sr_session_ctx_t *session, struct confd *_, struct sr_change *change)
{
	const char *prefix = ctp_crypt();
	char *hash, *salt;
	sr_val_t *val;

	val = change->new;
	if (!val)
		return SR_ERR_OK;

	hash = val->data.string_val;
	if (!ctp_hash_is_cleartext(hash))
		return SR_ERR_OK;

	if (strlen(hash) < 4) {
		sr_session_set_error(session, NULL, SR_ERR_VALIDATION_FAILED, "Too short password.");
		return SR_ERR_VALIDATION_FAILED;
	}

	salt = crypt_gensalt(prefix, 0, NULL, 0);
	if (!salt) {
		sr_session_set_error(session, NULL, SR_ERR_INTERNAL, "error %d generating salt, "
				     "prefix '%s'.", errno, prefix);
		return SR_ERR_INTERNAL;
	}

	hash = crypt(hash + 3, salt);
	if (!hash) {
		sr_session_set_error(session, NULL, SR_ERR_INTERNAL, "failed hashing password, "
				     "error %d.", errno);
		return SR_ERR_INTERNAL;
	}

	return sr_set_item_str(session, val->xpath, hash, NULL, 0);
}

static sr_error_t change_auth_ctp(struct confd *confd, sr_session_ctx_t *session)
{
	sr_error_t err;

	err = _sr_change_iter(session, confd, XPATH_AUTH_"/user[*]/password", check_user_ctp);
	if (err)
		return err;

	return SR_ERR_OK;
}

static sr_error_t change_auth_check(struct confd *confd, sr_session_ctx_t *session)
{
	sr_error_t err;

	err = _sr_change_iter(session, confd, XPATH_AUTH_"/user", check_sr_user_update);
	if (err)
		return err;

	return SR_ERR_OK;
}

static sr_error_t change_auth_done(struct confd *confd, sr_session_ctx_t *session)
{
	sr_error_t err;

	err = _sr_change_iter(session, confd, XPATH_AUTH_"/user", handle_sr_user_update);
	if (err)
		return err;

	err = _sr_change_iter(session, confd, XPATH_AUTH_"/user[*]/password", handle_sr_passwd_update);
	if (err)
		goto cleanup;

	err = _sr_change_iter(session, confd, XPATH_AUTH_"/user[*]/shell", handle_sr_shell_update);
	if (err)
		goto cleanup;

	err = generate_auth_keys(session, XPATH_AUTH_"/user//.");
	if (err) {
		AUDIT("failed saving authorized-key data.");
		goto cleanup;
	}

	DEBUG("Changes to authentication saved.");
cleanup:
	return err;
}

static int change_auth(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		       const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	struct confd *confd = (struct confd *)priv;

	switch (event) {
	case SR_EV_UPDATE:
		return change_auth_ctp(confd, session);
	case SR_EV_CHANGE:
		return change_auth_check(confd, session);
	case SR_EV_DONE:
		return change_auth_done(confd, session);
	default:
		break;
	}

	return SR_ERR_OK;
}

static int auth_cb(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		    const char *path, const char *request_path, uint32_t request_id,
		    struct lyd_node **parent, void *priv)
{
	struct spwd *spwd;

	ERROR("%s() path %s reqeust_path %s", __func__, path, request_path);

	setspent();
	while ((spwd = getspent())) {
		const char *fmt = "/ietf-system:system/authentication/user[name='%s']/password";
		char xpath[256];

		if (!spwd->sp_pwdp || spwd->sp_pwdp[0] == '*' || spwd->sp_pwdp[0] == '!')
			continue;

		snprintf(xpath, sizeof(xpath), fmt, spwd->sp_namp);
		lyd_new_path(*parent, NULL, xpath, spwd->sp_pwdp, 0, 0);
	}

	endspent();
	return SR_ERR_OK;
}

static int change_nacm(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		       const char *_, sr_event_t event, unsigned request_id, void *priv)
{
	sr_val_t *users = NULL;
	size_t user_count = 0;
	int rc;

	if (event != SR_EV_DONE)
		return SR_ERR_OK;

	/* Fetch all users from ietf-system */
	rc = sr_get_items(session, XPATH_AUTH_"/user/name", 0, 0, &users, &user_count);
	if (SR_ERR_OK != rc) {
		ERROR("Failed fetching system users: %s", sr_strerror(rc));
		goto cleanup;
	}

	for (size_t i = 0; i < user_count; i++) {
		const char *user = users[i].data.string_val;
		bool is_admin = is_admin_user(session, user);
		const char *shell;

		shell = sys_find_usable_shell(session, (char *)user);
		if (set_shell(user, shell))
			AUDIT("Failed adjusting shell for user \"%s\"", user);

		if (is_admin)
			add_group(user, "wheel");
		else
			del_group(user, "wheel");
	}

cleanup:
	if (users)
		sr_free_values(users, user_count);

	return 0;
}

static int change_motd(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		       const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	const char *fn = "/etc/motd";
	char *message;
	int rc = 0;

	/* Ignore all events except SR_EV_DONE */
	if (event != SR_EV_DONE)
		return SR_ERR_OK;

	message = srx_get_str(session, "%s", xpath);
	if (message) {
		rc = writesf(message, "w", "%s", fn);
		free(message);
	}

	if (rc) {
		ERRNO("failed saving %s", fn);
		return SR_ERR_SYS;
	}

	return SR_ERR_OK;
}

static int change_motd_banner(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		       const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	const char *fn = "/etc/motd";
	unsigned char *raw;
	char *legacy;
	int rc = 0;

	/* Ignore all events except SR_EV_DONE */
	if (event != SR_EV_DONE)
		return SR_ERR_OK;

	legacy = srx_get_str(session, "/ietf-system:system/infix-system:motd");
	if (legacy) {
		NOTE("Legacy /system/motd exists, skipping %s", xpath);
		free(legacy);
		return SR_ERR_OK;
	}

	raw = (unsigned char *)srx_get_str(session, "%s", xpath);
	if (raw) {
		unsigned char *txt;
		size_t txt_len;

		txt = base64_decode(raw, strlen((char *)raw), &txt_len);
		if (!txt) {
			ERRNO("failed base64 decoding of %s", xpath);
			rc = -1;
		} else {
			FILE *fp = fopen(fn, "w");

			if (!fp) {
				rc = -1;
			} else {
				size_t len = fwrite(txt, sizeof(txt[0]), txt_len, fp);

				if (len != txt_len) {
					ERROR("failed writing %s, wrote %zu bytes of total %zu",
					      fn, len, txt_len);
					rc = -1;
				}
				fclose(fp);
			}

			free(txt);
		}

		free(raw);
	} else {
		(void)remove(fn);
	}

	if (rc) {
		ERRNO("failed saving %s", fn);
		return SR_ERR_SYS;
	}

	return SR_ERR_OK;
}

static int change_editor(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
			 const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	const char *alt = "/etc/alternatives/editor";
	struct { const char *editor, *path; } map[] = {
		{ "emacs", "/usr/bin/mg" },
		{ "nano",  "/usr/bin/nano" },
		{ "vi",    "/bin/vi" },
	};
	char *editor;
	int rc = 0;

	/* Ignore all events except SR_EV_DONE */
	if (event != SR_EV_DONE)
		return SR_ERR_OK;

	editor = srx_get_str(session, "%s", xpath);
	if (!editor)
		return SR_ERR_OK;

	for (size_t i = 0; i < NELEMS(map); i++) {
		if (strcmp(map[i].editor, editor))
			continue;

		erase(alt);
		rc = systemf("ln -s %s %s", map[i].path, alt);
		if (rc)
			ERROR("Failed setting system editor '%s'", map[i].editor);
	}
	free(editor);

	if (rc)
		return SR_ERR_SYS;

	return SR_ERR_OK;
}

static char *get_mac(struct confd *confd, char *mac, size_t len)
{
	struct json_t *obj;

	obj = json_object_get(confd->root, "mac-address");
	if (!json_is_string(obj)) {
	fallback:
		ERROR("Unknown or missing base MAC address.");
		snprintf(mac, len, "UNKNOWN");
	} else {
		const char *ptr = json_string_value(obj);

		if (strlen(ptr) < 17)
			goto fallback;

		strlcpy(mac, &ptr[9], len);
		mac[2] = '-';
		mac[5] = '-';
	}

	return mac;
}

#define REPLACE_SPECIFIER(replacement)				\
	do {							\
		size_t repl_len = strlen(replacement);		\
		char *ptr;					\
								\
		hostlen += repl_len;				\
		ptr = realloc(hostname, hostlen + 1);		\
		if (!ptr) {					\
			free(hostname);				\
			free(*fmt);				\
			return -1;				\
		}						\
		hostname = ptr;					\
		strlcat(hostname, replacement, hostlen + 1);	\
		j += repl_len;					\
	} while (0)

/*
 * Decode hostname format specifiers (non-standard, Infix specifc)
 *
 * %i: OS ID (from /etc/os-release). infix on vanilla builds,
 * %h: Default hostname (DEFAULT_HOSTNAME from /etc/os-release). infix on vanilla builds,
 * %m: NIC specific part of base MAC, e.g., c0:ff:ee translated to c0-ff-ee
 * %%: Literal %
 *
 * NOTE: to be forward compatible, any unknown % combination will be silently consumed.
 *       E.g., "example-%z" will become "example-"
 *
 * XXX: PLEASE REFACTOR THIS INTO A PYTHON HELPER FOR FUTURE EXTENSIONS, OR BUGS!
 */
static int hostnamefmt(struct confd *confd, char **fmt)
{
	size_t hostlen, fmt_len;
	char *hostname;
	char mac[10];
	size_t i, j;

	if (!fmt || !*fmt || !strchr(*fmt, '%'))
		return 0;

	hostlen = fmt_len = strlen(*fmt);
	hostname = calloc(hostlen + 1, sizeof(char));
	if (!hostname)
		return -1;

	for (i = 0, j = 0; i < fmt_len; i++) {
		if ((*fmt)[i] == '%') {
			switch ((*fmt)[++i]) {
			case 'i':
				REPLACE_SPECIFIER(id);
				break;
			case 'h':
				REPLACE_SPECIFIER(nm);
				break;
			case 'm':
				REPLACE_SPECIFIER(get_mac(confd, mac, sizeof(mac)));
				break;
			case '%':
				if (j < hostlen) {
					hostname[j++] = '%';
					hostname[j]   = 0;
				}
				break;
			default:
				break; /* Unknown, skip */
			}
		} else {
			if (j < hostlen) {
				hostname[j++] = (*fmt)[i];
				hostname[j]   = 0;
			}
		}
	}

	free(*fmt);
	*fmt = hostname;

	return 0;
}

#undef REPLACE_SPECIFIER

static int change_hostname(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	struct confd *confd = (struct confd *)priv;
	const char *hostip = "127.0.1.1";
	char *hostnm, buf[256];
	FILE *nfp, *fp;
	int err, fd;

	if (event != SR_EV_DONE)
		return SR_ERR_OK;

	hostnm = srx_get_str(session, "%s", xpath);
	if (!hostnm) {
	fallback:
		hostnm = strdup(nm);
		if (!hostnm) {
			err = SR_ERR_NO_MEMORY;
			goto err;
		}
	} else if (hostnamefmt(confd, &hostnm))
		goto fallback;

	err = sethostname(hostnm, strlen(hostnm));
	if (err) {
		ERROR("failed setting hostname");
		err = SR_ERR_SYS;
		goto err;
	}

	fp = fopen(_PATH_HOSTNAME, "w");
	if (!fp) {
		err = SR_ERR_INTERNAL;
		goto err;
	}

	fprintf(fp, "%s\n", hostnm);
	fclose(fp);

	nfp = fopen(_PATH_HOSTS "+", "w");
	if (!nfp) {
		err = SR_ERR_INTERNAL;
		goto err;
	}
	fd = fileno(nfp);
	if (fd == -1 || fchown(fd, 0, 0) || fchmod(fd, 0644)) {
		fclose(nfp);
		goto err;
	}

	fp = fopen(_PATH_HOSTS, "r");
	if (!fp) {
		err = SR_ERR_INTERNAL;
		fclose(nfp);
		goto err;
	}

	while (fgets(buf, sizeof(buf), fp)) {
		if (!strncmp(buf, hostip, strlen(hostip)))
			snprintf(buf, sizeof(buf), "%s\t%s\n", hostip, hostnm);
		fputs(buf, nfp);
	}

	fclose(fp);
	fclose(nfp);
	if (rename(_PATH_HOSTS "+", _PATH_HOSTS))
		ERRNO("Failed activating changes to "_PATH_HOSTS);

	/* skip in bootstrap, lldpd and avahi have not started yet */
	if (systemf("runlevel >/dev/null 2>&1"))
		goto err;

	/* Inform any running lldpd and avahi of the change ... */
	systemf("lldpcli configure system hostname %s", hostnm);
	systemf("avahi-set-host-name %s", hostnm);
	systemf("initctl -nbq touch netbrowse");
err:
	if (hostnm)
		free(hostnm);

	if (err) {
		ERROR("Failed activating changes.");
		return err;
	}

	if (sys_reload_services())
		return SR_ERR_SYS;

	return SR_ERR_OK;
}

static int hostname_cb(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		       const char *path, const char *request_path, uint32_t request_id,
		       struct lyd_node **parent, void *priv)
{
	char hostname[128];
	int rc;

	gethostname(hostname, sizeof(hostname));
	rc = lyd_new_path(*parent, NULL, path, hostname, 0, NULL);
	if (rc) {
		ERROR("Failed building data tree, libyang error %d", rc);
		rc = SR_ERR_INTERNAL;
	}

	return rc;
}

int ietf_system_init(struct confd *confd)
{
	int rc;

	os_init();

	REGISTER_CHANGE(confd->session, "ietf-system", XPATH_AUTH_, SR_SUBSCR_UPDATE, change_auth, confd, &confd->sub);
	REGISTER_OPER(confd->session, "ietf-system", PASSWORD_PATH, auth_cb, confd, 0, &confd->sub);
	REGISTER_MONITOR(confd->session, "ietf-netconf-acm", "/ietf-netconf-acm:nacm//.",
			 0, change_nacm, confd, &confd->sub);

	REGISTER_CHANGE(confd->session, "ietf-system", XPATH_BASE_"/hostname", 0, change_hostname, confd, &confd->sub);
	REGISTER_OPER(confd->session, "ietf-system", XPATH_BASE_"/hostname", hostname_cb, confd, 0, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-system", XPATH_BASE_"/infix-system:motd", 0, change_motd, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-system", XPATH_BASE_"/infix-system:motd-banner", 0, change_motd_banner, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-system", XPATH_BASE_"/infix-system:text-editor", 0, change_editor, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-system", XPATH_BASE_"/clock", 0, change_clock, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-system", XPATH_BASE_"/ntp", 0, change_ntp, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-system", XPATH_BASE_"/dns-resolver", 0, change_dns, confd, &confd->sub);

	REGISTER_OPER(confd->session, "ietf-system", CLOCK_PATH_, clock_cb, NULL, 0, &confd->sub);
	REGISTER_OPER(confd->session, "ietf-system", PLATFORM_PATH_, platform_cb, NULL, 0, &confd->sub);

	REGISTER_RPC(confd->session, "/ietf-system:system-restart",  rpc_exec, "reboot", &confd->sub);
	REGISTER_RPC(confd->session, "/ietf-system:system-shutdown", rpc_exec, "poweroff", &confd->sub);
	REGISTER_RPC(confd->session, "/ietf-system:set-current-datetime", rpc_set_datetime, NULL, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
