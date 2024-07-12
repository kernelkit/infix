/* SPDX-License-Identifier: BSD-3-Clause */

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "core.h"

#define XPATH_BASE_    "/ietf-syslog:syslog/actions"
#define XPATH_FILE_    XPATH_BASE_"/file/log-file/name"
#define SYSLOG_D_      "/etc/syslog.d"

static char *basepath(char *xpath, char *path, size_t len)
{
	char *ptr;

	strlcpy(path, xpath, len);
	ptr = strrchr(path, '/');
	if (ptr)
		*ptr = 0;

	return path;
}

static char *filename(char *name, char *path, size_t len)
{
	char *ptr, *n;

	ptr = strrchr(name, '/');
	if (ptr)
		n = ++ptr;
	else
		n = name;

	snprintf(path, len, "/etc/syslog.d/confd-%s.conf", n);

	return path;
}

/* handle sysklogd/BSD excpetions */
static const char *fxlate(const char *facility)
{
	const char *f = facility;

	if (!strncmp(facility, "ietf-syslog:", 12))
		f = &facility[12];

	if (!strcmp(f, "all"))
		return "*";
	if (!strcmp(f, "audit"))
		return "security";
	if (!strcmp(f, "cron2"))
		return "cron_sol";

	return f;
}

/* handle general syslog excpetions */
static const char *sxlate(const char *severity)
{
	if (!strcmp(severity, "all"))
		return "*";
	if (!strcmp(severity, "emergency"))
		return "emerg";
	if (!strcmp(severity, "critical"))
		return "crit";
	/* sysklogd handles error -> err */

	return severity;
}

static void handle_selector(sr_session_ctx_t *session, char *name, const char *xpath)
{
    sr_val_t *tuples = NULL;
    size_t count = 0;
    char path[512];
    FILE *fp;
    int rc;

    snprintf(path, sizeof(path), "%s/facility-filter/facility-list", xpath);
    rc = sr_get_items(session, path, 0, 0, &tuples, &count);
    if (rc != SR_ERR_OK) {
	    ERROR("Cannot find facility-list for syslog file:%s: %s\n", name, sr_strerror(rc));
	    return;
    }

    fp = fopen(filename(name, path, sizeof(path)), "w");
    if (!fp) {
	    ERRNO("Failed opening %s", path);
	    goto done;
    }

    for (size_t i = 0; i < count; ++i) {
        sr_val_t *tuple = &tuples[i];
	char *facility, *severity;

	facility = srx_get_str(session, "%s/facility", tuple->xpath);
	severity = srx_get_str(session, "%s/severity", tuple->xpath);

	fprintf(fp, "%s%s.%s", i ? ";" : "", fxlate(facility), sxlate(severity));

	free(facility);
	free(severity);
    }
    if (name[0] == '/')
	    fprintf(fp, "\t-%s\n", name);
    else
	    fprintf(fp, "\t-/var/log/%s\n", name);

    fclose(fp);
done:
    sr_free_values(tuples, count);
}

static int file_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		       const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	sr_change_iter_t *iter;
	sr_change_oper_t op;
	sr_val_t *old, *new;
	sr_error_t err;

	if (SR_EV_DONE != event)
		return SR_ERR_OK;

	err = sr_get_changes_iter(session, xpath, &iter);
	if (err)
		return SR_ERR_OK;

	while (sr_get_change_next(session, iter, &op, &old, &new) == SR_ERR_OK) {
		sr_val_t *val = new ? new : old;
		char path[512];
		char *name;

		if (strncmp(val->data.string_val, "file:", 5))
			continue;
		name = &val->data.string_val[5];

		if (op == SR_OP_DELETED)
			remove(filename(name, path, sizeof(path)));
		else
			handle_selector(session, name, basepath(val->xpath, path, sizeof(path)));

		sr_free_val(new);
		sr_free_val(old);
	}
	sr_free_change_iter(iter);
	systemf("initctl -nbq touch syslogd");

	return SR_ERR_OK;
}

int ietf_syslog_init(struct confd *confd)
{
	int rc = SR_ERR_SYS;

	if (!confd)
		goto fail;

	REGISTER_CHANGE(confd->session, "ietf-syslog", XPATH_FILE_, 0, file_change, confd, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("%s failed: %s", __func__, sr_strerror(rc));
	return rc;
}
