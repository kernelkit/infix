#!/usr/bin/env python3
"""Extract all XPaths from YANG modules in a directory.

Outputs a CSV with columns: module, keyword, xpath
XPaths use NETCONF format: /module:top-container/sub/...
"""

import os
import sys
import csv
from pyang import context, repository


def build_xpath(node):
    """Build a NETCONF-style xpath for a schema node."""
    parts = []
    module_name = None

    while node is not None:
        if hasattr(node, "arg") and node.arg:
            parts.append(node.arg)
        parent = getattr(node, "parent", None)
        if parent is None or parent.keyword == "module":
            if parent is not None:
                module_name = parent.arg
            break
        node = parent

    reversed_parts = list(reversed(parts))
    if module_name and reversed_parts:
        reversed_parts[0] = f"{module_name}:{reversed_parts[0]}"

    return "/" + "/".join(reversed_parts)


def collect_xpaths(stmt, results):
    schema_keywords = {
        "container",
        "list",
        "leaf",
        "leaf-list",
        "choice",
        "case",
        "rpc",
        "action",
        "notification",
        "anyxml",
        "anydata",
    }

    # config false (operational state) is inherited; pyang resolves the
    # effective status into i_config during validation. Only skip when it is
    # explicitly False so rpc/action/notification (i_config is None) are kept.
    if stmt.keyword in schema_keywords and getattr(stmt, "i_config", None) is not False:
        results.append({
            "module": stmt.i_module.arg,
            "keyword": stmt.keyword,
            "xpath": build_xpath(stmt),
        })

    for child in getattr(stmt, "i_children", []):
        collect_xpaths(child, results)


def load_yang_modules(yang_dir):
    repo = repository.FileRepository(yang_dir)
    ctx = context.Context(repo)

    for root, _, files in os.walk(yang_dir):
        for file in sorted(files):
            if file.endswith(".yang") and "@" not in file:
                filepath = os.path.join(root, file)
                with open(filepath, "r", encoding="utf-8") as f:
                    text = f.read()
                module = ctx.add_module(file, text)
                if module is None:
                    print(f"Warning: failed to load {file}", file=sys.stderr)

    ctx.validate()
    return ctx


def export_xpaths(yang_dir, output_csv):
    ctx = load_yang_modules(yang_dir)

    results = []
    for module in ctx.modules.values():
        for child in getattr(module, "i_children", []):
            collect_xpaths(child, results)

    out_dir = os.path.dirname(output_csv)
    if out_dir:
        os.makedirs(out_dir, exist_ok=True)

    with open(output_csv, "w", newline="", encoding="utf-8") as csvfile:
        writer = csv.DictWriter(csvfile, fieldnames=["module", "keyword", "xpath"])
        writer.writeheader()
        for row in sorted(results, key=lambda x: (x["module"], x["xpath"])):
            writer.writerow(row)

    print(f"Extracted {len(results)} XPaths to {output_csv}")
    return len(results)


if __name__ == "__main__":
    if len(sys.argv) < 3:
        print(f"Usage: {sys.argv[0]} <yang_dir> <output_csv>")
        sys.exit(1)

    export_xpaths(sys.argv[1], sys.argv[2])
