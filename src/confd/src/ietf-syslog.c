/* SPDX-License-Identifier: BSD-3-Clause */

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "core.h"

#define XPATH_BASE_    "/ietf-syslog:syslog"
#define XPATH_FILE_    XPATH_BASE_"/actions/file/log-file/name"
#define XPATH_REMOTE_  XPATH_BASE_"/actions/remote/destination/name"
#define XPATH_ROTATE_  XPATH_BASE_"/infix-syslog:file-rotation"
#define XPATH_SERVER_  XPATH_BASE_"/infix-syslog:server"
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
	char opts[80] = "";
	char *sz, *cnt;

	act.fp = fopen(filename(name, addr ? true : false, act.path, sizeof(act.path)), "w");
	if (!act.fp) {
		ERRNO("Failed opening %s", act.path);
		return;
	}

	selector(session, &act);

	sz  = srx_get_str(session, "%s/file-rotation/max-file-size", xpath);
	cnt = srx_get_str(session, "%s/file-rotation/number-of-files", xpath);
	if (sz || cnt) {
		strlcat(opts, ";rotate=", sizeof(opts));
		if (sz) {
			strlcat(opts, sz, sizeof(opts));
			strlcat(opts, "k", sizeof(opts));
			free(sz);
		}
		if (cnt) {
			strlcat(opts, ":", sizeof(opts));
			strlcat(opts, cnt, sizeof(opts));
			free(cnt);
		}
	}

	/*
	 * The [] syntax is for IPv6, but the sysklogd parser handles
	 * them separately from the address conversion, so this works.
	 */
	if (addr)
		fprintf(act.fp, "\t@[%s]:%d\n", addr->address, addr->port);
	else if (name[0] == '/')
		fprintf(act.fp, "\t-%s\t%s\n", name, opts);
	else /* fall back to use system default log directory */
		fprintf(act.fp, "\t-/var/log/%s\t%s\n", name, opts);

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

		if (op == SR_OP_DELETED) {
			remove(filename(name, false, path, sizeof(path)));
		} else {
			xpath = basepath(val->xpath, path, sizeof(path));


			action(session, name, xpath, NULL);
		}

		sr_free_val(new);
		sr_free_val(old);
	}
	sr_free_change_iter(iter);
	systemf("initctl -nbq touch sysklogd");

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
	systemf("initctl -nbq touch sysklogd");

	return SR_ERR_OK;
}

static int rotate_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
			 const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	const char *path = "/etc/syslog.d/confd-rotate.conf";
	char *sz, *cnt;
	FILE *fp;

	if (SR_EV_DONE != event)
		return SR_ERR_OK;

	fp = fopen(path, "w");
	if (!fp) {
		ERRNO("Failed opening %s", path);
		return SR_ERR_SYS;
	}

	sz  = srx_get_str(session, "%s/max-file-size", xpath);
	if (sz) {
		fprintf(fp, "rotate_size %sk\n", sz);
		free(sz);
	}

	cnt = srx_get_str(session, "%s/number-of-files", xpath);
	if (cnt) {
		fprintf(fp, "rotate_count %s\n", cnt);
		free(cnt);
	}

	fclose(fp);
	systemf("initctl -nbq touch sysklogd");

	return SR_ERR_OK;
}

static int server_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
			 const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	const char *path = "/etc/syslog.d/confd-server.conf";
	sr_val_t *list = NULL;
	size_t count;
	FILE *fp;

	if (SR_EV_DONE != event)
		return SR_ERR_OK;

	if (!srx_enabled(session, "%s/enabled", xpath)) {
		remove(path);
		goto done;
	}

	fp = fopen(path, "w");
	if (!fp) {
		ERRNO("Failed opening %s", path);
		return SR_ERR_SYS;
	}

	/* Allow listening on port 514, or custom listen below */
	fprintf(fp, "secure_mode 0\n");

	if (!srx_get_items(session, &list, &count, "%s/listen/udp", xpath)) {
		for (size_t i = 0; i < count; ++i) {
			sr_val_t *entry = &list[i];
			char *address, *port;

			address = srx_get_str(session, "%s/address", entry->xpath);
			port = srx_get_str(session, "%s/port", entry->xpath);

			/* Accepted formats: address, :port, address:port */
			fprintf(fp, "listen %s%s%s\n", address ?: "", port ? ":" : "", port ?: "");
		}
	}

	fclose(fp);
done:
	systemf("initctl -nbq touch sysklogd");

	return SR_ERR_OK;
}

int ietf_syslog_init(struct confd *confd)
{
	int rc = SR_ERR_SYS;

	if (!confd)
		goto fail;

	REGISTER_CHANGE(confd->session, "ietf-syslog", XPATH_FILE_, 0, file_change, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-syslog", XPATH_REMOTE_, 0, remote_change, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-syslog", XPATH_ROTATE_, 0, rotate_change, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-syslog", XPATH_SERVER_, 0, server_change, confd, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("%s failed: %s", __func__, sr_strerror(rc));
	return rc;
}
