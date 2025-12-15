/* SPDX-License-Identifier: BSD-3-Clause */

#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <sysrepo.h>
#include <ev.h>
#include <string.h>
#include <errno.h>
#include <time.h>
#include <sys/stat.h>
#include <pthread.h>
#include <dirent.h>
#include <zlib.h>

#include <srx/common.h>

#include "journal.h"

#define JOURNAL_DIR "/var/lib/statd"
#define DUMP_FILE "/var/lib/statd/operational.json"
#define DUMP_INTERVAL 300.0  /* 5 minutes in seconds */

static void journal_stop_cb(struct ev_loop *loop, struct ev_async *, int)
{
	DEBUG("Journal thread stop signal received");
	ev_break(loop, EVBREAK_ALL);
}

static void get_timestamp_filename(char *buf, size_t len, time_t ts)
{
	struct tm *tm = gmtime(&ts);

	snprintf(buf, len, "%04d%02d%02d-%02d%02d%02d.json.gz",
		 tm->tm_year + 1900, tm->tm_mon + 1, tm->tm_mday,
		 tm->tm_hour, tm->tm_min, tm->tm_sec);
}

/* Compress a file using gzip */
static int gzip_file(const char *src, const char *dst)
{
	FILE *in;
	gzFile gz;
	char buf[4096];
	size_t n;

	in = fopen(src, "r");
	if (!in) {
		ERROR("Error, opening %s: %s", src, strerror(errno));
		return -1;
	}

	gz = gzopen(dst, "wb");
	if (!gz) {
		ERROR("Error, opening %s: %s", dst, strerror(errno));
		fclose(in);
		return -1;
	}

	while ((n = fread(buf, 1, sizeof(buf), in)) > 0) {
		if (gzwrite(gz, buf, n) != (int)n) {
			ERROR("Error, writing to %s", dst);
			gzclose(gz);
			fclose(in);
			unlink(dst);
			return -1;
		}
	}

	gzclose(gz);
	fclose(in);
	return 0;
}

/* Create timestamped snapshot and update operational.json */
static int create_snapshot(const struct lyd_node *tree)
{
	char timestamp_file[300];
	char timestamp_path[512];
	time_t now;
	int ret;

	/* Write latest snapshot as uncompressed operational.json for easy access */
	ret = lyd_print_path(DUMP_FILE, tree, LYD_JSON, LYD_PRINT_WITHSIBLINGS);
	if (ret != LY_SUCCESS) {
		ERROR("Error, writing operational.json: %d", ret);
		return -1;
	}

	/* Compress operational.json to timestamped archive */
	now = time(NULL);
	get_timestamp_filename(timestamp_file, sizeof(timestamp_file), now);
	snprintf(timestamp_path, sizeof(timestamp_path), "%s/%s",
		 JOURNAL_DIR, timestamp_file);

	if (gzip_file(DUMP_FILE, timestamp_path) != 0) {
		ERROR("Error, compressing snapshot to %s", timestamp_file);
		return -1;
	}

	DEBUG("Created snapshot %s", timestamp_file);
	return 0;
}

static void journal_timer_cb(struct ev_loop *, struct ev_timer *w, int)
{
	struct journal_ctx *jctx = (struct journal_ctx *)w->data;
	struct timespec start, end;
	struct snapshot *snapshots = NULL;
	sr_conn_ctx_t *con;
	const struct ly_ctx *ctx;
	sr_data_t *sr_data = NULL;
	sr_error_t err;
	int snapshot_count = 0;
	long duration_ms;

	clock_gettime(CLOCK_MONOTONIC, &start);
	DEBUG("Starting operational datastore dump");

	con = sr_session_get_connection(jctx->sr_query_ses);
	if (!con) {
		ERROR("Error, getting sr connection for dump");
		return;
	}

	ctx = sr_acquire_context(con);
	if (!ctx) {
		ERROR("Error, acquiring context for dump");
		return;
	}

	/* Query ALL operational data via second session
	 * This triggers our own operational callbacks running in main thread
	 */
	DEBUG("Calling sr_get_data on session %p", jctx->sr_query_ses);
	err = sr_get_data(jctx->sr_query_ses, "/*", 0, 0, 0, &sr_data);
	if (err != SR_ERR_OK) {
		ERROR("Error, getting operational data: %s", sr_strerror(err));
		sr_release_context(con);
		return;
	}
	DEBUG("sr_get_data succeeded, got data tree: %p", sr_data ? sr_data->tree : NULL);

	/* Create timestamped snapshot */
	if (sr_data && sr_data->tree) {
		if (create_snapshot(sr_data->tree) != 0) {
			sr_release_data(sr_data);
			sr_release_context(con);
			return;
		}
	} else {
		DEBUG("No operational data to dump");
	}

	sr_release_data(sr_data);
	sr_release_context(con);

	/* Apply retention policy */
	if (journal_scan_snapshots(JOURNAL_DIR, &snapshots, &snapshot_count) == 0) {
		DEBUG("Applying retention policy to %d snapshots", snapshot_count);
		journal_apply_retention_policy(JOURNAL_DIR, snapshots, snapshot_count, time(NULL));
		free(snapshots);
	}

	clock_gettime(CLOCK_MONOTONIC, &end);
	duration_ms = (end.tv_sec - start.tv_sec) * 1000 +
		      (end.tv_nsec - start.tv_nsec) / 1000000;

	INFO("Journal snapshot created and retention applied (took %ld ms)", duration_ms);
}

static void *journal_thread_fn(void *arg)
{
	struct journal_ctx *jctx = (struct journal_ctx *)arg;
	struct ev_timer journal_timer;

	INFO("Journal thread started");

	if (mkdir("/var/lib/statd", 0755) != 0 && errno != EEXIST) {
		ERROR("Error, creating directory /var/lib/statd: %s", strerror(errno));
	}

	jctx->journal_loop = ev_loop_new(EVFLAG_AUTO);
	if (!jctx->journal_loop) {
		ERROR("Error, creating journal thread event loop");
		return NULL;
	}

	/* Setup async watcher for stop signal */
	ev_async_init(&jctx->journal_stop, journal_stop_cb);
	ev_async_start(jctx->journal_loop, &jctx->journal_stop);

	/* Setup timer for periodic dumps */
	ev_timer_init(&journal_timer, journal_timer_cb, DUMP_INTERVAL, DUMP_INTERVAL);
	journal_timer.data = jctx;
	ev_timer_start(jctx->journal_loop, &journal_timer);

	DEBUG("Journal thread entering event loop");
	ev_run(jctx->journal_loop, 0);

	ev_timer_stop(jctx->journal_loop, &journal_timer);
	ev_async_stop(jctx->journal_loop, &jctx->journal_stop);
	ev_loop_destroy(jctx->journal_loop);

	INFO("Journal thread exiting");
	return NULL;
}

int journal_start(struct journal_ctx *jctx, sr_session_ctx_t *sr_query_ses)
{
	int err;

	jctx->sr_query_ses = sr_query_ses;
	jctx->journal_thread_running = 1;

	err = pthread_create(&jctx->journal_thread, NULL, journal_thread_fn, jctx);
	if (err) {
		ERROR("Error, creating journal thread: %s", strerror(err));
		return err;
	}

	INFO("Periodic operational dump enabled (every %.0f seconds)", DUMP_INTERVAL);
	return 0;
}

void journal_stop(struct journal_ctx *jctx)
{
	/* Signal thread to exit immediately via async watcher */
	jctx->journal_thread_running = 0;
	ev_async_send(jctx->journal_loop, &jctx->journal_stop);
	pthread_join(jctx->journal_thread, NULL);
}
