/* SPDX-License-Identifier: BSD-3-Clause */

#include "core.h"
#include <sys/utsname.h>
#include <sys/sysinfo.h>

#define CLOCK_PATH_    "/ietf-system:system-state/clock"
#define PLATFORM_PATH_ "/ietf-system:system-state/platform"


/* Return seconds since boot */
static long get_uptime(void)
{
	struct sysinfo info;

	/*
	 * !!!Linux specific!!! Use '/var/run/utmp' BOOT record
	 * to be portable. But utmp file can be missing in some
	 * Unixes.
	 */
	sysinfo(&info);
	return info.uptime;
}

static int get_time_as_str(time_t *time, char *buf, int bufsz)
{
	int n;

	n = strftime(buf, bufsz, "%Y-%m-%dT%H:%M:%S%z",
		     localtime(time));
	if (!n)
		return -1;

	/* Buf ends with +hhmm but should be +hh:mm, fix this */
	memmove(buf + n - 1, buf + n - 2, 3);
	buf[n - 2] = ':';

	return 0;
}

static int clock_cb(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		    const char *path, const char *request_path, uint32_t request_id,
		    struct lyd_node **parent, void *priv)
{
	static char boottime[64] = { 0 };
	const struct ly_ctx *ctx;
	char curtime[64];
	char *buf;
	time_t t;
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

	if (!*boottime) {
		t = time(NULL) - get_uptime();
		get_time_as_str(&t, boottime, sizeof(boottime));
	}

	rc = lyd_new_path(*parent, NULL, CLOCK_PATH_ "/boot-datetime", boottime, 0, NULL);
	if (rc)
		goto fail;

	t = time(NULL);
	get_time_as_str(&t, curtime, sizeof(curtime));
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
	struct utsname data;
	char *buf;
	int rc;

	DEBUG("path=%s request_path=%s", path, request_path);
	ctx = sr_acquire_context(sr_session_get_connection(session));

	/* POSIX func */
	uname(&data);

	rc = lyd_new_path(NULL, ctx, PLATFORM_PATH_, NULL, 0, parent);
	if (rc) {
	fail:
		ERROR("Failed building data tree, libyang error %d", rc);
		return SR_ERR_INTERNAL;
	}

	lyd_print_mem(&buf, *parent, LYD_XML, 0);
	DEBUG("%s", buf);

	rc = lyd_new_path(*parent, NULL, PLATFORM_PATH_"/os-name", data.sysname, 0, NULL);
	if (rc)
		goto fail;
	rc = lyd_new_path(*parent, NULL, PLATFORM_PATH_"/os-release", data.release, 0, NULL);
	if (rc)
		goto fail;
	rc = lyd_new_path(*parent, NULL, PLATFORM_PATH_"/os-version", data.version, 0, NULL);
	if (rc)
		goto fail;
	rc = lyd_new_path(*parent, NULL, PLATFORM_PATH_"/machine", data.machine, 0, NULL);
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

	if (run(priv))
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
	struct timeval tv;
	struct tm tm;
	time_t t;

	memset(&tm, 0, sizeof(tm));

	/* Parse 'current-datetime'. */
	sscanf(input->data.string_val, "%d-%d-%dT%d:%d:%d", &tm.tm_year,
	       &tm.tm_mon, &tm.tm_mday, &tm.tm_hour, &tm.tm_min,
	       &tm.tm_sec);

	DEBUG("Setting datetime to '%d-%02d-%02d %02d:%02d:%02d'",
	      tm.tm_year, tm.tm_mon, tm.tm_mday, tm.tm_hour, tm.tm_min,
	      tm.tm_sec);

	tm.tm_year -= 1900;
	tm.tm_mon--;

	/*
	 * We suppose that this is a local time and ignore timezone.
	 */

	t = mktime(&tm);

	tv.tv_sec = t;
	tv.tv_usec = 0;
	if (settimeofday(&tv, NULL)) {
		ERRNO("settimeofday() failed");
		return SR_ERR_SYS;
	}

	return SR_ERR_OK;
}

static int sys_reload_services(void)
{
	return run("initctl -nbq touch sysklogd lldpd");
}

static void print_val(sr_val_t *val)
{
	char *str;

	if (sr_print_val_mem(&str, val))
		return;
	ERROR("%s", str);
	free(str);
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

char *sr_get_str(sr_session_ctx_t *session, const char *fmt, ...)
{
	char *str = NULL;
	sr_val_t *val;
	char *xpath;
	va_list ap;
	int len;

	va_start(ap, fmt);
	len = vsnprintf(NULL, 0, fmt, ap) + 1;
	va_end(ap);

	xpath = alloca(len);
	if (!xpath)
		return NULL;

	va_start(ap, fmt);
	vsnprintf(xpath, len, fmt, ap);
	va_end(ap);

	if (sr_get_item(session, xpath, 0, &val)) {
		ERROR("Failed reading string value from xpath %s", xpath);
		goto fail;
	}
	if (!val || val->type != SR_STRING_T)
		goto fail;

	str = strdup(val->data.string_val);
fail:
	if (val)
		sr_free_val(val);
	return str;
}

#define CHRONY_CONF "/etc/chrony.conf"
#define CHRONY_PREV CHRONY_CONF "-"
#define CHRONY_NEXT CHRONY_CONF "+"

static int change_ntp(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	struct confd *confd = (struct confd *)priv;
	const char *fn = CHRONY_NEXT;
	int valid = -1;
	sr_data_t *sdt;
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
			run("initctl -nbq disable chronyd");
			return SR_ERR_OK;
		}
		/*
		 * If chrony is alrady enabled we tell Finit it's been
		 * modified , so Finit restarts it, otherwise enable it.
		 */
		run("initctl -nbq touch chronyd");
		run("initctl -nbq enable chronyd");
		return SR_ERR_OK;
	}

	if (rc = sr_get_items(session, "/ietf-system:system/ntp/server", 0, 0, &val, &cnt)) {
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
		ptr = sr_get_str(session, "%s/udp/address", xpath);
		if (ptr) {
			type = sr_get_str(session, "%s/association-type", xpath);
			fprintf(fp, "%s %s", type ?: "server", ptr);
			server++;
			if (type)
				free(type);
			free(ptr);

			if (ptr = sr_get_str(session, "%s/udp/port", xpath)) {
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

static int change_hostname(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	struct confd *confd = (struct confd *)priv;
	const char *host, *tmp = NULL;
	char **hosts, *current;
	int err, i, nhosts;
	int rc = SR_ERR_SYS;
	char *nm;

	switch (event) {
	case SR_EV_ENABLED:	/* first time, on register. */
	case SR_EV_CHANGE:	/* regular change (copy cand running) */
		/* Wait for DONE to activate changes */
		return SR_ERR_OK;

	case SR_EV_ABORT:	/* User abort, or other plugin failed */
		return SR_ERR_OK;

	case SR_EV_DONE:
		/* Activate changes */
		break;
	}

	nm = sr_get_str(session, xpath);
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

	if (aug_load_file(confd->aug, "/etc/hostname") ||
	    aug_load_file(confd->aug, "/etc/hosts")) {
		ERROR("ietf-system: Augeas initialization failed");
		goto err;
	}

	if (rc = sr_install_module(confd->conn, YANG_PATH_"ietf-system@2014-08-06.yang", NULL, features))
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
	REGISTER_CHANGE(confd->session, "ietf-system", "/ietf-system:system/ntp", 0, change_ntp, confd, &confd->sub);

	REGISTER_RPC(confd->session, "/ietf-system:system-restart",  rpc_exec, "reboot", &confd->sub);
	REGISTER_RPC(confd->session, "/ietf-system:system-shutdown", rpc_exec, "poweroff", &confd->sub);
	REGISTER_RPC(confd->session, "/ietf-system:set-current-datetime", rpc_set_datetime, NULL, &confd->sub);

	return SR_ERR_OK;
err:
	ERROR("init failed: %s", sr_strerror(rc));
	sr_unsubscribe(confd->sub);

	return rc;
}
