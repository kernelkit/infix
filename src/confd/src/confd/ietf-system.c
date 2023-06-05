/* SPDX-License-Identifier: BSD-3-Clause */

#include <assert.h>
#include <ctype.h>
#include <pwd.h>
#include <sys/utsname.h>
#include <sys/sysinfo.h>
#include <sys/types.h>

#include "core.h"
#include "lyx.h"
#include "srx_module.h"
#include "srx_val.h"

#define CLOCK_PATH_    "/ietf-system:system-state/clock"
#define PLATFORM_PATH_ "/ietf-system:system-state/platform"

static const char *sysfeat[] = {
	"authentication",
	"local-users",
	"ntp",
	"ntp-udp-port",
	"timezone-name",
	NULL
};

static const struct srx_module_requirement ietf_system_reqs[] = {
	{ .dir = YANG_PATH_, .name = "ietf-system", .rev = "2014-08-06", .features = sysfeat },
	{ .dir = YANG_PATH_, .name = "infix-system", .rev = "2023-04-11" },

	{ NULL }
};

struct sr_change {
	sr_change_oper_t op;
	sr_val_t *old;
	sr_val_t *new;
};

static char   *ver = NULL;
static char   *rel = NULL;
static char   *sys = NULL;
static char   *os  = NULL;

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

static void setvar(char *line, const char *nm, char **var)
{
	char *ptr;

	if (!strncmp(line, nm, strlen(nm)) && (ptr = strchr(line, '='))) {
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
	}
	fclose(fp);
}

static char *fmtime(time_t t, char *buf, size_t len)
{
        const char *isofmt = "%FT%T%z";
        struct tm tm;
        size_t i, n;

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

static sr_error_t _sr_change_iter(augeas *aug, sr_session_ctx_t *session, char *xpath,
				  sr_error_t cb(augeas *, struct sr_change *))
{
	struct sr_change change = {};
	sr_change_iter_t *iter;
	sr_error_t err;

	err = sr_dup_changes_iter(session, xpath, &iter);
	if (err)
		return err;

	while (sr_get_change_next(session, iter, &change.op, &change.old, &change.new) == SR_ERR_OK) {
		err = cb(aug, &change);
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
	int first = 1;
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

	if ((rc = lydx_new_path(ctx, parent, &first, CLOCK_PATH_, "boot-datetime", boottime)))
		goto fail;
	if ((rc = lydx_new_path(ctx, parent, &first, CLOCK_PATH_, "current-datetime", curtime)))
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
	int first = 1;
	int rc;

	ctx = sr_acquire_context(sr_session_get_connection(session));

	if ((rc = lydx_new_path(ctx, parent, &first, PLATFORM_PATH_, "os-name", os)))
		goto fail;
	if ((rc = lydx_new_path(ctx, parent, &first, PLATFORM_PATH_, "os-release", rel)))
		goto fail;
	if ((rc = lydx_new_path(ctx, parent, &first, PLATFORM_PATH_, "os-version", ver)))
		goto fail;
	if ((rc = lydx_new_path(ctx, parent, &first, PLATFORM_PATH_, "machine", sys)))
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
	DEBUG("path: %s", path);

	if (system(priv))
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
	if (settimeofday(&tv, NULL)) {
		ERRNO("ietf-system:settimeofday() failed");
		goto done;
	}

	rc = SR_ERR_OK;
done:
	unsetenv("TZ");
        free(buf);

	return rc;
}

static int sys_reload_services(void)
{
	return systemf("initctl -nbq touch sysklogd lldpd");
}

static int aug_set_dynpath(augeas *aug, const char *val, const char *fmt, ...)
{
	va_list ap;
	char *path;
	int res;

	va_start(ap, fmt);
	res = vasprintf(&path, fmt, ap);
	va_end(ap);
	if (res == -1)
		return res;

	res = aug_set(aug, path, val);
	if (res != 0)
		ERROR("Unable to set aug path \"%s\" to \"%s\"\n", path, val);

	free(path);

	return res;
}

#define TIMEZONE_CONF "/etc/timezone"
#define TIMEZONE_PREV TIMEZONE_CONF "-"
#define TIMEZONE_NEXT TIMEZONE_CONF "+"

static int change_clock(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	char *timezone;

	switch (event) {
	case SR_EV_ENABLED:	/* first time, on register. */
	case SR_EV_CHANGE:	/* regular change (copy cand running) */
		/* Set up next timezone */
		break;

	case SR_EV_ABORT:	/* User abort, or other plugin failed */
		remove(TIMEZONE_NEXT);
		return SR_ERR_OK;

	case SR_EV_DONE:
		/* Check if passed validation in previous event */
		if (access(TIMEZONE_NEXT, F_OK))
			return SR_ERR_OK;

		remove(TIMEZONE_PREV);
		rename(TIMEZONE_CONF, TIMEZONE_PREV);
		rename(TIMEZONE_NEXT, TIMEZONE_CONF);

		remove("/etc/localtime-");
		rename("/etc/localtime",  "/etc/localtime-");
		rename("/etc/localtime+", "/etc/localtime");

		return SR_ERR_OK;

	default:
		return SR_ERR_OK;
	}

	/* XXX: add support also for /ietf-system:system/clock/timezone-utc-offset (deviation) */
	timezone = srx_get_str(session, "/ietf-system:system/clock/timezone-name");
	if (!timezone) {
		ERROR("Failed reading timezone-name");
		return SR_ERR_VALIDATION_FAILED;
	}

	remove("/etc/localtime+");
	if (systemf("ln -s /usr/share/zoneinfo/%s /etc/localtime+", timezone)) {
		ERROR("No such timezone %s", timezone);
		return SR_ERR_VALIDATION_FAILED;
	}

	if (writesf(timezone, "w", TIMEZONE_NEXT)) {
		ERRNO("Failed preparing %s", TIMEZONE_NEXT);
		return SR_ERR_SYS;
	}

	return SR_ERR_OK;
}

#define CHRONY_CONF "/etc/chrony.conf"
#define CHRONY_PREV CHRONY_CONF "-"
#define CHRONY_NEXT CHRONY_CONF "+"

static int change_ntp(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	const char *fn = CHRONY_NEXT;
	int valid = -1;
	sr_val_t *val;
	size_t cnt;
	FILE *fp;
	int rc;

	switch (event) {
	case SR_EV_ENABLED:	/* first time, on register. */
	case SR_EV_CHANGE:	/* regular change (copy cand running) */
		/* Generate next config */
		break;

	case SR_EV_ABORT:	/* User abort, or other plugin failed */
		remove(CHRONY_NEXT);
		return SR_ERR_OK;

	case SR_EV_DONE:
		/* Check if passed validation in previous event */
		if (access(CHRONY_NEXT, F_OK))
			return SR_ERR_OK;

		remove(CHRONY_PREV);
		rename(CHRONY_CONF, CHRONY_PREV);
		rename(CHRONY_NEXT, CHRONY_CONF);
		if (srx_enabled(session, "/ietf-system:system/ntp/enabled")) {
			systemf("initctl -nbq disable chronyd");
			return SR_ERR_OK;
		}
		/*
		 * If chrony is alrady enabled we tell Finit it's been
		 * modified , so Finit restarts it, otherwise enable it.
		 */
		systemf("initctl -nbq touch chronyd");
		systemf("initctl -nbq enable chronyd");
		return SR_ERR_OK;

	default:
		return SR_ERR_OK;
	}

	rc = sr_get_items(session, "/ietf-system:system/ntp/server", 0, 0, &val, &cnt);
	if (rc) {
		remove(CHRONY_NEXT);
		return rc;
	}

	fp = fopen(fn, "w");
	if (!fp) {
		ERROR("failed updating %s: %s", fn, strerror(errno));
		sr_free_values(val, cnt);
		return SR_ERR_SYS;
	}

	for (size_t i = 0; i < cnt; i++) {
		const char *xpath = val[i].xpath;
		char *type, *ptr;
		int server = 0;

		/*
		 * Handle empty startup-config on SR_EV_ENABLED,
		 * prevents subscribe failure due to false invalid.
		 */
		if (i == 0)
			valid = 0;

		/* Get /ietf-system:system/ntp/server[name='foo'] */
		ptr = srx_get_str(session, "%s/udp/address", xpath);
		if (ptr) {
			type = srx_get_str(session, "%s/association-type", xpath);
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

			fprintf(fp, "\n");
			valid++;
		}
	}
	sr_free_values(val, cnt);

	fprintf(fp, "driftfile /var/lib/chrony/drift\n");
	fprintf(fp, "makestep 1.0 3\n");
	fprintf(fp, "maxupdateskew 100.0\n");
	fprintf(fp, "dumpdir /var/lib/chrony\n");
	fprintf(fp, "rtcfile /var/lib/chrony/rtc\n");
	fclose(fp);

	if (!valid) {
		remove(fn);
		return SR_ERR_VALIDATION_FAILED;
	}

	return SR_ERR_OK;
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
		remove(RESOLV_NEXT);
		return SR_ERR_OK;

	case SR_EV_DONE:
		/* Check if passed validation in previous event */
		if (access(RESOLV_NEXT, F_OK))
			return SR_ERR_OK;

		remove(RESOLV_PREV);
		rename(RESOLV_CONF, RESOLV_PREV);
		rename(RESOLV_NEXT, RESOLV_CONF);

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
		sr_free_values(val, cnt);
		return SR_ERR_SYS;
	}

	SRX_GET_UINT8(session, timeout, "/ietf-system:system/dns-resolver/options/timeout");
	SRX_GET_UINT8(session, timeout, "/ietf-system:system/dns-resolver/options/attempts");
	if (timeout || attempts) {
		fprintf(fp, "options");
		if (timeout)
			fprintf(fp, " timeout:%d", timeout);
		if (attempts)
			fprintf(fp, " attempts:%d ", attempts);
		fprintf(fp, "\n");
	}

	rc = sr_get_items(session, "/ietf-system:system/dns-resolver/search", 0, 0, &val, &cnt);
	if (rc)
		goto fail;

	if (cnt) {
		fprintf(fp, "search ");
		for (size_t i = 0; i < cnt; i++)
			fprintf(fp, "%s ", val[i].data.string_val);
		fprintf(fp, "\n");
	}
	sr_free_values(val, cnt);

	rc = sr_get_items(session, "/ietf-system:system/dns-resolver/server", 0, 0, &val, &cnt);
	if (rc)
		goto fail;

	for (size_t i = 0; i < cnt; i++) {
		const char *xpath = val[i].xpath;
		char *ptr;

		/* Get /ietf-system:system/dns-resolver/server[name='foo'] */
		ptr = srx_get_str(session, "%s/udp-and-tcp/address", xpath);
		if (ptr)
			/* XXX: add support also for udp-and-tcp/port */
			fprintf(fp, "nameserver %s\n", ptr);
	}
	sr_free_values(val, cnt);

	rc = SR_ERR_OK;
fail:
	fclose(fp);
	if (rc)
		remove(fn);

	return rc;
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

static int sys_del_user(char *user)
{
	char *args[] = {
		"deluser", user, NULL
	};
	int err;

	erasef("/var/run/sshd/%s.keys", user);
	err = systemv_silent(args);
	if (err) {
		ERROR("Error deleting user \"%s\"\n", user);
		return SR_ERR_SYS;
	}
	DEBUG("User \"%s\" deleted\n", user);

	return SR_ERR_OK;
}

/*
 * XXX: drop superuser/wheel exception as soon as we have clish-ng in place.
 *      It is supposed to connect to the netopeer server via 127.0.0.1 so
 *      that the full NACM applies to the user instead of the file system
 *      permissions that apply to the current klish.
 */
static int sys_add_new_user(char *name)
{
	char *shell = LOGIN_SHELL;
	char *sargs[] = {
		"adduser", "-D", "-s", shell, "-S", "-G", "wheel", name, NULL
	};
	char *uargs[] = {
		"adduser", "-D", "-s", shell, name, NULL
	};
	char **args;
	int err;

	/* XXX: group mapping to wheel should be done using nacm ACLs instead. */
	if (!strcmp(name, "admin"))
		args = sargs;	/* superuser */
	else
		args = uargs;	/* user */

	if (!shell || !whichp(shell))
		args[2] = "/bin/sh";

	/**
	 * The Busybox implementation of adduser -D sets the password to "!",
	 * which should prevent the new user from logging in until Augeas has
	 * updated it.
	 */
	err = systemv_silent(args);
	if (err) {
		ERROR("Error creating new user \"%s\"\n", name);
		return SR_ERR_SYS;
	}
	DEBUG("New user \"%s\" created\n", name);

	return SR_ERR_OK;
}

static sr_error_t handle_sr_passwd_update(augeas *aug, struct sr_change *change)
{
	sr_xpath_ctx_t state;
	struct passwd *pw;
	const char *hash;
	sr_val_t *val;
	char *xpath;
	char *user;

	val = change->old ? : change->new;
	assert(val);

	xpath = sr_xpath_key_value(val->xpath, "user", "name", &state);
	user = strdup(xpath);
	sr_xpath_recover(&state);

	pw = getpwnam(user);
	if (!pw) {
		DEBUG("Skipping attribute for missing user (%s)\n", user);
		return SR_ERR_OK;
	}

	switch (change->op) {
	case SR_OP_CREATED:
	case SR_OP_MODIFIED:
		assert(change->new);

		if (change->new->type != SR_STRING_T) {
			ERROR("Internal error, expected pass to be string type\n");
			return SR_ERR_INTERNAL;
		}
		hash = change->new->data.string_val;

		/*
		 * The iana-crypt-hash yang model is used to validate password
		 * hash sanity, this is an emergency sanity check. We DON'T
		 * want an empty string here.
		 */
		if (!hash || !strlen(hash)) {
			ERROR("Password hash sanity check failed\n");
			return SR_ERR_INTERNAL;
		}
		if (aug_set_dynpath(aug, hash, "etc/shadow/%s/password", user))
			return SR_ERR_SYS;
		DEBUG("Password updated for user %s\n", user);
		break;
	case SR_OP_DELETED:
		if (aug_set_dynpath(aug, "!", "etc/shadow/%s/password", user))
			return SR_ERR_SYS;
		DEBUG("Password deleted for user %s\n", user);
		break;
	case SR_OP_MOVED:
		return SR_ERR_OK;
	}

	return SR_ERR_OK;
}

static sr_error_t check_sr_user_update(augeas *aug, struct sr_change *change)
{
	sr_xpath_ctx_t state;
	sr_val_t *val;
	char *name;

	val = change->old ? : change->new;
	assert(val);

	name = sr_xpath_key_value(val->xpath, "user", "name", &state);
	if (!is_valid_username(name)) {
		ERROR("Invalid username \"%s\"\n", name);
		return SR_ERR_VALIDATION_FAILED;
	}
	DEBUG("Username \"%s\" is valid\n", name);

	return SR_ERR_OK;
}

static sr_error_t handle_sr_user_update(augeas *aug, struct sr_change *change)
{
	sr_xpath_ctx_t state;
	char *name;
	sr_error_t err;

	switch (change->op) {
	case SR_OP_CREATED:
		assert(change->new);

		name = sr_xpath_key_value(change->new->xpath, "user", "name", &state);
		err = sys_add_new_user(name);
		if (err) {
			sr_xpath_recover(&state);
			return err;
		}
		DEBUG("User %s created\n", name);
		sr_xpath_recover(&state);
	break;
	case SR_OP_DELETED:
		assert(change->old);

		name = sr_xpath_key_value(change->old->xpath, "user", "name", &state);
		err = sys_del_user(name);
		if (err) {
			sr_xpath_recover(&state);
			return err;
		}
		DEBUG("User %s deleted\n", name);
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

	LYX_LIST_FOR_EACH(parent, elem, list)
		num++;

	return num == 0;
}

static sr_error_t generate_auth_keys(sr_session_ctx_t *session, const char *xpath)
{
	struct lyd_node *auth, *user, *key;
	sr_error_t err = 0;
	sr_data_t *cfg;

	err = sr_get_data(session, xpath, 0, 0, 0, &cfg);
	if (err)
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
			ERROR("failed opening %s authorized_keys file: %s", username, strerror(errno));
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

static sr_error_t change_auth_check(augeas *aug, sr_session_ctx_t *session)
{
	sr_error_t err;

	err = _sr_change_iter(aug, session, "/ietf-system:system/authentication/user",
			      check_sr_user_update);
	if (err)
		return err;

	return SR_ERR_OK;
}

static sr_error_t change_auth_done(augeas *aug, sr_session_ctx_t *session)
{
	sr_error_t err;

	err = _sr_change_iter(aug, session, "/ietf-system:system/authentication/user",
			      handle_sr_user_update);
	if (err)
		return err;

	/**
	 * Load any newly created user into aug.
	 *
	 * We want aug_load_file() here but it seems broken. The file doesn't appear
	 * to be reloaded after calling it. It returns no error and it seems to find
	 * the appropriate lens. Further investigation needed.
	 */
	err = aug_load(aug);
	if (err) {
		ERROR("Error loading files into aug tree\n");
		return SR_ERR_INTERNAL;
	}

	err = _sr_change_iter(aug, session, "/ietf-system:system/authentication/user[*]/password",
			      handle_sr_passwd_update);
	if (err)
		return err;

	err = aug_save(aug);
	if (err) {
		ERROR("Error saving auth changes\n");
		return SR_ERR_SYS;
	}

	err = generate_auth_keys(session, "/ietf-system:system/authentication/user//.");
	if (err) {
		ERROR("failed saving authorized-key data.");
		return err;
	}

	DEBUG("Changes to authentication saved\n");

	return SR_ERR_OK;
}

static int change_auth(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		       const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	struct confd *confd = (struct confd *)priv;

	if (event == SR_EV_CHANGE)
		return change_auth_check(confd->aug, session);
	if (event == SR_EV_DONE)
		return change_auth_done(confd->aug, session);

	return SR_ERR_OK;
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

	message = srx_get_str(session, xpath);
	if (message) {
		rc = writesf(message, "w", fn);
		free(message);
	} else {
		remove(fn);
	}

	if (rc) {
		ERROR("failed writing /etc/motd: %s", strerror(errno));
		return SR_ERR_SYS;
	}

	return SR_ERR_OK;
}

static int change_hostname(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	struct confd *confd = (struct confd *)priv;
	const char *host, *tmp = NULL;
	char **hosts, *current;
	int err, i, nhosts;
	char *nm;

	if (event != SR_EV_DONE)
		return SR_ERR_OK;

	nm = srx_get_str(session, xpath);
	if (!nm) {
		/* XXX: derive from global "options.h" or /usr/share/factory/ */
		nm = strdup("infix");
		if (!nm) {
			err = SR_ERR_NO_MEMORY;
			goto err;
		}
	}

	if (aug_get(confd->aug, "etc/hostname/hostname", &tmp) <= 0) {
		err = SR_ERR_INTERNAL;
		goto err;
	}

	current = strdup(tmp);
	if (!current) {
		err = SR_ERR_NO_MEMORY;
		goto err;
	}

	err = sethostname(nm, strlen(nm));
	err = err ? : aug_set(confd->aug, "etc/hostname/hostname", nm);

	nhosts = aug_match(confd->aug, "etc/hosts/*/canonical", &hosts);
	for (i = 0; i < nhosts; i++) {
		if (aug_get(confd->aug, hosts[i], &host) <= 0)
			continue;

		if (!strcmp(host, current))
			err = err ? : aug_set(confd->aug, hosts[i], nm);

		free(hosts[i]);
	}

	if (nhosts)
		free(hosts);
	free(current);
err:
	if (nm)
		free(nm);

	if (err) {
		ERROR("Failed activating changes.");
		return err;
	}

	err = err ? : aug_save(confd->aug);
	if (sys_reload_services())
		return SR_ERR_SYS;

	return SR_ERR_OK;
}

int ietf_system_init(struct confd *confd)
{
	int rc;

	os_init();

	/* TODO: Every file and lens seems to be already loaded at aug_init() */
	if (aug_load_file(confd->aug, "/etc/hostname") ||
	    aug_load_file(confd->aug, "/etc/hosts")) {
		ERROR("ietf-system: Augeas initialization failed");
		rc = SR_ERR_INTERNAL;
		goto fail;
	}

	rc = srx_require_modules(confd->conn, ietf_system_reqs);
	if (rc)
		goto fail;

	REGISTER_CHANGE(confd->session, "ietf-system", "/ietf-system:system/authentication", 0, change_auth, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-system", "/ietf-system:system/hostname", 0, change_hostname, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-system", "/ietf-system:system/infix-system:motd", 0, change_motd, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-system", "/ietf-system:system/clock", 0, change_clock, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-system", "/ietf-system:system/ntp", 0, change_ntp, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-system", "/ietf-system:system/dns-resolver", 0, change_dns, confd, &confd->sub);

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
