/* Sysrepo plugin of ietf-system@2014-08-06.yang module */

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

#define CLOCK_PATH_    "/ietf-system:system-state/clock"
#define PLATFORM_PATH_ "/ietf-system:system-state/platform"

#define DEBUG(frmt, ...)
//#define DEBUG(frmt, ...) syslog(LOG_DEBUG, "%s: "frmt, __func__, ##__VA_ARGS__)
#define ERROR(frmt, ...) syslog(LOG_ERR, "%s: " frmt, __func__, ##__VA_ARGS__)
#define ERRNO(frmt, ...) syslog(LOG_ERR, "%s: " frmt ": %s", __func__, ##__VA_ARGS__, strerror(errno))

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

static int exec_rpc_cb(sr_session_ctx_t *session, uint32_t sub_id, const char *path,
		       const sr_val_t *input, const size_t input_cnt,
		       sr_event_t event, unsigned request_id,
		       sr_val_t **output, size_t *output_cnt,
		       void *priv)
{
	int rc;

	DEBUG("path: %s", path);

	rc = system(priv);
	if (WIFSIGNALED(rc) || (WIFEXITED(rc) && WEXITSTATUS(rc)))
		return SR_ERR_INTERNAL;

	return SR_ERR_OK;
}

/* '/ietf-system:set-current-date-time' */
static int set_datetime_rpc_cb(sr_session_ctx_t *session, uint32_t sub_id,
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

int hostname_change_cb(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		       const char *xpath, sr_event_t event,
		       unsigned request_id, void *priv)
{
	sr_val_t *old_val, *new_val, *val;
	sr_change_iter_t *iter;
	sr_change_oper_t op;
	int rc;

	if (event != SR_EV_ENABLED && event != SR_EV_DONE)
		return SR_ERR_OK;

	rc = sr_get_changes_iter(session, "//.", &iter);
	if (rc) {
		ERROR("failed to get changes iter: %s", sr_strerror(rc));
		return rc;
	}

	while (sr_get_change_next(session, iter, &op, &old_val, &new_val) == SR_ERR_OK) {
		val = new_val ? new_val : old_val;
		if (strcmp(val->xpath, "/ietf-system:system/hostname"))
			goto free_vals;

		switch (op) {
		case SR_OP_CREATED:
		case SR_OP_MODIFIED:
			if (sethostname(new_val->data.string_val,
					strlen(new_val->data.string_val))) {
				ERRNO("Failed to set hostname");
				return SR_ERR_SYS;
			}

			DEBUG("Set hostname to '%s'", new_val->data.string_val);
			break;
		}

	free_vals:
		sr_free_val(old_val);
		sr_free_val(new_val);
	}

	return SR_ERR_OK;
}

int sr_plugin_init_cb(sr_session_ctx_t *sess, void **priv)
{
	sr_subscription_ctx_t *sub = NULL;
	sr_conn_ctx_t *conn;
	int rc;

	openlog("sysrepo ietf-system plugin", LOG_USER, 0);
	conn = sr_session_get_connection(sess);
	sr_install_module(conn, YANG_PATH_"ietf-system.yang", NULL, NULL);

	rc = sr_oper_get_subscribe(sess, "ietf-system", CLOCK_PATH_,
				   clock_cb, NULL, SR_SUBSCR_DEFAULT, &sub);
	if (rc != SR_ERR_OK)
		goto err;

	rc = sr_oper_get_subscribe(sess, "ietf-system", PLATFORM_PATH_,
				   platform_cb, NULL, SR_SUBSCR_DEFAULT, &sub);
	if (rc != SR_ERR_OK)
		goto err;

	rc = sr_rpc_subscribe(sess, "/ietf-system:system-restart",
			      exec_rpc_cb, "reboot",
			      0, SR_SUBSCR_DEFAULT, &sub);
	if (rc != SR_ERR_OK)
		goto err;

	rc = sr_rpc_subscribe(sess, "/ietf-system:system-shutdown",
			      exec_rpc_cb, "poweroff", 0, SR_SUBSCR_DEFAULT, &sub);
	if (rc != SR_ERR_OK)
		goto err;

	rc = sr_rpc_subscribe(sess, "/ietf-system:set-current-datetime",
			      set_datetime_rpc_cb, NULL, 0, SR_SUBSCR_DEFAULT, &sub);
	if (rc != SR_ERR_OK)
		goto err;

	rc = sr_module_change_subscribe(sess, "ietf-system", "/ietf-system:system/hostname",
				        hostname_change_cb, NULL, 0,
					SR_SUBSCR_DEFAULT | SR_SUBSCR_ENABLED, &sub);
	if (rc != SR_ERR_OK) {
		ERROR("failed to subscribe to changes of hostname: %s",
		      sr_strerror(rc));
		goto err;
	}

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
