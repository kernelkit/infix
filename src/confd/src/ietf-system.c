/* SPDX-License-Identifier: BSD-3-Clause */

#include "core.h"
#include "srx_val.h"

#include <ctype.h>
#include <sys/utsname.h>
#include <sys/sysinfo.h>

#define CLOCK_PATH_    "/ietf-system:system-state/clock"
#define PLATFORM_PATH_ "/ietf-system:system-state/platform"

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

	if (systemf(priv))
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

	if (writesf(timezone, TIMEZONE_NEXT)) {
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
		if (systemf("initctl cond get hook/sys/up"))
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

static int change_motd(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		       const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	/* XXX: derive from global "options.h" or /usr/share/factory/ */
	const char *msg = "\033[1;90mNote:\033[0m"
		"\033[0;90m configuration is done using the CLI, type 'cli' at the shell prompt.\033[0m";
	char *str;
	int rc;

	/* Ignore all events except SR_EV_DONE */
	if (event != SR_EV_DONE)
		return SR_ERR_OK;

	str = srx_get_str(session, xpath);
	if (str) {
		rc = writesf(str, "/etc/motd");
		free(str);
	} else {
		rc = writesf(msg, "/etc/motd");
	}

	if (rc)
		return SR_ERR_SYS;

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
	const char *features[] = {
		"ntp",
		"ntp-udp-port",
		"timezone-name",
		NULL
	};
	int rc;

	os_init();

	if (aug_load_file(confd->aug, "/etc/hostname") ||
	    aug_load_file(confd->aug, "/etc/hosts")) {
		ERROR("ietf-system: Augeas initialization failed");
		rc = SR_ERR_INTERNAL;
		goto fail;
	}

	rc = sr_install_module(confd->conn, YANG_PATH_"/ietf-system@2014-08-06.yang", NULL, features);
	if (rc)
		goto fail;
	/* Augment to ietf-systems */
	rc = sr_install_module(confd->conn, YANG_PATH_"/infix-system@2014-08-06.yang", NULL, NULL);
	if (rc)
		goto fail;

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
