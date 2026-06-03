/* SPDX-License-Identifier: BSD-3-Clause */

#include <signal.h>
#include <stdio.h>
#include <stdlib.h>

#include <libite/lite.h>
#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>
#include "core.h"

#define XPATH_BASE    "/ietf-system:system/infix-schedule:schedules"
#define CRONTAB_FILE  "/var/spool/cron/crontabs/admin"

/* Features register a consumer to run a command on a schedule. */
static const struct cron_consumer **consumers;
static size_t consumer_count;

int schedule_consumer_register(const struct cron_consumer *consumer)
{
	const struct cron_consumer **vec;

	if (!consumer || !consumer->path || !consumer->sched_leaf || !consumer->command)
		return -1;

	vec = realloc(consumers, (consumer_count + 1) * sizeof(*vec));
	if (!vec) {
		ERROR("schedule: out of memory registering %s", consumer->path);
		return -1;
	}
	consumers = vec;
	consumers[consumer_count++] = consumer;
	return 0;
}

/*
 * Convert ietf-schedule recurrence to a 5-field cron expression.
 *
 * Frequency mapping:
 *   minutely/N → *\/N * * * *
 *   hourly/N   → 0 *\/N * * *
 *   daily/N    → 0 0 *\/N * *
 *   weekly/N   → 0 0 * * *\/N
 *   monthly/N  → 0 0 1 *\/N *
 *   yearly     → 0 0 1 1 *   (interval > 1 not expressible in cron)
 *
 * Optional by-* leaves refine the expression:
 *   byminute    → replaces the minute field
 *   byhour      → replaces the hour field
 *   byday       → replaces the day-of-week field
 *   bymonthday  → replaces the day-of-month field (positive values only, 1-31)
 *   byyearmonth → replaces the month field
 */
static void build_cron_expr(struct lyd_node *recurrence, char *expr, size_t sz)
{
	char min[64], hr[64], dom[64], mon[64], dow[64];
	const char *freq, *ivstr;
	struct lyd_node *node;
	int iv, first;

	snprintf(min, sizeof(min), "*");
	snprintf(hr,  sizeof(hr),  "*");
	snprintf(dom, sizeof(dom), "*");
	snprintf(mon, sizeof(mon), "*");
	snprintf(dow, sizeof(dow), "*");

	freq  = lydx_get_cattr(recurrence, "frequency");
	ivstr = lydx_get_cattr(recurrence, "interval");
	if (!freq || !ivstr)
		goto done;

	iv = atoi(ivstr);
	if (iv <= 0)
		iv = 1;

	if (strstr(freq, "minutely")) {
		if (iv == 1)
			snprintf(min, sizeof(min), "*");
		else
			snprintf(min, sizeof(min), "*/%d", iv);
	} else if (strstr(freq, "hourly")) {
		snprintf(min, sizeof(min), "0");
		if (iv == 1)
			snprintf(hr, sizeof(hr), "*");
		else
			snprintf(hr, sizeof(hr), "*/%d", iv);
	} else if (strstr(freq, "daily")) {
		snprintf(min, sizeof(min), "0");
		snprintf(hr,  sizeof(hr),  "0");
		if (iv > 1)
			snprintf(dom, sizeof(dom), "*/%d", iv);
	} else if (strstr(freq, "weekly")) {
		snprintf(min, sizeof(min), "0");
		snprintf(hr,  sizeof(hr),  "0");
		if (iv == 1)
			snprintf(dow, sizeof(dow), "*");
		else
			snprintf(dow, sizeof(dow), "*/%d", iv);
	} else if (strstr(freq, "monthly")) {
		snprintf(min, sizeof(min), "0");
		snprintf(hr,  sizeof(hr),  "0");
		snprintf(dom, sizeof(dom), "1");
		if (iv > 1)
			snprintf(mon, sizeof(mon), "*/%d", iv);
	} else if (strstr(freq, "yearly")) {
		/* Once a year: midnight Jan 1, refined by byyearmonth/bymonthday.
		 * "every N years" (iv > 1) has no five-field cron equivalent. */
		snprintf(min, sizeof(min), "0");
		snprintf(hr,  sizeof(hr),  "0");
		snprintf(dom, sizeof(dom), "1");
		snprintf(mon, sizeof(mon), "1");
	}

	/* byminute: override minute field with explicit list */
	first = 1;
	LYX_LIST_FOR_EACH(lyd_child(recurrence), node, "byminute") {
		const char *val = lyd_get_value(node);
		if (!val) continue;
		if (first) { snprintf(min, sizeof(min), "%s", val); first = 0; }
		else strncat(min, ",", sizeof(min) - strlen(min) - 1),
		     strncat(min, val, sizeof(min) - strlen(min) - 1);
	}

	/* byhour: override hour field with explicit list */
	first = 1;
	LYX_LIST_FOR_EACH(lyd_child(recurrence), node, "byhour") {
		const char *val = lyd_get_value(node);
		if (!val) continue;
		if (first) { snprintf(hr, sizeof(hr), "%s", val); first = 0; }
		else strncat(hr, ",", sizeof(hr) - strlen(hr) - 1),
		     strncat(hr, val, sizeof(hr) - strlen(hr) - 1);
	}

	/* bymonthday: override day-of-month field */
	first = 1;
	LYX_LIST_FOR_EACH(lyd_child(recurrence), node, "bymonthday") {
		const char *val = lyd_get_value(node);
		if (!val) continue;
		if (first) { snprintf(dom, sizeof(dom), "%s", val); first = 0; }
		else strncat(dom, ",", sizeof(dom) - strlen(dom) - 1),
		     strncat(dom, val, sizeof(dom) - strlen(dom) - 1);
	}

	/* byyearmonth: override month field */
	first = 1;
	LYX_LIST_FOR_EACH(lyd_child(recurrence), node, "byyearmonth") {
		const char *val = lyd_get_value(node);
		if (!val) continue;
		if (first) { snprintf(mon, sizeof(mon), "%s", val); first = 0; }
		else strncat(mon, ",", sizeof(mon) - strlen(mon) - 1),
		     strncat(mon, val, sizeof(mon) - strlen(mon) - 1);
	}

	/* byday: override day-of-week field */
	first = 1;
	LYX_LIST_FOR_EACH(lyd_child(recurrence), node, "byday") {
		const char *val = lydx_get_cattr(node, "weekday");
		const char *num = NULL;
		if (!val) continue;
		/* map YANG weekday names to cron numbers (0=sunday) */
		if (!strcmp(val, "sunday"))    num = "0";
		else if (!strcmp(val, "monday"))    num = "1";
		else if (!strcmp(val, "tuesday"))   num = "2";
		else if (!strcmp(val, "wednesday")) num = "3";
		else if (!strcmp(val, "thursday"))  num = "4";
		else if (!strcmp(val, "friday"))    num = "5";
		else if (!strcmp(val, "saturday"))  num = "6";
		if (!num) continue;
		if (first) { snprintf(dow, sizeof(dow), "%s", num); first = 0; }
		else strncat(dow, ",", sizeof(dow) - strlen(dow) - 1),
		     strncat(dow, num, sizeof(dow) - strlen(dow) - 1);
	}

done:
	snprintf(expr, sz, "%s %s %s %s %s", min, hr, dom, mon, dow);
}

static void reload_crond(void)
{
	char *args[] = { "pkill", "-HUP", "crond", NULL };

	runbg(args, 0);
}

/*
 * Resolve a schedule by name to a cron expression.  Returns 0 on success.
 * A disabled schedule is the master kill-switch: it resolves to "not active"
 * (-1) so every feature referencing it stops firing.
 */
static int schedule_to_cron(struct lyd_node *config, const char *name,
			    char *expr, size_t sz)
{
	struct lyd_node *schedules, *sched;

	if (!config || !name)
		return -1;

	schedules = lydx_get_xpathf(config, XPATH_BASE);
	if (!schedules)
		return -1;

	LYX_LIST_FOR_EACH(lyd_child(schedules), sched, "schedule") {
		const char *sname = lydx_get_cattr(sched, "name");

		if (!sname || strcmp(sname, name))
			continue;

		if (!lydx_is_enabled(sched, "enabled"))
			return -1;

		build_cron_expr(lydx_get_child(sched, "recurrence"), expr, sz);
		return 0;
	}

	return -1;
}

/*
 * Rebuild the crontab from every registered consumer.  Each consumer points
 * at a feature container holding a schedule-ref; we resolve that to cron
 * fields and emit one line running the consumer's own command.
 */
static void apply_schedules(struct lyd_node *config)
{
	int count = 0;
	FILE *fp;
	size_t i;

	makepath("/var/spool/cron/crontabs");
	fp = fopen(CRONTAB_FILE, "w");
	if (!fp) {
		ERROR("schedule: failed to open %s", CRONTAB_FILE);
		return;
	}
	fprintf(fp, "# Managed by infix-schedule\n");

	if (!config)
		goto out;

	for (i = 0; i < consumer_count; i++) {
		const struct cron_consumer *c = consumers[i];
		struct lyd_node *node;
		const char *name;
		char expr[128];

		node = lydx_get_xpathf(config, "%s", c->path);
		if (!node)
			continue;

		if (c->enabled_leaf && !lydx_is_enabled(node, c->enabled_leaf))
			continue;

		name = lydx_get_cattr(node, c->sched_leaf);
		if (!name)
			continue;

		if (schedule_to_cron(config, name, expr, sizeof(expr))) {
			NOTE("schedule: '%s' references inactive schedule '%s', skipping", c->path, name);
			continue;
		}

		fprintf(fp, "# %s -> %s\n%s %s\n", c->path, name, expr, c->command);
		NOTE("schedule: %s → cron '%s %s'", name, expr, c->command);
		count++;
	}

out:
	fclose(fp);
	reload_crond();
	NOTE("schedule: %d active job(s) written to crontab", count);
}

int schedule_change(sr_session_ctx_t *session, struct lyd_node *config,
		    struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	if (event != SR_EV_DONE && event != SR_EV_ENABLED)
		return SR_ERR_OK;

	apply_schedules(config);
	return SR_ERR_OK;
}
