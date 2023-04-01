/* SPDX-License-Identifier: BSD-3-Clause */

#include <augeas.h>
#include <stdio.h>
#include <syslog.h>
#include <stdlib.h>
#include <string.h>
#include <sys/utsname.h>
#include <time.h>
#include <sys/time.h>
#include <errno.h>
#include <unistd.h>

/* Linux specific */
#include <sys/sysinfo.h>

#include <libyang/libyang.h>
#include <sysrepo.h>
#include <sysrepo/values.h>
#include <sysrepo/xpath.h>

#define CLOCK_PATH_    "/ietf-system:system-state/clock"
#define PLATFORM_PATH_ "/ietf-system:system-state/platform"


#define NELEMS(arr) (sizeof(arr) / sizeof(arr[0]))

#define DEBUG(frmt, ...)
//#define DEBUG(frmt, ...) syslog(LOG_DEBUG, "%s: "frmt, __func__, ##__VA_ARGS__)
#define ERROR(frmt, ...) syslog(LOG_ERR, "%s: " frmt, __func__, ##__VA_ARGS__)
#define ERRNO(frmt, ...) syslog(LOG_ERR, "%s: " frmt ": %s", __func__, ##__VA_ARGS__, strerror(errno))

static augeas *aug;


/**
 * Like system(), but takes a formatted string as argument.
 * @param fmt  printf style format list to command to run
 *
 * This system() wrapper greatly simplifies operations that usually
 * consist of composing a command from parts into a dynamic buffer
 * before calling it.  The return value from system() is also parsed,
 * checking for proper exit and signals.
 *
 * @returns If the command exits normally, the return code of the command
 * is returned.  Otherwise, if the command is signalled, the return code
 * is -1 and @a errno is set to @c EINTR.
 */
static int run(const char *fmt, ...)
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
	const char *host, *nm, *tmp;
	char **hosts, *current;
	int err, i, nhosts;
	int rc;

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
		ERROR("No valid hostname in current session");
		nm = "infix";	/* XXX: derive from global "options.h" */
	}
	ERROR("Got hostname %s", nm);

	aug_get(aug, "etc/hostname/hostname", &tmp);
	current = strdup(tmp);
	ERROR("Current hostname %s", current);

	err = sethostname(nm, strlen(nm));
	err = err ? : aug_set(aug, "etc/hostname/hostname", nm);

	nhosts = aug_match(aug, "etc/hosts/*/canonical", &hosts);
	for (i = 0; i < nhosts; i++) {
		aug_get(aug, hosts[i], &host);
		if (!strcmp(host, current))
			err = err ? : aug_set(aug, hosts[i], nm);
		free(hosts[i]);
	}
	free(hosts);
	free(current);

	if (err) {
		ERROR("Failed activating changes.");
		return err;
	}

	ERROR("Reload hostname-dependent services ...");
	err = err ? : aug_save(aug);
	if (sys_reload_services())
		return SR_ERR_SYS;

	return SR_ERR_OK;
}

static int register_change(sr_session_ctx_t *session, const char *xpath,
			   sr_module_change_cb cb, void *arg, sr_subscription_ctx_t **sub)
{
	int rc = sr_module_change_subscribe(session, "ietf-system", xpath, cb, arg, 0,
					    SR_SUBSCR_DEFAULT | SR_SUBSCR_ENABLED, sub);
	if (rc)
		ERROR("failed subscribing to changes of %s: %s", xpath, sr_strerror(rc));
	return rc;
}

static int register_rpc(sr_session_ctx_t *session, const char *xpath,
			sr_rpc_cb cb, void *arg, sr_subscription_ctx_t **sub)
{
	int rc = sr_rpc_subscribe(session, xpath, cb, arg, 0, SR_SUBSCR_DEFAULT, sub);
	if (rc)
		ERROR("failed subscribing to %s rpc: %s", xpath, sr_strerror(rc));
	return rc;
}

#define REGISTER_CHANGE(s,x,c,a,u) \
	if (rc = register_change(s, x, c, a, u))\
		goto err

#define REGISTER_RPC(s,x,c,a,u) \
	if (rc = register_rpc(s, x, c, a, u))	\
		goto err

int sr_plugin_init_cb(sr_session_ctx_t *session, void **priv)
{
	sr_subscription_ctx_t *sub = NULL;
	sr_conn_ctx_t *conn;
	const char *features[] = {
		"ntp",
		"ntp-udp-port",
		"timezone-name",
		NULL
	};
	int rc;

	aug = aug_init(NULL, "", 0);
	if (!aug ||
	    aug_load_file(aug, "/etc/hostname") ||
	    aug_load_file(aug, "/etc/hosts")) {
		ERROR("ietf-system: Augeas initialization failed");
		goto err;
	}

	openlog("ietf-system", LOG_USER, 0);
	conn = sr_session_get_connection(session);
	sr_install_module(conn, YANG_PATH_"ietf-system@2014-08-06.yang", NULL, features);

	rc = sr_oper_get_subscribe(session, "ietf-system", CLOCK_PATH_,
				   clock_cb, NULL, SR_SUBSCR_DEFAULT, &sub);
	if (rc != SR_ERR_OK)
		goto err;

	rc = sr_oper_get_subscribe(session, "ietf-system", PLATFORM_PATH_,
				   platform_cb, NULL, SR_SUBSCR_DEFAULT, &sub);
	if (rc != SR_ERR_OK)
		goto err;

	REGISTER_RPC(session, "/ietf-system:system-restart",  rpc_exec, "reboot", &sub);
	REGISTER_RPC(session, "/ietf-system:system-shutdown", rpc_exec, "poweroff", &sub);
	REGISTER_RPC(session, "/ietf-system:set-current-datetime", rpc_set_datetime, NULL, &sub);

	REGISTER_CHANGE(session, "/ietf-system:system/hostname", change_hostname, NULL, &sub);
	REGISTER_CHANGE(session, "/ietf-system:system/ntp", change_ntp, NULL, &sub);

	*(sr_subscription_ctx_t **)priv = sub;
	DEBUG("init ok");

	return SR_ERR_OK;

err:
	ERROR("init failed: %s", sr_strerror(rc));
	sr_unsubscribe(sub);

	return rc;
}

void sr_plugin_cleanup_cb(sr_session_ctx_t *session, void *priv)
{
        sr_unsubscribe((sr_subscription_ctx_t *)priv);

        DEBUG("cleanup ok");
}
