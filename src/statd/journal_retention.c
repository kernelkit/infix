/* SPDX-License-Identifier: BSD-3-Clause */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <dirent.h>
#include <unistd.h>
#include <errno.h>

#ifndef JOURNAL_RETENTION_STUB
#include <srx/common.h>
#else
/* Simple logging for test stub without srx dependency */
#define ERROR(fmt, ...) fprintf(stderr, "ERROR: " fmt "\n", ##__VA_ARGS__)
#define DEBUG(fmt, ...) do { } while (0)
#endif

#include "journal.h"

/* Retention policy age thresholds */
#define AGE_1_HOUR    (60 * 60)
#define AGE_1_DAY     (24 * AGE_1_HOUR)
#define AGE_1_WEEK    (7 * AGE_1_DAY)
#define AGE_1_MONTH   (30 * AGE_1_DAY)
#define AGE_1_YEAR    (365 * AGE_1_DAY)

static int parse_timestamp_filename(const char *filename, time_t *ts)
{
	struct tm tm = {0};
	int year, mon, day, hour, min, sec;

	if (sscanf(filename, "%4d%2d%2d-%2d%2d%2d.json",
		   &year, &mon, &day, &hour, &min, &sec) != 6)
		return -1;

	tm.tm_year = year - 1900;
	tm.tm_mon = mon - 1;
	tm.tm_mday = day;
	tm.tm_hour = hour;
	tm.tm_min = min;
	tm.tm_sec = sec;

	*ts = timegm(&tm);
	return 0;
}

/* Comparison function for qsort */
static int snapshot_compare(const void *a, const void *b)
{
	const struct snapshot *sa = (const struct snapshot *)a;
	const struct snapshot *sb = (const struct snapshot *)b;

	if (sa->timestamp < sb->timestamp)
		return -1;
	if (sa->timestamp > sb->timestamp)
		return 1;
	return 0;
}

int journal_scan_snapshots(const char *dir, struct snapshot **out_snapshots, int *out_count)
{
	DIR *d = opendir(dir);
	struct dirent *entry;
	struct snapshot *snapshots = NULL;
	int count = 0;
	int capacity = 0;

	if (!d) {
		ERROR("Failed to open directory %s: %s", dir, strerror(errno));
		return -1;
	}

	while ((entry = readdir(d)) != NULL) {
		time_t ts;

		if (strcmp(entry->d_name, "operational.json") == 0)
			continue;
		if (!strstr(entry->d_name, ".json"))
			continue;

		if (parse_timestamp_filename(entry->d_name, &ts) != 0)
			continue;

		if (count >= capacity) {
			struct snapshot *new_snapshots;

			capacity = capacity ? capacity * 2 : 32;
			new_snapshots = realloc(snapshots, capacity * sizeof(struct snapshot));
			if (!new_snapshots) {
				ERROR("Failed to allocate memory for snapshots");
				free(snapshots);
				closedir(d);
				return -1;
			}
			snapshots = new_snapshots;
		}

		/* Add snapshot */
		snprintf(snapshots[count].filename, sizeof(snapshots[count].filename),
			 "%s", entry->d_name);
		snapshots[count].timestamp = ts;
		count++;
	}

	closedir(d);

	/* Sort by timestamp (oldest first) */
	if (count > 0)
		qsort(snapshots, count, sizeof(struct snapshot), snapshot_compare);

	*out_snapshots = snapshots;
	*out_count = count;
	return 0;
}

static void delete_snapshot(const char *dir, const char *filename)
{
	char path[512];

	snprintf(path, sizeof(path), "%s/%s", dir, filename);
	if (unlink(path) != 0)
		ERROR("Failed to delete snapshot %s: %s", filename, strerror(errno));
	else
		DEBUG("Deleted snapshot %s", filename);
}

/* Check if this is the first snapshot in the given period by checking if there's
 * an earlier snapshot in the same period */
static int is_first_in_period(struct snapshot *snapshots, int count, int idx,
			       int (*same_period)(time_t a, time_t b))
{
	int i;

	for (i = 0; i < idx; i++) {
		if (same_period(snapshots[i].timestamp, snapshots[idx].timestamp))
			return 0;  /* Found earlier snapshot in same period */
	}

	return 1;  /* This is the first */
}

/* Check if two timestamps are in the same hour */
static int same_hour(time_t a, time_t b)
{
	struct tm tm_a, tm_b;

	gmtime_r(&a, &tm_a);
	gmtime_r(&b, &tm_b);

	return tm_a.tm_year == tm_b.tm_year &&
	       tm_a.tm_yday == tm_b.tm_yday &&
	       tm_a.tm_hour == tm_b.tm_hour;
}

/* Check if two timestamps are in the same day */
static int same_day(time_t a, time_t b)
{
	struct tm tm_a, tm_b;

	gmtime_r(&a, &tm_a);
	gmtime_r(&b, &tm_b);

	return tm_a.tm_year == tm_b.tm_year &&
	       tm_a.tm_yday == tm_b.tm_yday;
}

/* Check if two timestamps are in the same week (Sunday-based) */
static int same_week(time_t a, time_t b)
{
	struct tm tm_a, tm_b;
	time_t sunday_a, sunday_b;

	gmtime_r(&a, &tm_a);
	gmtime_r(&b, &tm_b);

	/* Calculate Sunday midnight for each timestamp */
	tm_a.tm_mday -= tm_a.tm_wday;
	tm_a.tm_hour = 0;
	tm_a.tm_min = 0;
	tm_a.tm_sec = 0;
	sunday_a = timegm(&tm_a);

	tm_b.tm_mday -= tm_b.tm_wday;
	tm_b.tm_hour = 0;
	tm_b.tm_min = 0;
	tm_b.tm_sec = 0;
	sunday_b = timegm(&tm_b);

	return sunday_a == sunday_b;
}

/* Check if two timestamps are in the same month */
static int same_month(time_t a, time_t b)
{
	struct tm tm_a, tm_b;

	gmtime_r(&a, &tm_a);
	gmtime_r(&b, &tm_b);

	return tm_a.tm_year == tm_b.tm_year &&
	       tm_a.tm_mon == tm_b.tm_mon;
}

/* Check if two timestamps are in the same year */
static int same_year(time_t a, time_t b)
{
	struct tm tm_a, tm_b;

	gmtime_r(&a, &tm_a);
	gmtime_r(&b, &tm_b);

	return tm_a.tm_year == tm_b.tm_year;
}

/* Keep all snapshots in the 5-minute bucket */
static int should_keep_5min(struct snapshot *snapshots, int count, int idx)
{
	(void)snapshots;
	(void)count;
	(void)idx;
	return 1;  /* Keep all */
}

static int should_keep_hourly(struct snapshot *snapshots, int count, int idx)
{
	return is_first_in_period(snapshots, count, idx, same_hour);
}

static int should_keep_daily(struct snapshot *snapshots, int count, int idx)
{
	return is_first_in_period(snapshots, count, idx, same_day);
}

static int should_keep_weekly(struct snapshot *snapshots, int count, int idx)
{
	return is_first_in_period(snapshots, count, idx, same_week);
}

static int should_keep_monthly(struct snapshot *snapshots, int count, int idx)
{
	return is_first_in_period(snapshots, count, idx, same_month);
}

static int should_keep_yearly(struct snapshot *snapshots, int count, int idx)
{
	return is_first_in_period(snapshots, count, idx, same_year);
}

/* Apply retention policy to snapshots */
void journal_apply_retention_policy(const char *dir, struct snapshot *snapshots, int count, time_t now)
{
	int *keep;
	int i;

	if (count == 0)
		return;

	/* Mark snapshots to keep (1) or delete (0) */
	keep = calloc(count, sizeof(int));
	if (!keep) {
		ERROR("Failed to allocate memory for retention policy");
		return;
	}

	for (i = 0; i < count; i++) {
		time_t age = now - snapshots[i].timestamp;

		if (age <= AGE_1_HOUR) {
			keep[i] = should_keep_5min(snapshots, count, i);
		} else if (age <= AGE_1_DAY) {
			keep[i] = should_keep_hourly(snapshots, count, i);
		} else if (age <= AGE_1_WEEK) {
			keep[i] = should_keep_daily(snapshots, count, i);
		} else if (age <= AGE_1_MONTH) {
			keep[i] = should_keep_weekly(snapshots, count, i);
		} else if (age <= AGE_1_YEAR) {
			keep[i] = should_keep_monthly(snapshots, count, i);
		} else {
			keep[i] = should_keep_yearly(snapshots, count, i);
		}
	}

	/* Delete snapshots not marked for keeping */
	for (i = 0; i < count; i++) {
		if (!keep[i])
			delete_snapshot(dir, snapshots[i].filename);
	}

	free(keep);
}
