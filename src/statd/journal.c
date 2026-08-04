/* SPDX-License-Identifier: BSD-3-Clause */

/*
 * Periodic snapshots of the operational datastore for post-mortem and
 * trend analysis: /var/lib/statd/operational.json is always the latest,
 * with gzipped timestamped archives kept according to the retention
 * policy in journal_retention.c.
 *
 * The work runs in a forked child, renamed statd-journal, at reduced
 * priority.  This keeps statd's event loop free to serve operational
 * get callbacks -- including those triggered by the snapshot itself.
 * The dump is chunked per YANG module, releasing all datastore locks
 * between each read, so configuration changes and status queries from
 * interactive users interleave with the dump instead of queueing up
 * behind one long read.
 */

#include <errno.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/prctl.h>
#include <sys/stat.h>
#include <sys/wait.h>
#include <time.h>
#include <unistd.h>
#include <zlib.h>

#include <ev.h>
#include <libyang/libyang.h>
#include <sysrepo.h>

#include <srx/common.h>

#include "journal.h"

#define JOURNAL_DIR   "/var/lib/statd"
#define DUMP_FILE     JOURNAL_DIR "/operational.json"
#define DUMP_INTERVAL 300.0	/* seconds of rest between snapshots */
#define CHUNK_DELAY   50000	/* us breather between module reads */
#define CHUNK_TIMEOUT 10000	/* ms, keep short: the read holds module locks
				 * that configuration changes wait on */

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
	ret = lyd_print_path(DUMP_FILE, tree, LYD_JSON, LYD_PRINT_SIBLINGS);
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

/*
 * Read operational data one module at a time, merging into a single
 * tree.  Every sr_get_data() releases its locks on return, giving
 * other datastore users a chance to run between chunks.
 */
static struct lyd_node *dump_modules(sr_session_ctx_t *ses, const struct ly_ctx *ctx,
				     int *skipped)
{
	const struct lys_module *mod;
	struct lyd_node *tree = NULL;
	uint32_t idx = 0;

	while ((mod = ly_ctx_get_module_iter(ctx, &idx))) {
		char xpath[300];
		sr_data_t *data;
		int err;

		if (!mod->implemented || !mod->compiled || !mod->compiled->data)
			continue;

		snprintf(xpath, sizeof(xpath), "/%s:*", mod->name);
		err = sr_get_data(ses, xpath, 0, CHUNK_TIMEOUT, 0, &data);
		if (err) {
			INFO("Skipping %s: %s", mod->name, sr_strerror(err));
			(*skipped)++;
			continue;
		}

		if (data) {
			if (data->tree && lyd_merge_siblings(&tree, data->tree, 0))
				ERROR("Error, merging %s data", mod->name);
			sr_release_data(data);
		}

		usleep(CHUNK_DELAY);
	}

	return tree;
}

/*
 * Forked child: fresh sysrepo connection, dump, archive, retention,
 * then _exit() -- never touch inherited statd state.
 */
static void snapshot_process(void)
{
	struct snapshot *snapshots = NULL;
	sr_session_ctx_t *ses = NULL;
	sr_conn_ctx_t *conn = NULL;
	struct timespec start, end;
	const struct ly_ctx *ctx;
	struct lyd_node *tree;
	int rc = EXIT_FAILURE;
	int skipped = 0;
	int count = 0;
	long ms;

	prctl(PR_SET_NAME, "statd-journal", 0, 0, 0);
	closelog();	/* drop log connection inherited from statd */
	openlog("statd-journal", LOG_PID | LOG_NDELAY | (debug ? LOG_PERROR : 0), LOG_DAEMON);
	nice(10);

	NOTE("Starting operational datastore snapshot");
	clock_gettime(CLOCK_MONOTONIC, &start);

	if (mkdir(JOURNAL_DIR, 0755) && errno != EEXIST)
		ERROR("Error, creating directory " JOURNAL_DIR ": %s", strerror(errno));

	if (sr_connect(SR_CONN_DEFAULT, &conn)) {
		ERROR("Error, connecting to sysrepo");
		_exit(rc);
	}
	if (sr_session_start(conn, SR_DS_OPERATIONAL, &ses)) {
		ERROR("Error, starting session");
		goto done;
	}

	ctx = sr_acquire_context(conn);
	if (!ctx) {
		ERROR("Error, acquiring context");
		goto done;
	}

	tree = dump_modules(ses, ctx, &skipped);
	if (tree) {
		rc = create_snapshot(tree) ? EXIT_FAILURE : EXIT_SUCCESS;
		lyd_free_all(tree);
	} else {
		DEBUG("No operational data to dump");
		rc = EXIT_SUCCESS;
	}
	sr_release_context(conn);

	if (journal_scan_snapshots(JOURNAL_DIR, &snapshots, &count) == 0) {
		DEBUG("Applying retention policy to %d snapshots", count);
		journal_apply_retention_policy(JOURNAL_DIR, snapshots, count, time(NULL));
		free(snapshots);
	}

	clock_gettime(CLOCK_MONOTONIC, &end);
	ms = (end.tv_sec - start.tv_sec) * 1000 +
	     (end.tv_nsec - start.tv_nsec) / 1000000;
	if (skipped)
		NOTE("Snapshot created and retention applied (took %ld ms, %d modules busy, skipped)",
		     ms, skipped);
	else
		NOTE("Snapshot created and retention applied (took %ld ms)", ms);
done:
	if (ses)
		sr_session_stop(ses);
	sr_disconnect(conn);
	_exit(rc);
}

/*
 * The timer is one-shot, re-armed only when the previous snapshot has
 * finished.  Snapshots can thus never overlap, and DUMP_INTERVAL is
 * the rest between them rather than a fixed cadence -- on a slow, or
 * busy, system snapshots are simply taken further apart.
 */
static void journal_rearm(struct journal_ctx *jctx)
{
	ev_timer_set(&jctx->timer, DUMP_INTERVAL, 0.0);
	ev_timer_start(jctx->loop, &jctx->timer);
}

static void journal_child_cb(struct ev_loop *loop, struct ev_child *w, int revents)
{
	struct journal_ctx *jctx = (struct journal_ctx *)
		((char *)w - offsetof(struct journal_ctx, child));

	(void)revents;

	ev_child_stop(loop, w);
	jctx->pid = 0;

	if (!WIFEXITED(w->rstatus) || WEXITSTATUS(w->rstatus))
		ERROR("Journal snapshot failed, status %d", w->rstatus);

	journal_rearm(jctx);
}

static void journal_timer_cb(struct ev_loop *loop, ev_timer *w, int revents)
{
	struct journal_ctx *jctx = (struct journal_ctx *)
		((char *)w - offsetof(struct journal_ctx, timer));
	pid_t pid;

	(void)revents;

	pid = fork();
	if (pid < 0) {
		ERRNO("Failed forking journal snapshot process");
		journal_rearm(jctx);
		return;
	}
	if (!pid)
		snapshot_process();	/* never returns */

	jctx->pid = pid;
	ev_child_init(&jctx->child, journal_child_cb, pid, 0);
	ev_child_start(loop, &jctx->child);
}

int journal_start(struct journal_ctx *jctx, struct ev_loop *loop)
{
	jctx->loop = loop;
	jctx->pid  = 0;

	ev_timer_init(&jctx->timer, journal_timer_cb, DUMP_INTERVAL, 0.0);
	ev_timer_start(loop, &jctx->timer);

	NOTE("Periodic operational snapshot enabled (every %.0f seconds)", DUMP_INTERVAL);
	return 0;
}

void journal_stop(struct journal_ctx *jctx)
{
	ev_timer_stop(jctx->loop, &jctx->timer);

	/* Snapshot in progress completes on its own, reaped by init */
	if (jctx->pid)
		ev_child_stop(jctx->loop, &jctx->child);
}
