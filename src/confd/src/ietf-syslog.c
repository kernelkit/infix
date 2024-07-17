/* SPDX-License-Identifier: BSD-3-Clause */

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "core.h"

#define XPATH_BASE_    "/ietf-syslog:syslog"
#define XPATH_FILE_    XPATH_BASE_"/actions/file/log-file"
#define XPATH_REMOTE_  XPATH_BASE_"/actions/remote/destination"
#define XPATH_ROTATE_  XPATH_BASE_"/infix-syslog:file-rotation"
#define XPATH_SERVER_  XPATH_BASE_"/infix-syslog:server"

#define SYSLOG_D_      "/etc/syslog.d"
#define SYSLOG_FILE    SYSLOG_D_"/log-file-%s.conf"
#define SYSLOG_REMOTE  SYSLOG_D_"/remote-%s.conf"
#define SYSLOG_ROTATE  SYSLOG_D_"/rotate.conf"
#define SYSLOG_SERVER  SYSLOG_D_"/server.conf"

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


static char *filename(const char *name, bool remote, char *path, size_t len)
{
	const char *n;
	char *ptr;

	ptr = strrchr(name, '/');
	if (ptr)
		n = ++ptr;
	else
		n = name;

	snprintf(path, len, remote ? SYSLOG_REMOTE : SYSLOG_FILE, n);

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
	char *sz, *cnt, *fmt;
	char opts[80] = "\t";
	char *sep = ";";

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

		sep = ",";
	}

	fmt = srx_get_str(session, "%s/format", xpath);
	if (fmt) {
		char *ptr = strchr(fmt, ':'); /* skip any prefix */

		if (ptr)
			fmt = &ptr[1];
		strlcat(opts, sep, sizeof(opts));
		strlcat(opts, fmt, sizeof(opts));
		sep = ",";
	}

	/*
	 * The [] syntax is for IPv6, but the sysklogd parser handles
	 * them separately from the address conversion, so this works.
	 */
	if (addr)
		fprintf(act.fp, "\t@[%s]:%d%s\n", addr->address, addr->port, opts);
	else if (name[0] == '/')
		fprintf(act.fp, "\t-%s%s\n", name, opts);
	else /* fall back to use system default log directory */
		fprintf(act.fp, "\t-/var/log/%s%s\n", name, opts);

	fclose(act.fp);
}

/* Read 'name' node, then construct XPath for next operation. */
static const char *getnm(struct lyd_node *node, char *xpath, size_t len)
{
	const char *name = lyd_get_value(node);

	strlcat(xpath, "[name='", len);
	strlcat(xpath, name, len);
	strlcat(xpath, "']", len);

	if (!strncmp(name, "file:", 5))
		name += 5;

	return name;
}

static int file_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		       const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	struct lyd_node *files, *file;
	int err;

	if (SR_EV_DONE != event)
		return SR_ERR_OK;

	err = srx_get_changes(session, XPATH_FILE_, &files);
	if (err)
		return SR_ERR_OK;

	LYX_LIST_FOR_EACH(files, file, "log-file") {
		struct lyd_node *node = lydx_get_child(file, "name");
		enum lydx_op op = lydx_get_op(node);
		char path[512] = XPATH_FILE_;
		const char *name;

		name = getnm(node, path, sizeof(path));
		if (op == LYDX_OP_DELETE)
			remove(filename(name, false, path, sizeof(path)));
		else
			action(session, name, path, NULL);
	}

	srx_free_changes(files);
	systemf("initctl -nbq touch sysklogd");

	return SR_ERR_OK;
}

static int remote_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
			 const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	struct lyd_node *remotes, *remote;
	int err;

	if (SR_EV_DONE != event)
		return SR_ERR_OK;

	err = srx_get_changes(session, XPATH_REMOTE_, &remotes);
	if (err)
		return SR_ERR_OK;

	LYX_LIST_FOR_EACH(remotes, remote, "destination") {
		struct lyd_node *node = lydx_get_child(remote, "name");
		enum lydx_op op = lydx_get_op(node);
		char path[512] = XPATH_REMOTE_;
		const char *name;

		name = getnm(node, path, sizeof(path));
		if (op == LYDX_OP_DELETE) {
			remove(filename(name, true, path, sizeof(path)));
		} else {
			struct addr addr;

			addr.address = srx_get_str(session, "%s/udp/address", path);
			srx_get_int(session, &addr.port, SR_UINT16_T, "%s/udp/port", path);

			action(session, name, xpath, &addr);
		}
	}

	srx_free_changes(remotes);
	systemf("initctl -nbq touch sysklogd");

	return SR_ERR_OK;
}

static int rotate_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
			 const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	char *sz, *cnt;
	FILE *fp;

	if (SR_EV_DONE != event)
		return SR_ERR_OK;

	fp = fopen(SYSLOG_ROTATE, "w");
	if (!fp) {
		ERRNO("Failed opening %s", SYSLOG_ROTATE);
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
	sr_val_t *list = NULL;
	size_t count;
	FILE *fp;

	if (SR_EV_DONE != event)
		return SR_ERR_OK;

	if (!srx_enabled(session, "%s/enabled", xpath)) {
		remove(SYSLOG_SERVER);
		goto done;
	}

	fp = fopen(SYSLOG_SERVER, "w");
	if (!fp) {
		ERRNO("Failed opening %s", SYSLOG_SERVER);
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

	REGISTER_CHANGE(confd->session, "ietf-syslog", XPATH_FILE_"//.", 0, file_change, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-syslog", XPATH_REMOTE_"//.", 0, remote_change, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-syslog", XPATH_ROTATE_"//.", 0, rotate_change, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ietf-syslog", XPATH_SERVER_"//.", 0, server_change, confd, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("%s failed: %s", __func__, sr_strerror(rc));
	return rc;
}
