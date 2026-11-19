#!/usr/bin/env python3
"""Add a Test System section to the test report

Reads the AsciiDoc report 9pm writes and the topology the tests ran on,
and writes the report with the name of the test system on the cover and
a section with the test server, the topology and the devices in it.
"""
import argparse
import json
import os
import re
import shutil
import subprocess
import sys

ANCHOR = "=== Test Overview"
TITLE = "= Test Report"


def graph(dot):
    """The topology as graphviz lays it out, and the devices under test in it"""
    out = subprocess.run(["dot", "-Tjson0", dot], check=True, text=True,
                         stdout=subprocess.PIPE).stdout
    topo = json.loads(out)
    nodes = {n["_gvid"]: n for n in topo.get("objects", [])}
    duts = {gvid: n for gvid, n in nodes.items()
            if "infix" in n.get("provides", "").split()}
    return topo, nodes, duts


def ports(node):
    """Port names of a record shaped node"""
    return re.findall(r"<([^>]+)>", node.get("label", ""))


def links(topo, nodes, duts):
    """Per device: links to the test server and to other devices"""
    count = {gvid: [0, 0] for gvid in duts}
    for edge in topo.get("edges", []):
        ends = (edge["tail"], edge["head"])
        for end, peer in (ends, ends[::-1]):
            if end not in duts:
                continue
            if peer in duts:
                count[end][1] += 1
            elif "controller" in nodes[peer].get("provides", "").split():
                count[end][0] += 1
    return count


def picture(dot, outdir):
    """The SVG kept beside the topology, or one rendered from it"""
    svg = os.path.join(outdir, "topology.svg")
    beside = re.sub(r"\.dot(\.in)?$", ".svg", dot)
    if not os.path.exists(beside) or beside == dot:
        subprocess.run(["dot", "-Tsvg", "-o", svg, dot], check=True)
    elif os.path.abspath(beside) != svg:
        shutil.copyfile(beside, svg)
    return svg


def section(name, server, svg, topo, nodes, duts):
    count = links(topo, nodes, duts)
    to_server = sum(c[0] for c in count.values())
    between = sum(c[1] for c in count.values()) // 2

    lines = [
        "=== Test System",
        "",
        '[cols="1h,2", width=50%]',
        "|===",
        f"| Name    | {name}",
        f"| Server  | {server}",
        f"| Devices | {len(duts)}",
        f"| Links   | {to_server} to the server, {between} between devices",
        "|===",
        "",
        '[cols="2,5,1,1,1", options="header"]',
        "|===",
        "| Device | Provides | Ports | To server | To devices",
    ]
    for gvid, node in sorted(duts.items(), key=lambda item: item[1]["name"]):
        lines.append(f"| {node['name']} | {node.get('provides', '')} | {len(ports(node))} "
                     f"| {count[gvid][0]} | {count[gvid][1]}")
    lines += ["|===", "",
              f"image::{svg}[Physical topology, pdfwidth=100%, align=center]", "",
              "<<<", ""]
    return lines


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("report", help="report.adoc as written by 9pm")
    ap.add_argument("-t", "--topology", required=True, help="topology the tests ran on")
    ap.add_argument("-s", "--server", default=os.uname().nodename, help="name of the test server")
    ap.add_argument("-o", "--output", required=True, help="report to write, the SVG lands beside it")
    args = ap.parse_args()

    with open(args.report, encoding="utf-8") as f:
        report = f.read().split("\n")
    if ANCHOR not in report or TITLE not in report:
        sys.exit(f"{args.report}: not a report this script knows, '{ANCHOR}' or '{TITLE}' is missing")

    missing = None
    if not os.path.exists(args.topology):
        missing = f"no topology {args.topology}"
    elif not shutil.which("dot"):
        missing = "graphviz is not installed"
    if missing:
        print(f"warning: {missing}, the report gets no Test System section", file=sys.stderr)
        shutil.copyfile(args.report, args.output)
        return

    outdir = os.path.dirname(os.path.abspath(args.output))
    topo, nodes, duts = graph(args.topology)
    name = topo.get("name", os.path.basename(args.topology))
    svg = picture(args.topology, outdir)

    at = report.index(TITLE)
    report[at] = f"{TITLE}: {name}"
    at = report.index(ANCHOR)
    report[at:at] = section(name, args.server, svg, topo, nodes, duts)

    with open(args.output, "w", encoding="utf-8") as f:
        f.write("\n".join(report))


if __name__ == "__main__":
    main()
