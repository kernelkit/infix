/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef STATD_JOURNAL_H_
#define STATD_JOURNAL_H_

#include <time.h>

#ifndef JOURNAL_RETENTION_STUB
#include <sys/types.h>
#include <ev.h>

struct journal_ctx {
	struct ev_loop *loop;
	ev_timer        timer;   /* Periodic snapshot trigger */
	struct ev_child child;   /* Reaper for the snapshot process */
	pid_t           pid;     /* Non-zero while a snapshot is running */
};

int  journal_start(struct journal_ctx *jctx, struct ev_loop *loop);
void journal_stop(struct journal_ctx *jctx);
#endif

/* Snapshot structure for tracking journal files */
struct snapshot {
	char filename[256];
	time_t timestamp;
};

int journal_scan_snapshots(const char *dir, struct snapshot **snapshots, int *count);
void journal_apply_retention_policy(const char *dir, struct snapshot *snapshots, int count, time_t now);

#endif
