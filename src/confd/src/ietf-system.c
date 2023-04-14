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
	char *buf;
	int rc;

	DEBUG("path=%s, request_path=%s", path, request_path);
	ctx = sr_acquire_context(sr_session_get_connection(session));

	rc = lyd_new_path(NULL, ctx, CLOCK_PATH_, NULL, 0, parent);
	if (rc) {
	fail:
		ERROR("Failed building data tree, libyang error %d", rc);
		return SR_ERR_INTERNAL;
	}

	lyd_print_mem(&buf, *parent, LYD_XML, 0);
	DEBUG("%s", buf);

	now = time(NULL);
	if (!boottime[0]) {
		struct sysinfo si;

		sysinfo(&si);
		boot = now - si.uptime;
		fmtime(boot, boottime, sizeof(boottime));
	}

	rc = lyd_new_path(*parent, NULL, CLOCK_PATH_ "/boot-datetime", boottime, 0, NULL);
	if (rc)
		goto fail;

	fmtime(now, curtime, sizeof(curtime));
	rc = lyd_new_path(*parent, NULL, CLOCK_PATH_ "/current-datetime", curtime, 0, NULL);
	if (rc)
		goto fail;

	lyd_print_mem(&buf, *parent, LYD_XML, 0);
	DEBUG("%s", buf);

	return SR_ERR_OK;
}

static int platform_cb(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		       const char *path, const char *request_path, uint32_t request_id,
		       struct lyd_node **parent, void *priv)
{
	const struct ly_ctx *ctx;
	char *buf;
	int rc;

	DEBUG("path=%s request_path=%s", path, request_path);
	ctx = sr_acquire_context(sr_session_get_connection(session));

	rc = lyd_new_path(NULL, ctx, PLATFORM_PATH_, NULL, 0, parent);
	if (rc) {
	fail:
		ERROR("Failed building data tree, libyang error %d", rc);
		return SR_ERR_INTERNAL;
	}

	lyd_print_mem(&buf, *parent, LYD_XML, 0);
	DEBUG("%s", buf);

	rc = lyd_new_path(*parent, NULL, PLATFORM_PATH_"/os-name", os, 0, NULL);
	if (rc)
		goto fail;
	rc = lyd_new_path(*parent, NULL, PLATFORM_PATH_"/os-release", rel, 0, NULL);
	if (rc)
		goto fail;
	rc = lyd_new_path(*parent, NULL, PLATFORM_PATH_"/os-version", ver, 0, NULL);
	if (rc)
		goto fail;
	rc = lyd_new_path(*parent, NULL, PLATFORM_PATH_"/machine", sys, 0, NULL);
	if (rc)
		goto fail;

	lyd_print_mem(&buf, *parent, LYD_XML, 0);
	DEBUG("%s", buf);

	return SR_ERR_OK;
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

int sr_get_int(sr_session_ctx_t *session, const char *fmt, ...)
{
	sr_val_t *val = NULL;
	char *xpath;
	va_list ap;
	int rc = 0;
	int len;

	va_start(ap, fmt);
	len = vsnprintf(NULL, 0, fmt, ap) + 1;
	va_end(ap);

	xpath = alloca(len);
	if (!xpath)
		goto fail;

	va_start(ap, fmt);
	vsnprintf(xpath, len, fmt, ap);
	va_end(ap);

	if (sr_get_item(session, xpath, 0, &val) || !val)
		goto fail;

	switch (val->type) {
	case SR_INT8_T:
		rc = val->data.int8_val;
		break;
	case SR_UINT8_T:
		rc = val->data.uint8_val;
		break;
	case SR_INT16_T:
		rc = val->data.int16_val;
		break;
	case SR_UINT16_T:
		rc = val->data.uint16_val;
		break;
	case SR_INT32_T:
		rc = val->data.int32_val;
		break;
	case SR_UINT32_T:
		rc = val->data.uint32_val;
		break;
	case SR_INT64_T:
		rc = val->data.int64_val;
		break;
	case SR_UINT64_T:
		rc = val->data.uint64_val;
		break;
	default:
		goto fail;
	}

fail:
	if (val)
		sr_free_val(val);
	return rc;
}

int sr_get_bool(sr_session_ctx_t *session, const char *fmt, ...)
{
	sr_val_t *val = NULL;
	char *xpath;
	va_list ap;
	int rc = 0;
	int len;

	va_start(ap, fmt);
	len = vsnprintf(NULL, 0, fmt, ap) + 1;
	va_end(ap);

	xpath = alloca(len);
	if (!xpath)
		goto fail;

	va_start(ap, fmt);
	vsnprintf(xpath, len, fmt, ap);
	va_end(ap);

	if (sr_get_item(session, xpath, 0, &val))
		goto fail;
	if (!val || val->type != SR_BOOL_T)
		goto fail;

	rc = val->data.bool_val;
fail:
	if (val)
		sr_free_val(val);
	return rc;
}

#define TIMEZONE_CONF "/etc/timezone"
#define TIMEZONE_PREV TIMEZONE_CONF "-"
#define TIMEZONE_NEXT TIMEZONE_CONF "+"

static int change_clock(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	char *timezone;
	FILE *fp;

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

	fp = fopen(TIMEZONE_NEXT, "w");
	if (!fp) {
		ERRNO("Failed preparing %s", TIMEZONE_NEXT);
		return SR_ERR_SYS;
	}
	fprintf(fp, "%s\n", timezone);
	fclose(fp);

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
		if (!sr_get_bool(session, "/ietf-system:system/ntp/enabled")) {
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
			if (sr_get_bool(session, "%s/iburst", xpath))
				fprintf(fp, " iburst");
			if (sr_get_bool(session, "%s/prefer", xpath))
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
	int timeout, attempts;
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

	timeout  = sr_get_int(session, "/ietf-system:system/dns-resolver/options/timeout");
	attempts = sr_get_int(session, "/ietf-system:system/dns-resolver/options/attempts");
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
	char *nm;
	FILE *fp;

	/* Ignore all events except SR_EV_DONE */
	if (event != SR_EV_DONE)
		return SR_ERR_OK;

	fp = fopen("/etc/motd", "w");
	if (!fp)
		return SR_ERR_SYS;

	nm = srx_get_str(session, xpath);
	if (nm) {
		fprintf(fp, "%s\n", nm);
	} else {
		/* XXX: derive from global "options.h" or /usr/share/factory/ */
		fprintf(fp, "\033[1;90mNote:\033[0m ");
		fprintf(fp, "\033[0;90m use help, show, and setup commands to set up and diagnose the system\033[0m\n");
	}
	fclose(fp);

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
		if (!nm)
			goto err;
	}

	if (aug_get(confd->aug, "etc/hostname/hostname", &tmp) <= 0)
		goto err;

	current = strdup(tmp);
	if (!current)
		goto err;

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

	ERROR("Reload hostname-dependent services ...");
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
		goto err;
	}

	rc = sr_install_module(confd->conn, YANG_PATH_"/ietf-system@2014-08-06.yang", NULL, features);
	if (rc)
		goto err;
	/* Augment to ietf-systems */
	rc = sr_install_module(confd->conn, YANG_PATH_"/infix-system@2014-08-06.yang", NULL, NULL);
	if (rc)
		goto err;

	rc = sr_oper_get_subscribe(confd->session, "ietf-system", CLOCK_PATH_,
				   clock_cb, NULL, SR_SUBSCR_DEFAULT, &confd->sub);
	if (rc != SR_ERR_OK)
		goto err;

	rc = sr_oper_get_subscribe(confd->session, "ietf-system", PLATFORM_PATH_,
				   platform_cb, NULL, SR_SUBSCR_DEFAULT, &confd->sub);
	if (rc != SR_ERR_OK)
		goto err;

	REGISTER_CHANGE(confd->session, "ietf-system", "/ietf-system:system/hostname", 0, change_hostname, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-system", "/ietf-system:system/infix-system:motd", 0, change_motd, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-system", "/ietf-system:system/clock", 0, change_clock, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-system", "/ietf-system:system/ntp", 0, change_ntp, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-system", "/ietf-system:system/dns-resolver", 0, change_dns, confd, &confd->sub);

	REGISTER_RPC(confd->session, "/ietf-system:system-restart",  rpc_exec, "reboot", &confd->sub);
	REGISTER_RPC(confd->session, "/ietf-system:system-shutdown", rpc_exec, "poweroff", &confd->sub);
	REGISTER_RPC(confd->session, "/ietf-system:set-current-datetime", rpc_set_datetime, NULL, &confd->sub);

	return SR_ERR_OK;
err:
	ERROR("init failed: %s", sr_strerror(rc));
	sr_unsubscribe(confd->sub);

	return rc;
}
