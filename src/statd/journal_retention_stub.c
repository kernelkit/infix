/* SPDX-License-Identifier: BSD-3-Clause */

/*
 * Test stub for journal retention policy
 *
 * This program applies the retention policy to a directory of timestamped
 * JSON snapshots. It's used by the Python unit tests.
 *
 * Usage: journal_retention_stub <directory> <now>
 *   directory: Path to directory containing timestamped JSON snapshots
 *   now: Unix timestamp (seconds since epoch) representing "current time"
 *
 * Example:
 *   journal_retention_stub /tmp/testdir 1704067200
 */

#include <stdio.h>
#include <stdlib.h>

#include "journal.h"

int main(int argc, char *argv[])
{
	struct snapshot *snapshots;
	int snapshot_count;
	time_t now;

	if (argc != 3) {
		fprintf(stderr, "Usage: %s <directory> <unix_timestamp>\n", argv[0]);
		fprintf(stderr, "  directory: Path to snapshots directory\n");
		fprintf(stderr, "  unix_timestamp: Current time as seconds since epoch\n");
		return 1;
	}

	now = (time_t)atol(argv[2]);

	if (journal_scan_snapshots(argv[1], &snapshots, &snapshot_count) != 0)
		return 1;

	journal_apply_retention_policy(argv[1], snapshots, snapshot_count, now);

	free(snapshots);
	return 0;
}
