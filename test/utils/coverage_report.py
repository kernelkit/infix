#!/usr/bin/env python3
"""Generate an XPath coverage report after a test run.

Usage:
    coverage_report.py <xpaths_all.csv> <xpath_coverage.log> <output.md>

xpaths_all.csv  - produced by extract_xpaths.py; columns: module,keyword,xpath
xpath_coverage.log - produced by infamy/coverage.py; one xpath per line
output.md       - destination for the markdown report
"""

import csv
import os
import sys
from collections import defaultdict
from datetime import datetime


# Thresholds for colour coding
GREEN_THRESHOLD  = 80   # %
ORANGE_THRESHOLD =  1   # %  (anything >= 1 and < 80 is orange)

# Modules excluded from the report
SKIP_MODULES: set[str] = {
    "infix-if-base",   # submodule of infix-interfaces; only a structural choice node
}


def load_all_xpaths(csv_path: str) -> list[dict]:
    rows = []
    with open(csv_path, newline="", encoding="utf-8") as f:
        for row in csv.DictReader(f):
            if row["module"] not in SKIP_MODULES:
                rows.append(row)
    return rows


def load_tracked_xpaths(log_path: str) -> set[str]:
    tracked: set[str] = set()
    if not os.path.exists(log_path):
        return tracked
    with open(log_path, encoding="utf-8") as f:
        for line in f:
            x = line.strip()
            if x:
                tracked.add(x)
    return tracked


def is_covered(yang_xpath: str, tracked: set[str], module: str,
               xpath_module: dict[str, str]) -> bool:
    """ Return True if yang_xpath was exercised by a test. """
    if f"/{module}:*" in tracked:
        return True
    if yang_xpath in tracked:
        return True

    prefix = yang_xpath + "/"
    for t in tracked:
        if not prefix.startswith(t + "/"):
            continue
        # Determine the defining module of the tracked ancestor
        t_module = xpath_module.get(t)
        if t_module is None:
            # Not in the CSV; infer from the root path segment (e.g. /mod:node → mod)
            seg = t.lstrip("/").split("/")[0]
            t_module = seg.split(":")[0] if ":" in seg else None
        if t_module == module:
            return True

    return False


def build_coverage(all_xpaths: list[dict], tracked: set[str]) -> dict:
    """Return per-module coverage data.

    Returns:
        {module: {"total": int, "covered": int, "rows": [{"xpath", "keyword", "covered"}]}}
    """
    xpath_module: dict[str, str] = {row["xpath"]: row["module"] for row in all_xpaths}
    modules: dict[str, dict] = defaultdict(lambda: {"total": 0, "covered": 0, "rows": []})

    for row in all_xpaths:
        mod   = row["module"]
        xpath = row["xpath"]
        kw    = row["keyword"]
        hit   = is_covered(xpath, tracked, mod, xpath_module)

        modules[mod]["total"]  += 1
        modules[mod]["covered"] += int(hit)
        modules[mod]["rows"].append({"xpath": xpath, "keyword": kw, "covered": hit})

    # Sort rows within each module by xpath
    for mod_data in modules.values():
        mod_data["rows"].sort(key=lambda r: r["xpath"])

    return dict(sorted(modules.items()))


def status_emoji(pct: float) -> str:
    if pct >= GREEN_THRESHOLD:
        return "🟢"
    if pct >= ORANGE_THRESHOLD:
        return "🟠"
    return "🔴"


def pct(covered: int, total: int) -> float:
    return 100.0 * covered / total if total else 0.0


def write_markdown(coverage: dict, output_path: str) -> None:
    total_all   = sum(d["total"]   for d in coverage.values())
    covered_all = sum(d["covered"] for d in coverage.values())
    overall_pct = pct(covered_all, total_all)

    lines: list[str] = []
    lines.append("# XPath Coverage Report\n")
    lines.append(f"Generated: {datetime.now().strftime('%Y-%m-%d')}\n")
    lines.append("")

    # ── Overall summary ──────────────────────────────────────────────────────
    lines.append("## Summary\n")
    lines.append(
        f"{status_emoji(overall_pct)} **Overall: "
        f"{covered_all}/{total_all} XPaths covered "
        f"({overall_pct:.1f}%)**\n"
    )
    lines.append("")

    lines.append("| Status | Module | Covered | Total | % |")
    lines.append("|--------|--------|--------:|------:|--:|")
    for mod, d in sorted(coverage.items()):
        p = pct(d["covered"], d["total"])
        lines.append(
            f"| {status_emoji(p)} | {mod} "
            f"| {d['covered']} | {d['total']} | {p:.1f}% |"
        )
    lines.append("")

    # ── Per-module detail ─────────────────────────────────────────────────────
    lines.append("## Details\n")

    for mod, d in coverage.items():
        p = pct(d["covered"], d["total"])
        lines.append(
            f"### {status_emoji(p)} {mod} "
            f"({d['covered']}/{d['total']} — {p:.1f}%)\n"
        )
        lines.append("| XPath | Keyword | Covered |")
        lines.append("|-------|---------|:-------:|")
        for row in d["rows"]:
            tick = "✅" if row["covered"] else "❌"
            lines.append(f"| `{row['xpath']}` | {row['keyword']} | {tick} |")
        lines.append("")

    with open(output_path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))

    print(f"Coverage report written to {output_path}")


def main() -> None:
    if len(sys.argv) < 4:
        print(f"Usage: {sys.argv[0]} <xpaths_all.csv> <xpath_coverage.log> <output.md>")
        sys.exit(1)

    all_csv    = sys.argv[1]
    cov_log    = sys.argv[2]
    out_md     = sys.argv[3]

    if not os.path.exists(all_csv):
        print(f"Error: XPath list not found: {all_csv}", file=sys.stderr)
        sys.exit(1)

    all_xpaths = load_all_xpaths(all_csv)
    tracked    = load_tracked_xpaths(cov_log)
    coverage   = build_coverage(all_xpaths, tracked)

    write_markdown(coverage, out_md)

    total_all   = sum(d["total"]   for d in coverage.values())
    covered_all = sum(d["covered"] for d in coverage.values())
    print(
        f"Overall: {covered_all}/{total_all} XPaths covered "
        f"({pct(covered_all, total_all):.1f}%)"
    )


if __name__ == "__main__":
    main()
