#!/usr/bin/env python3
"""
Test journal retention policy

This test simulates creating snapshots over a 13-month period and verifies
that the retention policy keeps the first snapshot of each period:
- Every 5 minutes for the last hour (13 snapshots)
- First snapshot of each hour for the last day (23 snapshots)
- First snapshot of each day for the last week (6 snapshots)
- First snapshot of each week for the last month (3 snapshots)
- First snapshot of each month for the last year (11 snapshots)
- First snapshot of each year forever (1 snapshot per year)
"""

import os
import sys
import tempfile
import subprocess
from datetime import datetime

from infamy.tap import Test

def create_snapshot(test_dir, timestamp):
    """Create an empty snapshot file with the given timestamp"""
    dt = datetime.utcfromtimestamp(timestamp)
    filename = dt.strftime("%Y%m%d-%H%M%S.json.gz")
    path = os.path.join(test_dir, filename)
    open(path, 'w').close()
    return filename

def count_snapshots(test_dir):
    """Count compressed snapshot files in directory"""
    return len([f for f in os.listdir(test_dir) if f.endswith('.json.gz')])

def count_snapshots_by_age(test_dir, now):
    """Count snapshots by age bucket"""
    AGE_1_HOUR = 60 * 60
    AGE_1_DAY = 24 * AGE_1_HOUR
    AGE_1_WEEK = 7 * AGE_1_DAY
    AGE_1_MONTH = 30 * AGE_1_DAY
    AGE_1_YEAR = 365 * AGE_1_DAY

    counts = {
        'hour': 0,
        'day': 0,
        'week': 0,
        'month': 0,
        'year': 0,
        'older': 0
    }

    for filename in os.listdir(test_dir):
        if not filename.endswith('.json.gz'):
            continue

        # Parse timestamp from filename (YYYYMMDD-HHMMSS.json.gz)
        try:
            ts_str = filename.replace('.json.gz', '')
            dt = datetime.strptime(ts_str, "%Y%m%d-%H%M%S")
            ts = int(dt.timestamp())
            age = now - ts

            if age <= AGE_1_HOUR:
                counts['hour'] += 1
            elif age <= AGE_1_DAY:
                counts['day'] += 1
            elif age <= AGE_1_WEEK:
                counts['week'] += 1
            elif age <= AGE_1_MONTH:
                counts['month'] += 1
            elif age <= AGE_1_YEAR:
                counts['year'] += 1
            else:
                counts['older'] += 1
        except ValueError:
            pass

    return counts

def run_retention_stub(stub_path, test_dir, now):
    """Run the retention policy stub"""
    result = subprocess.run(
        [stub_path, test_dir, str(now)],
        capture_output=True,
        text=True
    )
    if result.returncode != 0:
        print(f"Stub failed with exit code {result.returncode}")
        print(f"Stderr: {result.stderr}")
        print(f"Stdout: {result.stdout}")
        raise Exception(f"Retention stub failed with exit code {result.returncode}")

with Test() as test:
    with test.step("Find journal_retention_stub binary"):
        script_dir = os.path.dirname(os.path.abspath(__file__))
        repo_root = os.path.abspath(os.path.join(script_dir, "../../../.."))
        stub_path = os.path.join(repo_root, "output/build/statd-1.0/journal_retention_stub")

        if not os.path.exists(stub_path):
            stub_path = os.path.join(repo_root, "src/statd/journal_retention_stub")
            if not os.path.exists(stub_path):
                test.skip()

        print(f"Using stub binary: {stub_path}")

    with tempfile.TemporaryDirectory() as test_dir:
        print(f"Using test directory: {test_dir}")

        with test.step("Create 13 months of snapshots every 5 minutes"):
            base_time = datetime(2024, 1, 1, 0, 0, 0)
            start_time = int(base_time.timestamp())
            interval_5min = 5 * 60
            interval_1day = 24 * 3600
            total_duration = 13 * 30 * 24 * 3600

            t = start_time
            last_retention_run = start_time
            while t < start_time + total_duration:
                create_snapshot(test_dir, t)
                simulated_now = t

                # Apply retention policy once per simulated day
                if t - last_retention_run >= interval_1day:
                    run_retention_stub(stub_path, test_dir, simulated_now)
                    last_retention_run = t

                t += interval_5min

            # Final retention run
            run_retention_stub(stub_path, test_dir, simulated_now)
            print(f"Final simulated time: {datetime.utcfromtimestamp(simulated_now)}")

        # Count remaining snapshots
        counts = count_snapshots_by_age(test_dir, simulated_now)
        print(f"Snapshot counts: hour={counts['hour']}, day={counts['day']}, "
              f"week={counts['week']}, month={counts['month']}, "
              f"year={counts['year']}, older={counts['older']}")

        with test.step("Verify last hour retention (13 snapshots)"):
            assert counts['hour'] == 13, f"Expected 13, got {counts['hour']}"

        with test.step("Verify last day retention (23 snapshots)"):
            assert counts['day'] == 23, f"Expected 23, got {counts['day']}"

        with test.step("Verify last week retention (6 snapshots)"):
            assert counts['week'] == 6, f"Expected 6, got {counts['week']}"

        with test.step("Verify last month retention (3 snapshots)"):
            assert counts['month'] == 3, f"Expected 3, got {counts['month']}"

        with test.step("Verify last year retention (11 snapshots)"):
            assert counts['year'] == 11, f"Expected 11, got {counts['year']}"

        with test.step("Verify older than 1 year retention (1 snapshot)"):
            assert counts['older'] == 1, f"Expected 1, got {counts['older']}"

    test.succeed()
