/* SPDX-License-Identifier: BSD-3-Clause */

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "core.h"

#define XPATH_BASE_    "/ietf-syslog:syslog/actions"
#define XPATH_FILE_    XPATH_BASE_"/file/log-file/name"
#define XPATH_REMOTE_  XPATH_BASE_"/remote/destination/name"
#define SYSLOG_D_      "/etc/syslog.d"

struct addr {
	char *address;
	int   port;
};

struct action {
	const char  *name;
	const char  *xpath;

	char         path[512];
	FILE        *fp;

	struct addr *addr;
};


static char *basepath(char *xpath, char *path, size_t len)
{
	char *ptr;

	strlcpy(path, xpath, len);
	ptr = strrchr(path, '/');
	if (ptr)
		*ptr = 0;

	return path;
}

static char *filename(const char *name, bool remote, char *path, size_t len)
{
	const char *n;
	char *ptr;

	ptr = strrchr(name, '/');
	if (ptr)
		n = ++ptr;
	else
		n = name;

	snprintf(path, len, "/etc/syslog.d/confd-%s%s.conf", remote ? "remote-" : "", n);

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

static void selector(sr_session_ctx_t *session, struct action *act)
{
	sr_val_t *list = NULL;
	size_t count = 0;
	int rc;

	snprintf(act->path, sizeof(act->path), "%s/facility-filter/facility-list", act->xpath);
	rc = sr_get_items(session, act->path, 0, 0, &list, &count);
	if (rc != SR_ERR_OK) {
		ERROR("Cannot find facility-list for syslog file:%s: %s\n", act->name, sr_strerror(rc));
		return;
	}

	for (size_t i = 0; i < count; ++i) {
		sr_val_t *entry = &list[i];
		char *facility, *severity;

		facility = srx_get_str(session, "%s/facility", entry->xpath);
		severity = srx_get_str(session, "%s/severity", entry->xpath);

		fprintf(act->fp, "%s%s.%s", i ? ";" : "", fxlate(facility), sxlate(severity));

		free(facility);
		free(severity);
	}

	sr_free_values(list, count);
}

static void action(sr_session_ctx_t *session, const char *name, const char *xpath, struct addr *addr)
{
	struct action act = {
		.name  = name,
		.xpath = xpath,
		.addr  = addr,
	};

	act.fp = fopen(filename(name, addr ? true : false, act.path, sizeof(act.path)), "w");
	if (!act.fp) {
		ERRNO("Failed opening %s", act.path);
		return;
	}

	selector(session, &act);

	/*
	 * The [] syntax is for IPv6, but the sysklogd parser handles
	 * them separately from the address conversion, so this works.
	 */
	if (addr)
		fprintf(act.fp, "\t@[%s]:%d\n", addr->address, addr->port);
	else if (name[0] == '/')
		fprintf(act.fp, "\t-%s\n", name);
	else /* fall back to use system default log directory */
		fprintf(act.fp, "\t-/var/log/%s\n", name);

	fclose(act.fp);
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
			remove(filename(name, false, path, sizeof(path)));
		else
			action(session, name, basepath(val->xpath, path, sizeof(path)), NULL);

		sr_free_val(new);
		sr_free_val(old);
	}
	sr_free_change_iter(iter);
	systemf("initctl -nbq touch syslogd");

	return SR_ERR_OK;
}

static int remote_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
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

		name = val->data.string_val;
		if (op == SR_OP_DELETED) {
			remove(filename(name, true, path, sizeof(path)));
		} else {
			struct addr addr;

			xpath = basepath(val->xpath, path, sizeof(path));
			addr.address = srx_get_str(session, "%s/udp/address", xpath);
			srx_get_int(session, &addr.port, SR_UINT16_T, "%s/udp/port", xpath);

			action(session, name, xpath, &addr);
		}

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
	REGISTER_CHANGE(confd->session, "ietf-syslog", XPATH_REMOTE_, 0, remote_change, confd, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("%s failed: %s", __func__, sr_strerror(rc));
	return rc;
}
