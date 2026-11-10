/* SPDX-License-Identifier: BSD-3-Clause */
/*
 * Alarm operations.  The alarm list lives in /run/alarm as files, one
 * per alarm, kept by yangerd: active/<type>/<name> is a raised alarm,
 * entry/<type>/<name> is what yangerd knows about it, state/<type>/<name>
 * is the operator's history.  confd works on the same files: it renders
 * the shelves and severity assignments for yangerd, appends operator
 * state, removes entries when asked to purge, and raises and clears the
 * external alarms a client may own.
 */
#include <ctype.h>
#include <errno.h>
#include <glob.h>
#include <limits.h>
#include <stdio.h>
#include <string.h>
#include <sys/stat.h>
#include <time.h>
#include <unistd.h>
#include <jansson.h>
#include <srx/common.h>
#include <srx/helpers.h>
#include <srx/lyx.h>

#include "core.h"

#define ALARM_DIR    "/run/alarm"
#define ALARM_CONFIG ALARM_DIR "/config.json"
#define ALARM_XPATH  "/ietf-alarms:alarms"
#define TYPE_MODULE  "infix-alarms:"
#define EXTERNAL     "external"
#define NAME_MAX_LEN 160

/* The identity name is the directory name, only this module's types exist */
static const char *type_dir(const char *id)
{
	if (!id || strncmp(id, TYPE_MODULE, strlen(TYPE_MODULE)))
		return NULL;
	return id + strlen(TYPE_MODULE);
}

/* What yangerd accepts as a resource or qualifier, i.e. a safe file name */
static int valid_name(const char *s)
{
	if (!s || !*s)
		return 0;
	for (; *s; s++) {
		if (!isalnum((unsigned char)*s) && !strchr("._:-", *s))
			return 0;
	}
	return 1;
}

/*
 * The short name of a resource: the last key value of an instance
 * identifier, /ietf-interfaces:interfaces/interface[name='eth0'] is
 * eth0, any other resource as is.  Markers are named by it.
 */
static const char *resource_name(const char *resource, char *buf, size_t len)
{
	const char *start = NULL, *end, *p;

	if (!resource || *resource != '/')
		return resource;
	for (p = resource; (p = strstr(p, "='")); p += 2)
		start = p + 2;
	if (!start)
		return resource;
	end = strchr(start, '\'');
	if (!end)
		return resource;
	snprintf(buf, len, "%.*s", (int)(end - start), start);

	return buf;
}

static void marker_name(char *buf, size_t len, const char *resource, const char *qualifier)
{
	char name[NAME_MAX_LEN];

	resource = resource_name(resource, name, sizeof(name));
	if (qualifier && *qualifier)
		snprintf(buf, len, "%s@%s", resource, qualifier);
	else
		snprintf(buf, len, "%s", resource);
}

static const char *now_stamp(char *buf, size_t len)
{
	time_t now = time(NULL);
	struct tm tm;

	gmtime_r(&now, &tm);
	strftime(buf, len, "%Y-%m-%dT%H:%M:%SZ", &tm);
	return buf;
}

static time_t parse_stamp(const char *s)
{
	struct tm tm = { 0 };

	if (!s || !strptime(s, "%Y-%m-%dT%H:%M:%S", &tm))
		return 0;
	return timegm(&tm);
}

/* Write obj to path through a temp file, yangerd never sees half of it */
static int save_json(const char *path, json_t *obj)
{
	char tmp[PATH_MAX];

	snprintf(tmp, sizeof(tmp), "%s.tmp", path);
	if (json_dump_file(obj, tmp, JSON_COMPACT)) {
		ERRNO("failed writing %s", tmp);
		return -1;
	}
	if (rename(tmp, path)) {
		ERRNO("failed renaming %s", tmp);
		remove(tmp);
		return -1;
	}
	return 0;
}

static int severity_rank(const char *s)
{
	const char *levels[] = { "indeterminate", "warning", "minor", "major", "critical" };

	for (size_t i = 0; i < NELEMS(levels); i++) {
		if (s && !strcmp(s, levels[i]))
			return (int)i;
	}
	return -1;
}

/* The caller's name for the operator history, as far as the transport tells */
static const char *operator_name(sr_session_ctx_t *session)
{
	const char *via, *user = rpc_user(session, &via);

	return user ?: via;
}

/*
 * confd renders the running configuration as the JSON encoding of the
 * container.  yangerd parses that for the shelves and the severity
 * assignments, so there is no second format to keep in step.
 */
int alarm_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff,
		 sr_event_t event, struct confd *confd)
{
	struct lyd_node *alarms;
	FILE *fp;

	if (event != SR_EV_DONE || !lydx_get_xpathf(diff, ALARM_XPATH))
		return SR_ERR_OK;

	alarms = lydx_get_xpathf(config, ALARM_XPATH);
	if (!alarms) {
		remove(ALARM_CONFIG);
		return SR_ERR_OK;
	}

	fp = fopen(ALARM_CONFIG ".tmp", "w");
	if (!fp) {
		ERRNO("failed creating %s", ALARM_CONFIG);
		return SR_ERR_SYS;
	}
	lyd_print_file(fp, alarms, LYD_JSON, 0);
	fclose(fp);
	if (rename(ALARM_CONFIG ".tmp", ALARM_CONFIG)) {
		ERRNO("failed renaming %s", ALARM_CONFIG ".tmp");
		return SR_ERR_SYS;
	}

	return SR_ERR_OK;
}

/* Append an operator state change to the history of one alarm */
static int record_operator_state(sr_session_ctx_t *session, const char *typ, const char *name,
				 const char *state, const char *text)
{
	char path[PATH_MAX], stamp[32];
	json_t *history, *change;

	snprintf(path, sizeof(path), ALARM_DIR "/state/%s", typ);
	if (mkdir(path, 0755) && errno != EEXIST) {
		ERRNO("failed creating %s", path);
		return -1;
	}
	snprintf(path, sizeof(path), ALARM_DIR "/state/%s/%s", typ, name);

	history = json_load_file(path, 0, NULL);
	if (!history || !json_is_array(history)) {
		json_decref(history);
		history = json_array();
	}

	change = json_pack("{s:s, s:s, s:s}", "time", now_stamp(stamp, sizeof(stamp)),
			   "operator", operator_name(session), "state", state);
	if (text && *text)
		json_object_set_new(change, "text", json_string(text));
	json_array_append_new(history, change);

	AUDIT("Alarm %s %s %s by \"%s\".", typ, name, state, operator_name(session));
	if (save_json(path, history)) {
		json_decref(history);
		return -1;
	}
	json_decref(history);

	return 0;
}

static int set_operator_state(sr_session_ctx_t *session, uint32_t sub_id, const char *op_path,
			      const struct lyd_node *input, sr_event_t event, uint32_t request_id,
			      struct lyd_node *output, void *priv)
{
	const char *resource, *qualifier, *typ, *state, *text;
	char name[NAME_MAX_LEN], short_name[NAME_MAX_LEN], path[PATH_MAX];
	struct lyd_node *in = (struct lyd_node *)input;
	struct lyd_node *entry = lyd_parent(in);
	struct stat st;

	if (event != SR_EV_RPC)
		return SR_ERR_OK;

	resource = lydx_get_cattr(entry, "resource");
	qualifier = lydx_get_cattr(entry, "alarm-type-qualifier");
	typ = type_dir(lydx_get_cattr(entry, "alarm-type-id"));
	resource = resource_name(resource, short_name, sizeof(short_name));
	if (!typ || !valid_name(resource) || (qualifier && *qualifier && !valid_name(qualifier)))
		return rpc_failed(session, "No such alarm");

	marker_name(name, sizeof(name), resource, qualifier);
	snprintf(path, sizeof(path), ALARM_DIR "/entry/%s/%s", typ, name);
	if (stat(path, &st))
		return rpc_failed(session, "No such alarm");

	state = lydx_get_cattr(in, "state") ?: "none";
	text = lydx_get_cattr(in, "text");
	if (record_operator_state(session, typ, name, state, text))
		return rpc_failed(session, "Failed recording operator state");

	return SR_ERR_OK;
}

/* Filter from an action's input, RFC 8632 filter-input grouping */
struct alarm_filter {
	const char *clearance;	/* any, cleared, not-cleared */
	time_t      older_than;	/* 0 when unset */
	const char *sev_op;	/* below, is, above, or NULL */
	int         sev_rank;
	const char *op_state;	/* NULL when unset */
	const char *op_user;	/* NULL when unset */
	int         shelved;	/* purge-shelved-alarms */
};

static void alarm_filter_init(struct alarm_filter *f, struct lyd_node *in, int shelved)
{
	struct lyd_node *node;
	const char *val;

	memset(f, 0, sizeof(*f));
	f->shelved = shelved;
	f->clearance = lydx_get_cattr(in, "alarm-clearance-status") ?: "cleared";

	node = lydx_get_child(in, "older-than");
	if (node) {
		const struct { const char *unit; time_t mul; } units[] = {
			{ "seconds", 1 }, { "minutes", 60 }, { "hours", 3600 },
			{ "days", 86400 }, { "weeks", 7 * 86400 },
		};
		for (size_t i = 0; i < NELEMS(units); i++) {
			val = lydx_get_cattr(node, units[i].unit);
			if (val)
				f->older_than = (time_t)atol(val) * units[i].mul;
		}
	}

	node = lydx_get_child(in, "severity");
	if (node) {
		const char *ops[] = { "below", "is", "above" };

		for (size_t i = 0; i < NELEMS(ops); i++) {
			val = lydx_get_cattr(node, ops[i]);
			if (val) {
				f->sev_op = ops[i];
				f->sev_rank = severity_rank(val);
			}
		}
	}

	node = lydx_get_child(in, "operator-state-filter");
	if (node) {
		f->op_state = lydx_get_cattr(node, "state");
		f->op_user = lydx_get_cattr(node, "user");
	}
}

/* The last operator state of an alarm, "none" and "" when it has none */
static void last_operator_state(const char *typ, const char *name, const char **state, const char **user, json_t **keep)
{
	char path[PATH_MAX];
	json_t *history, *last;

	*state = "none";
	*user = "";
	*keep = NULL;

	snprintf(path, sizeof(path), ALARM_DIR "/state/%s/%s", typ, name);
	history = json_load_file(path, 0, NULL);
	if (!history)
		return;
	last = json_array_get(history, json_array_size(history) - 1);
	if (last) {
		*state = json_string_value(json_object_get(last, "state")) ?: "none";
		*user = json_string_value(json_object_get(last, "operator")) ?: "";
	}
	*keep = history;
}

static int alarm_match(const struct alarm_filter *f, const char *typ, const char *name, json_t *entry, time_t now)
{
	const char *shelf = json_string_value(json_object_get(entry, "shelf"));
	int cleared = json_is_true(json_object_get(entry, "is-cleared"));
	const char *state, *user;
	json_t *history;
	int match = 1;

	if ((shelf && *shelf) != f->shelved)
		return 0;
	if (!strcmp(f->clearance, "cleared") && !cleared)
		return 0;
	if (!strcmp(f->clearance, "not-cleared") && cleared)
		return 0;

	if (f->older_than) {
		time_t changed = parse_stamp(json_string_value(json_object_get(entry, "last-changed")));

		if (!changed || now - changed < f->older_than)
			return 0;
	}

	if (f->sev_op) {
		int rank = severity_rank(json_string_value(json_object_get(entry, "severity")));

		if (!strcmp(f->sev_op, "below") && rank >= f->sev_rank)
			return 0;
		if (!strcmp(f->sev_op, "is") && rank != f->sev_rank)
			return 0;
		if (!strcmp(f->sev_op, "above") && rank <= f->sev_rank)
			return 0;
	}

	if (f->op_state || f->op_user) {
		last_operator_state(typ, name, &state, &user, &history);
		if (f->op_state && strcmp(f->op_state, state))
			match = 0;
		if (f->op_user && strcmp(f->op_user, user))
			match = 0;
		json_decref(history);
	}

	return match;
}

/* Splits .../entry/<typ>/<name> in a copy of the path */
static void entry_type_name(char *copy, size_t len, const char *path, char **typ, char **name)
{
	snprintf(copy, len, "%s", path);
	*name = strrchr(copy, '/');
	**name = 0;
	(*name)++;
	*typ = strrchr(copy, '/') + 1;
}

/* The alarm-list action of infix-alarms: one operator state on every
 * alarm the filter selects, the ones already in that state skipped */
static int set_operator_states(sr_session_ctx_t *session, uint32_t sub_id, const char *op_path,
			       const struct lyd_node *input, sr_event_t event, uint32_t request_id,
			       struct lyd_node *output, void *priv)
{
	struct lyd_node *in = (struct lyd_node *)input;
	const char *state, *text;
	struct alarm_filter filter;
	time_t now = time(NULL);
	char count[16];
	int changed = 0;
	glob_t gl;

	if (event != SR_EV_RPC)
		return SR_ERR_OK;

	state = lydx_get_cattr(in, "state") ?: "none";
	text = lydx_get_cattr(in, "text");
	alarm_filter_init(&filter, in, 0);

	if (!glob(ALARM_DIR "/entry/*/*", 0, NULL, &gl)) {
		for (size_t i = 0; i < gl.gl_pathc; i++) {
			const char *path = gl.gl_pathv[i];
			const char *last, *user;
			json_t *entry, *history;
			char copy[PATH_MAX];
			char *name, *typ;
			int same;

			entry = json_load_file(path, 0, NULL);
			if (!entry)
				continue;

			entry_type_name(copy, sizeof(copy), path, &typ, &name);
			if (alarm_match(&filter, typ, name, entry, now)) {
				last_operator_state(typ, name, &last, &user, &history);
				same = !strcmp(last, state);
				json_decref(history);
				if (!same && !record_operator_state(session, typ, name, state, text))
					changed++;
			}
			json_decref(entry);
		}
		globfree(&gl);
	}

	snprintf(count, sizeof(count), "%d", changed);
	if (lyd_new_term(output, NULL, "changed-alarms", count, LYD_NEW_VAL_OUTPUT, NULL)) {
		ERROR("set-operator-state: failed setting output");
		return SR_ERR_INTERNAL;
	}

	return SR_ERR_OK;
}

static int purge_alarms(sr_session_ctx_t *session, uint32_t sub_id, const char *op_path,
			const struct lyd_node *input, sr_event_t event, uint32_t request_id,
			struct lyd_node *output, void *priv)
{
	struct lyd_node *in = (struct lyd_node *)input;
	struct alarm_filter filter;
	time_t now = time(NULL);
	char count[16];
	int purged = 0;
	glob_t gl;

	if (event != SR_EV_RPC)
		return SR_ERR_OK;

	alarm_filter_init(&filter, in, priv != NULL);

	if (!glob(ALARM_DIR "/entry/*/*", 0, NULL, &gl)) {
		for (size_t i = 0; i < gl.gl_pathc; i++) {
			char copy[PATH_MAX], other[PATH_MAX];
			const char *path = gl.gl_pathv[i];
			char *name, *typ;
			json_t *entry;

			entry = json_load_file(path, 0, NULL);
			if (!entry)
				continue;

			entry_type_name(copy, sizeof(copy), path, &typ, &name);
			if (alarm_match(&filter, typ, name, entry, now)) {
				remove(path);
				snprintf(other, sizeof(other), ALARM_DIR "/state/%s/%s", typ, name);
				remove(other);
				snprintf(other, sizeof(other), ALARM_DIR "/active/%s/%s", typ, name);
				remove(other);
				purged++;
			}
			json_decref(entry);
		}
		globfree(&gl);
	}

	AUDIT("Purged %d %salarms by \"%s\".", purged, filter.shelved ? "shelved " : "", operator_name(session));
	snprintf(count, sizeof(count), "%d", purged);
	if (lyd_new_term(output, NULL, "purged-alarms", count, LYD_NEW_VAL_OUTPUT, NULL)) {
		ERROR("purge-alarms: failed setting output");
		return SR_ERR_INTERNAL;
	}

	return SR_ERR_OK;
}

static int raise_alarm(sr_session_ctx_t *session, uint32_t sub_id, const char *op_path,
		       const struct lyd_node *input, sr_event_t event, uint32_t request_id,
		       struct lyd_node *output, void *priv)
{
	const char *resource, *qualifier, *severity, *text;
	struct lyd_node *in = (struct lyd_node *)input;
	char name[NAME_MAX_LEN], path[PATH_MAX];
	json_t *marker;

	if (event != SR_EV_RPC)
		return SR_ERR_OK;

	resource = lydx_get_cattr(in, "resource");
	qualifier = lydx_get_cattr(in, "alarm-type-qualifier");
	severity = lydx_get_cattr(in, "severity") ?: "major";
	text = lydx_get_cattr(in, "text");

	if (!valid_name(resource))
		return rpc_failed(session, "Resource must be a single word: letters, digits, '.', '_', ':' or '-'");
	if (qualifier && *qualifier && !valid_name(qualifier))
		return rpc_failed(session, "Qualifier must be a single word: letters, digits, '.', '_', ':' or '-'");
	if (!text || !*text)
		return rpc_failed(session, "Alarm text is required");

	marker_name(name, sizeof(name), resource, qualifier);
	snprintf(path, sizeof(path), ALARM_DIR "/active/" EXTERNAL "/%s", name);

	marker = json_pack("{s:s, s:s}", "severity", severity, "text", text);
	AUDIT("Alarm " EXTERNAL " %s raised (%s) by \"%s\".", name, severity, operator_name(session));
	if (save_json(path, marker)) {
		json_decref(marker);
		return rpc_failed(session, "Failed raising alarm");
	}
	json_decref(marker);

	return SR_ERR_OK;
}

static int clear_alarm(sr_session_ctx_t *session, uint32_t sub_id, const char *op_path,
		       const struct lyd_node *input, sr_event_t event, uint32_t request_id,
		       struct lyd_node *output, void *priv)
{
	struct lyd_node *in = (struct lyd_node *)input;
	char name[NAME_MAX_LEN], path[PATH_MAX];
	const char *resource, *qualifier;

	if (event != SR_EV_RPC)
		return SR_ERR_OK;

	resource = lydx_get_cattr(in, "resource");
	qualifier = lydx_get_cattr(in, "alarm-type-qualifier");
	if (!valid_name(resource) || (qualifier && *qualifier && !valid_name(qualifier)))
		return rpc_failed(session, "No such alarm");

	marker_name(name, sizeof(name), resource, qualifier);
	snprintf(path, sizeof(path), ALARM_DIR "/active/" EXTERNAL "/%s", name);
	if (remove(path)) {
		if (errno == ENOENT)
			return rpc_failed(session, "No such alarm");
		ERRNO("failed removing %s", path);
		return rpc_failed(session, "Failed clearing alarm");
	}

	AUDIT("Alarm " EXTERNAL " %s cleared by \"%s\".", name, operator_name(session));
	return SR_ERR_OK;
}

int alarm_rpc_init(struct confd *confd)
{
	int rc;

	REGISTER_RPC_TREE(confd->session, ALARM_XPATH "/alarm-list/alarm/set-operator-state",
			  set_operator_state, NULL, &confd->sub);
	REGISTER_RPC_TREE(confd->session, ALARM_XPATH "/alarm-list/infix-alarms:set-operator-state",
			  set_operator_states, NULL, &confd->sub);
	REGISTER_RPC_TREE(confd->session, ALARM_XPATH "/alarm-list/purge-alarms",
			  purge_alarms, NULL, &confd->sub);
	REGISTER_RPC_TREE(confd->session, ALARM_XPATH "/shelved-alarms/purge-shelved-alarms",
			  purge_alarms, "shelved", &confd->sub);
	REGISTER_RPC_TREE(confd->session, "/infix-alarms:raise-alarm", raise_alarm, NULL, &confd->sub);
	REGISTER_RPC_TREE(confd->session, "/infix-alarms:clear-alarm", clear_alarm, NULL, &confd->sub);

	return SR_ERR_OK;
fail:
	return rc;
}
