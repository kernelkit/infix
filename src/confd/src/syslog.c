/* SPDX-License-Identifier: BSD-3-Clause */

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "core.h"

#include <syslog/syslog.h>	/* sysklogd syslogp_r() API */

#define XPATH_BASE_       "/ietf-syslog:syslog"
#define XPATH_FILE_       XPATH_BASE_"/actions/file"
#define XPATH_LOG_FILE    XPATH_BASE_"/actions/file/log-file"
#define XPATH_REMOTE_     XPATH_BASE_"/actions/remote"
#define XPATH_REMOTE_DST  XPATH_BASE_"/actions/remote/destination"
#define XPATH_ROTATE_     XPATH_BASE_"/infix-syslog:file-rotation"
#define XPATH_SERVER_     XPATH_BASE_"/infix-syslog:server"

#define SYSLOG_D_         "/etc/syslog.d"
#define SYSLOG_FILE       SYSLOG_D_"/log-file-%s.conf"
#define SYSLOG_REMOTE     SYSLOG_D_"/remote-%s.conf"
#define SYSLOG_ROTATE     SYSLOG_D_"/rotate.conf"
#define SYSLOG_SERVER     SYSLOG_D_"/server.conf"

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

	if (!facility)
		return "";

	if (!strncmp(facility, "ietf-syslog:", 12))
		f = &facility[12];
	if (!strncmp(facility, "infix-syslog:", 13))
		f = &facility[13];

	if (!strcmp(f, "all"))
		return "*";
	if (!strcmp(f, "rauc"))
		return "local0";
	if (!strcmp(f, "container"))
		return "local1";
	if (!strcmp(f, "web"))
		return "local7";

	return f;
}

/* handle general syslog excpetions */
static const char *sxlate(const char *severity)
{
	if (!severity)
		return "";

	if (!strcmp(severity, "all"))
		return "*";
	if (!strcmp(severity, "emergency"))
		return "emerg";
	if (!strcmp(severity, "critical"))
		return "crit";
	/* sysklogd handles error -> err */

	return severity;
}

static size_t selector(sr_session_ctx_t *session, struct action *act)
{
	char xpath[strlen(act->xpath) + 64];
	bool has_pattern = false;
	sr_val_t *list = NULL;
	size_t count = 0;
	size_t num = 0;
	char *pattern;
	int rc;

	/* Check for hostname-filter (infix-syslog augment) */
	snprintf(xpath, sizeof(xpath), "%s/infix-syslog:hostname-filter", act->xpath);
	rc = sr_get_items(session, xpath, 0, 0, &list, &count);
	if (rc == SR_ERR_OK && count > 0) {
		fprintf(act->fp, "+");
		for (size_t i = 0; i < count; i++)
			fprintf(act->fp, "%s%s", i ? "," : "", list[i].data.string_val);
		fprintf(act->fp, "\n");

		sr_free_values(list, count);
		list = NULL;
		count = 0;
	}

	/* Check for property-filter (infix-syslog augment) */
	char *property = srx_get_str(session, "%s/infix-syslog:property-filter/property", act->xpath);
	if (property) {
		char *operator = srx_get_str(session, "%s/infix-syslog:property-filter/operator", act->xpath);
		char *value = srx_get_str(session, "%s/infix-syslog:property-filter/value", act->xpath);
		char *case_str = srx_get_str(session, "%s/infix-syslog:property-filter/case-insensitive", act->xpath);
		char *negate_str = srx_get_str(session, "%s/infix-syslog:property-filter/negate", act->xpath);

		if (operator && value) {
			/* Build operator prefix: [!][icase_]operator */
			char op_prefix[32] = "";

			/* Only apply negate if explicitly set to true */
			if (negate_str && !strcmp(negate_str, "true"))
				strlcat(op_prefix, "!", sizeof(op_prefix));

			/* Only apply icase_ if explicitly set to true */
			if (case_str && !strcmp(case_str, "true"))
				strlcat(op_prefix, "icase_", sizeof(op_prefix));

			strlcat(op_prefix, operator, sizeof(op_prefix));

			/* Property-based filter: :property, [!][icase_]operator, "value" */
			fprintf(act->fp, ":%s, %s, \"%s\"\n", property, op_prefix, value);
			has_pattern = true;
		}

		free(property);
		free(operator);
		free(value);
		free(case_str);
		free(negate_str);
	}

	/* Check for pattern-match (select-match feature) */
	pattern = srx_get_str(session, "%s/pattern-match", act->xpath);
	if (pattern) {
		/* Property-based filter: :msg, ereregex, "pattern" */
		fprintf(act->fp, ":msg, ereregex, \"%s\"\n", pattern);
		has_pattern = true;
		free(pattern);
	}

	snprintf(xpath, sizeof(xpath), "%s/facility-filter/facility-list", act->xpath);
	rc = sr_get_items(session, xpath, 0, 0, &list, &count);
	if (rc || !count) {
		if (has_pattern) {
			fprintf(act->fp, "*.*");
			return 1;
		}

		return 0;
	}

	for (size_t i = 0; i < count; ++i) {
		sr_val_t *entry = &list[i];
		char *facility, *severity, *compare, *action_str;
		const char *prefix = "";

		facility = srx_get_str(session, "%s/facility", entry->xpath);
		if (!facility)
			continue;
		severity = srx_get_str(session, "%s/severity", entry->xpath);
		if (!severity) {
			free(facility);
			continue;
		}

		/* Check for advanced-compare (select-adv-compare feature) */
		compare = srx_get_str(session, "%s/advanced-compare/compare", entry->xpath);
		action_str = srx_get_str(session, "%s/advanced-compare/action", entry->xpath);

		/*
		 * Handle compare + action combinations:
		 * - compare: equals -> use '=' after dot (facility.=severity)
		 * - compare: equals-or-higher (default) -> no prefix
		 * - action: block/stop -> use '!' after dot (facility.!severity)
		 * - action: log (default) -> no prefix modification
		 *
		 * Note: action block/stop takes precedence over compare equals
		 */
		/* First check compare */
		if (compare && strstr(compare, "equals") && !strstr(compare, "equals-or-higher")) {
			prefix = "=";
		}

		/* Then check action (can override compare) */
		if (action_str) {
			const char *a = action_str;
			if (strstr(a, "block") || strstr(a, "stop")) {
				prefix = "!";
			}
			free(action_str);
		}

		fprintf(act->fp, "%s%s.%s%s", i ? ";" : "", fxlate(facility), prefix, sxlate(severity));
		num++;

		free(facility);
		free(severity);
		free(compare);
	}

	sr_free_values(list, count);

	/* Return non-zero if we have either pattern or facilities */
	return has_pattern ? (num > 0 ? num : 1) : num;
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
	const char *fn;

	fn = filename(name, addr ? true : false, act.path, sizeof(act.path));
	act.fp = fopen(fn, "w");
	if (!act.fp) {
		ERRNO("Failed opening %s", fn);
		return;
	}

	if (!selector(session, &act)) {
		/* No selectors, must've been a delete operation after all. */
		fclose(act.fp);
		if (remove(act.path))
			ERRNO("failed removing %s", act.path);
		return;
	}

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

	fmt = srx_get_str(session, "%s/log-format", xpath);
	if (fmt) {
		char *ptr = strchr(fmt, ':'); /* skip any prefix */

		if (ptr)
			ptr = &ptr[1];
		else
			ptr = fmt;
		strlcat(opts, sep, sizeof(opts));
		strlcat(opts, ptr, sizeof(opts));
		sep = ",";
		free(fmt);
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

static int file_change(sr_session_ctx_t *session, struct lyd_node *config,struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	struct lyd_node *files, *file, *tree;
	int err;

	if (SR_EV_DONE != event || !lydx_get_xpathf(diff, XPATH_FILE_))
		return SR_ERR_OK;

	err = srx_get_diff(session, &tree);
	if (err)
		return SR_ERR_OK;

	files = lydx_get_descendant(tree, "syslog", "actions", "file", "log-file", NULL);
	LYX_LIST_FOR_EACH(files, file, "log-file") {
		struct lyd_node *node = lydx_get_child(file, "name");
		enum lydx_op op = lydx_get_op(node);
		char path[512] = XPATH_LOG_FILE;
		const char *name;

		name = getnm(node, path, sizeof(path));
		if (op == LYDX_OP_DELETE) {
			if (remove(filename(name, false, path, sizeof(path))))
				ERRNO("failed removing %s", path);
		} else {
			action(session, name, path, NULL);
		}
	}

	srx_free_changes(tree);
	finit_reload("sysklogd");

	return SR_ERR_OK;
}

static int remote_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	struct lyd_node *dremotes, *remote;

	if (SR_EV_DONE != event || !lydx_get_xpathf(diff, XPATH_REMOTE_))
		return SR_ERR_OK;

	dremotes = lydx_get_descendant(diff, "syslog", "actions", "remote", "destination", NULL);
	LYX_LIST_FOR_EACH(dremotes, remote, "destination") {
		struct lyd_node *node = lydx_get_child(remote, "name");
		enum lydx_op op = lydx_get_op(node);
		char path[512] = XPATH_REMOTE_DST;
		const char *name;

		name = getnm(node, path, sizeof(path));
		if (op == LYDX_OP_DELETE) {
			if (remove(filename(name, true, path, sizeof(path))))
				ERRNO("failed removing %s", path);
		} else {
			struct addr addr;

			addr.address = srx_get_str(session, "%s/udp/address", path);
			srx_get_int(session, &addr.port, SR_UINT16_T, "%s/udp/port", path);

			action(session, name, path, &addr);
		}
	}

	finit_reload("sysklogd");

	return SR_ERR_OK;
}

static int rotate_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	char path[512] = XPATH_ROTATE_;
	char *sz, *cnt;
	FILE *fp;

	if (SR_EV_DONE != event || !lydx_get_xpathf(diff, XPATH_ROTATE_))
		return SR_ERR_OK;

	fp = fopen(SYSLOG_ROTATE, "w");
	if (!fp) {
		ERRNO("Failed opening %s", SYSLOG_ROTATE);
		return SR_ERR_SYS;
	}

	sz  = srx_get_str(session, "%s/max-file-size", path);
	if (sz) {
		fprintf(fp, "rotate_size %sk\n", sz);
		free(sz);
	}

	cnt = srx_get_str(session, "%s/number-of-files", path);
	if (cnt) {
		fprintf(fp, "rotate_count %s\n", cnt);
		free(cnt);
	}

	fclose(fp);
	finit_reload("sysklogd");

	return SR_ERR_OK;
}

static int server_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	char path[512] = XPATH_SERVER_;
	sr_val_t *list = NULL;
	size_t count;
	FILE *fp;

	if (SR_EV_DONE != event || !lydx_get_xpathf(diff, XPATH_SERVER_))
		return SR_ERR_OK;

	if (!srx_enabled(session, "%s/enabled", path)) {
		if (erase(SYSLOG_SERVER))
			ERRNO("failed disabling syslog server");
		goto done;
	}

	fp = fopen(SYSLOG_SERVER, "w");
	if (!fp) {
		ERRNO("Failed opening %s", SYSLOG_SERVER);
		return SR_ERR_SYS;
	}

	/* Allow listening on port 514, or custom listen below */
	fprintf(fp, "secure_mode 0\n");

	if (!srx_get_items(session, &list, &count, "%s/listen/udp", path)) {
		for (size_t i = 0; i < count; ++i) {
			sr_val_t *entry = &list[i];
			char *address, *port;

			address = srx_get_str(session, "%s/address", entry->xpath);
			port = srx_get_str(session, "%s/port", entry->xpath);

			/* Accepted formats: address, :port, address:port */
			fprintf(fp, "listen %s%s%s\n", address ?: "", port ? ":" : "", port ?: "");
			free(address);
			free(port);
		}
	}
	if (list)
		sr_free_values(list, count);
	fclose(fp);
done:
	finit_reload("sysklogd");

	return SR_ERR_OK;
}

int syslog_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	int rc = SR_ERR_OK;

	if ((rc = file_change(session, config, diff, event, confd)))
		return rc;
	if ((rc = remote_change(session, config, diff, event, confd)))
		return rc;
	if ((rc = rotate_change(session, config, diff, event, confd)))
		return rc;
	if ((rc = server_change(session, config, diff, event, confd)))
		return rc;

	return SR_ERR_OK;
}

/*
 * RPC: /infix-syslog:log
 */

static int log_severity(const char *name)
{
	static const char *map[] = {
		"emergency", "alert", "critical", "error",
		"warning", "notice", "info", "debug",
	};

	if (!name)
		return LOG_NOTICE;

	for (size_t i = 0; i < NELEMS(map); i++) {
		if (!strcmp(map[i], name))
			return (int)i;
	}

	return LOG_NOTICE;
}

/*
 * The event session runs as confd, so its own user says nothing about the
 * caller.  Only the originator crosses over: netopeer2 pushes [nc-sid,
 * username], the CLI and the rpc tool set their originator name to the
 * user.  RESTCONF sets neither, then the tag is left at NILVALUE rather
 * than naming root, or confd, which libsyslog would fall back to.
 */
static const char *log_user(sr_session_ctx_t *session)
{
	const char *orig = sr_session_get_orig_name(session);
	const void *data;
	uint32_t size;

	if (orig && !strcmp(orig, "netopeer2")) {
		if (!sr_session_get_orig_data(session, 1, &size, &data) && size > 1)
			return data;
	} else if (orig && orig[0])
		return orig;

	return "-";
}

/* RFC 5424 PARAM-VALUE: escape '"', '\\', and ']', syslogd does not sanitize SD */
static char *sd_escape(char *ptr, const char *value)
{
	for (; *value; value++) {
		if (*value == '"' || *value == '\\' || *value == ']')
			*ptr++ = '\\';
		if ((unsigned char)*value < 0x20 || *value == 0x7f)
			*ptr++ = ' ';
		else
			*ptr++ = *value;
	}

	return ptr;
}

/* libsyslog splices MSGID into its printf format, see sysklogd vsyslogp_r() */
static char *msgid_escape(const char *msgid, char *buf)
{
	char *ptr = buf;

	for (; *msgid; msgid++) {
		if (*msgid == '%')
			*ptr++ = '%';
		*ptr++ = *msgid;
	}
	*ptr = 0;

	return buf;
}

/*
 * Bytes on the wire, libsyslog formats the whole packet in a 2048 byte
 * buffer and syslogd drops a message with truncated structured data.
 * RFC 5424 sec. 6: <PRI>1 TIMESTAMP HOSTNAME APP-NAME PROCID MSGID SD MSG
 */
#define SYSLOG_MAX_LEN 2048

static size_t log_len(const char *tag, const char *msgid, const char *sd, const char *msg)
{
	char host[256] = "-";

	gethostname(host, sizeof(host));
	host[sizeof(host) - 1] = 0;

	return 5 + 2 + 33 + strlen(host) + 1 + strlen(tag) + 1 + 11
		+ strlen(msgid ? msgid : "-") + 1 + strlen(sd ? sd : "-") + 1 + strlen(msg) + 1;
}

/* Render structured-data list as [id name="value" ...][id2 ...] */
static char *sd_build(const struct lyd_node *input)
{
	struct lyd_node *elem, *param;
	char *sd, *ptr;
	size_t len = 1;

	LYX_LIST_FOR_EACH(lyd_child(input), elem, "structured-data") {
		len += strlen(lydx_get_cattr(elem, "id")) + 2;
		LYX_LIST_FOR_EACH(lyd_child(elem), param, "param") {
			len += strlen(lydx_get_cattr(param, "name")) + 4;
			len += strlen(lydx_get_cattr(param, "value")) * 2;
		}
	}

	if (len == 1)
		return NULL;

	sd = ptr = malloc(len);
	if (!sd)
		return NULL;

	LYX_LIST_FOR_EACH(lyd_child(input), elem, "structured-data") {
		ptr += sprintf(ptr, "[%s", lydx_get_cattr(elem, "id"));
		LYX_LIST_FOR_EACH(lyd_child(elem), param, "param") {
			ptr += sprintf(ptr, " %s=\"", lydx_get_cattr(param, "name"));
			ptr  = sd_escape(ptr, lydx_get_cattr(param, "value"));
			*ptr++ = '"';
		}
		*ptr++ = ']';
	}
	*ptr = 0;

	return sd;
}

static int rpc_log(sr_session_ctx_t *session, uint32_t sub_id, const char *op_path,
		   const struct lyd_node *input, sr_event_t event, uint32_t request_id,
		   struct lyd_node *output, void *priv)
{
	struct syslog_data log = SYSLOG_DATA_INIT;
	struct lyd_node *in = (struct lyd_node *)input;
	const char *msg, *tag, *msgid;
	char *sd, *id = NULL;
	size_t len;
	int pri;

	msgid = lydx_get_cattr(in, "msgid");
	char idbuf[msgid ? strlen(msgid) * 2 + 1 : 1];

	msg = lydx_get_cattr(in, "message");
	if (!msg)
		return SR_ERR_INVAL_ARG;

	pri = LOG_USER | log_severity(lydx_get_cattr(in, "severity"));
	tag = lydx_get_cattr(in, "app-name");
	if (!tag)
		tag = log_user(session);
	if (msgid)
		id = msgid_escape(msgid, idbuf);

	sd = sd_build(in);
	len = log_len(tag, msgid, sd, msg);
	if (len > SYSLOG_MAX_LEN) {
		sr_session_set_error_message(session, "Log message too long, %zu bytes with header, max %d",
					     len, SYSLOG_MAX_LEN);
		free(sd);
		return SR_ERR_INVAL_ARG;
	}

	log.log_tag = tag;
	if (sd)
		syslogp_r(pri, &log, id, "%s", "%s", sd, msg);
	else
		syslogp_r(pri, &log, id, NULL, "%s", msg);
	closelog_r(&log);
	free(sd);

	return SR_ERR_OK;
}

int syslog_rpc_init(struct confd *confd)
{
	int rc;

	REGISTER_RPC_TREE(confd->session, "/infix-syslog:log", rpc_log, NULL, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
