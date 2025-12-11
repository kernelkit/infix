/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef STATD_JOURNAL_H_
#define STATD_JOURNAL_H_

#include <pthread.h>
#include <sysrepo.h>
#include <ev.h>
#include <time.h>

/* Snapshot structure for tracking journal files */
struct snapshot {
	char filename[256];
	time_t timestamp;
};

struct journal_ctx {
	sr_session_ctx_t *sr_query_ses;  /* Consumer session for queries */
	struct ev_loop *journal_loop;    /* Event loop for journal thread */
	pthread_t journal_thread;        /* Thread for periodic dumps */
	struct ev_async journal_stop;    /* Signal to stop journal thread */
	volatile int journal_thread_running; /* Flag to stop journal thread */
};

int journal_start(struct journal_ctx *jctx, sr_session_ctx_t *sr_query_ses);
void journal_stop(struct journal_ctx *jctx);

int journal_scan_snapshots(const char *dir, struct snapshot **snapshots, int *count);
void journal_apply_retention_policy(const char *dir, struct snapshot *snapshots, int count, time_t now);

#endif
