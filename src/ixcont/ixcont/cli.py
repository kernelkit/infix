"""ixcont: Finit hooks, YANG actions and the admin CLI for Infix containers.

Finit's container@.conf template calls this as pre: (setup), post: and
cleanup: script.  confd calls it for the YANG actions and the container
network hooks, klish for the admin-exec commands.
"""
import argparse
import base64
import hashlib
import json
import os
import shutil
import subprocess
import sys
import syslog
import time

from . import bundle, image as images, net, oci, paths
from .state import (ContainerError, all_states, conf_names, cond_name, err, initctl,
                    load_conf, load_state, log, pid, running, save_state, sh, update_state)

RESTART_LIMIT = 10
STOP_TIMEOUT = 30


# --- runtime directory ----------------------------------------------------

def _mounted(path):
    with open("/proc/self/mounts") as f:
        return any(line.split()[1] == path for line in f)


def _umount(path):
    for _ in range(5):
        if not _mounted(path):
            return
        sh("umount", path, check=False, quiet=True)
        time.sleep(0.2)
    if _mounted(path):
        sh("umount", "-l", path, check=False)


def _mount_overlay(rt, lower):
    rootfs = os.path.join(rt, "rootfs")
    upper, work = os.path.join(rt, "upper"), os.path.join(rt, "work")
    for d in (rootfs, upper, work):
        os.makedirs(d, exist_ok=True)
    sh("mount", "-t", "overlay", "overlay",
       "-o", f"lowerdir={lower},upperdir={upper},workdir={work}", rootfs)


def _etc_files(rt, cfg, hostname, networks):
    etc = os.path.join(rt, "etc")
    os.makedirs(etc, exist_ok=True)
    network = cfg.get("network", {})

    hosts = "127.0.0.1\tlocalhost\n::1\tlocalhost ip6-localhost\n"
    for n in networks or []:
        for addr in n.get("addresses", []):
            hosts += f"{addr}\t{hostname}\n"
            break
    with open(os.path.join(etc, "hosts"), "w") as f:
        f.write(hosts)
    with open(os.path.join(etc, "hostname"), "w") as f:
        f.write(hostname + "\n")

    resolv = ""
    if network.get("dns"):
        resolv = "".join(f"nameserver {srv}\n" for srv in network["dns"])
    else:
        try:
            with open("/etc/resolv.conf") as f:
                resolv = "".join(line for line in f if line.startswith("nameserver"))
        except OSError:
            pass
    if network.get("search"):
        resolv += "search " + " ".join(network["search"]) + "\n"
    with open(os.path.join(etc, "resolv.conf"), "w") as f:
        f.write(resolv)


def _content_files(rt, cfg):
    files = os.path.join(rt, "files")
    os.makedirs(files, exist_ok=True)
    for m in cfg.get("mount", []):
        if "content" not in m:
            continue
        path = os.path.join(files, m["name"])
        mode = int(m.get("mode", "0644"), 8)
        fd = os.open(path, os.O_CREAT | os.O_WRONLY | os.O_TRUNC, 0o600)
        with os.fdopen(fd, "wb") as f:
            f.write(base64.b64decode(m["content"]))
        os.chmod(path, mode)


def _volumes(cfg, rootfs):
    """Named volumes live on flash and are seeded from the image on first use."""
    for v in cfg.get("volume", []):
        path = os.path.join(paths.VOLUMES, f"{cfg['name']}-{v['name']}")
        if os.path.isdir(path):
            continue
        src = oci.inside(rootfs, v["target"], follow=True)
        os.makedirs(paths.VOLUMES, exist_ok=True)
        if os.path.isdir(src):
            shutil.copytree(src, path, symlinks=True)
        else:
            os.makedirs(path)


def _container_id(cfg, image_id):
    doc = json.dumps(cfg, sort_keys=True) + image_id
    return hashlib.sha256(doc.encode()).hexdigest()


def _teardown_runtime(name, st):
    sh("crun", "delete", "-f", name, check=False, quiet=True)
    rt = paths.rundir(name)
    _umount(os.path.join(rt, "rootfs"))
    net.teardown(name, (st or {}).get("networks"))
    net.apply_nft()


# --- Finit hooks ----------------------------------------------------------

def setup(name):
    cfg = load_conf(name)
    st = load_state(name) or {}
    rt = paths.rundir(name)
    marker = os.path.join(rt, "upgrade")
    force = os.path.exists(marker)

    _teardown_runtime(name, st)
    for d in ("upper", "work", "files", "etc"):
        shutil.rmtree(os.path.join(rt, d), ignore_errors=True)

    img = images.resolve(cfg["image"], cfg.get("checksum"), force=force)
    _mount_overlay(rt, img.rootfs)
    rootfs = os.path.join(rt, "rootfs")

    _volumes(cfg, rootfs)
    _content_files(rt, cfg)
    networks = net.setup(cfg)
    hostname = cfg.get("hostname") or name
    _etc_files(rt, cfg, hostname, networks)

    netns = net.netns_path(name) if networks is not None else None
    config = bundle.build(cfg, img, rt, netns)
    with open(os.path.join(rt, "config.json"), "w") as f:
        json.dump(config, f, indent=2)

    state = {
        "name": name,
        "id": _container_id(cfg, img.id),
        "image": cfg["image"],
        "image-id": img.id,
        "image-path": img.path,
        "hostname": hostname,
        "cmdline": " ".join(config["process"]["args"]),
        "status": "created",
        "created": time.time(),
        "networks": networks,
        "publish": cfg.get("network", {}).get("publish", []) if networks is not None else [],
        "cgroup": paths.cgroup(name),
        "restart-policy": cfg.get("restart-policy", "always"),
        "restarts": st.get("restarts", 0),
    }
    save_state(name, state)
    net.apply_nft()
    if force:
        os.unlink(marker)
        images.prune({s["image-path"] for s in all_states()})
    log(f"{name}: created from {img.path}")


def _remove(name):
    _teardown_runtime(name, load_state(name))
    shutil.rmtree(paths.rundir(name), ignore_errors=True)


def post(name):
    """Container process exited: tear down and apply the restart policy."""
    if not os.path.exists(paths.conf(name)):
        _remove(name)
        return

    st = load_state(name) or {}
    _teardown_runtime(name, st)

    stopping = os.path.join(paths.rundir(name), "stopping")
    if os.path.exists(stopping):
        os.unlink(stopping)
        update_state(name, status="stopped", pid=0)
        return

    code = os.environ.get("EXIT_CODE", "exited")
    status = os.environ.get("EXIT_STATUS", "")
    update_state(name, status=f"exited ({status})" if code == "exited" else f"{code} ({status})", pid=0)

    policy = st.get("restart-policy", "always")
    if policy == "never":
        log(f"{name}: exited, restart policy never")
        initctl("cond", "clear", cond_name(name))
    elif policy == "retry":
        restarts = st.get("restarts", 0) + 1
        update_state(name, restarts=restarts)
        if restarts >= RESTART_LIMIT:
            err(f"{name}: giving up after {restarts} restarts")
            initctl("cond", "clear", cond_name(name))


def cleanup(name):
    """Container removed: nothing of it may remain.

    Called by Finit when the service is removed while it runs, and by
    confd for a deleted container, which may have stopped long before.
    One that still runs is being stopped by Finit, post() then removes
    it.  The usr/container-NAME condition is confd's: a re-enable may
    already have set it again while this runs, so it is left alone here.
    """
    if not running(name):
        _remove(name)


def poststart():
    """OCI poststart hook: crun passes the container state on stdin."""
    try:
        st = json.load(sys.stdin)
    except ValueError:
        return
    name = st.get("id")
    if name and os.path.isfile(paths.state(name)):
        update_state(name, status="running", pid=int(st.get("pid", 0)), started=time.time())


def hook():
    """Called by Finit without arguments, dispatch on the script type."""
    kind = os.environ.get("SERVICE_SCRIPT_TYPE")
    name = os.environ.get("SERVICE_ID")
    if not kind or not name:
        return False
    try:
        {"pre": setup, "post": post, "cleanup": cleanup}[kind](name)
    except KeyError:
        return False
    return True


# --- actions --------------------------------------------------------------

def _wait(name, state, timeout):
    for _ in range(timeout * 5):
        if running(name) == state:
            return True
        time.sleep(0.2)
    return False


def start(name):
    rt = paths.rundir(name)
    os.makedirs(rt, exist_ok=True)
    stopping = os.path.join(rt, "stopping")
    if os.path.exists(stopping):
        os.unlink(stopping)
    update_state(name, restarts=0)
    initctl("cond", "set", cond_name(name))
    initctl("start", f"container:{name}")


def stop(name):
    rt = paths.rundir(name)
    os.makedirs(rt, exist_ok=True)
    with open(os.path.join(rt, "stopping"), "w"):
        pass
    initctl("cond", "clear", cond_name(name))
    initctl("stop", f"container:{name}")
    if not _wait(name, False, STOP_TIMEOUT):
        raise ContainerError(f"{name}: timed out waiting for container to stop")


def restart(name):
    if running(name):
        stop(name)
    start(name)


def upgrade(name, reset_volumes=False):
    cfg = load_conf(name)
    print(f">> Upgrading container {name} from image {cfg['image']} ...")
    if running(name):
        print(">> Stopping container ... ", end="", flush=True)
        stop(name)
        print("done")

    if reset_volumes:
        print(">> Resetting named volumes (all configuration will be lost): ", end="")
        for v in cfg.get("volume", []):
            path = os.path.join(paths.VOLUMES, f"{name}-{v['name']}")
            if os.path.isdir(path):
                print(v["name"], end=" ")
                shutil.rmtree(path)
        print("done")

    os.makedirs(paths.rundir(name), exist_ok=True)
    with open(os.path.join(paths.rundir(name), "upgrade"), "w"):
        pass
    print(">> Recreating container ...")
    start(name)
    if not _wait(name, True, 60):
        raise ContainerError(f"{name}: container did not come up after upgrade, see log")
    print(f">> Container {name} upgraded successfully.")


# --- admin CLI ------------------------------------------------------------

def exec_(name, cmd, tty):
    args = ["crun", "exec"] + (["-t"] if tty else []) + [name] + cmd
    os.execvp("crun", args)


def shell(name, cmd):
    if cmd:
        exec_(name, ["sh", "-c", " ".join(cmd)], False)
    exec_(name, ["sh", "-l"], True)


def run(ref, cmd, detach=False):
    """Run an image interactively, with host networking, nothing persists."""
    name = f"run-{os.getpid()}"
    rt = paths.rundir(name)
    cfg = {"name": name, "network": {"host": True}}
    if cmd:
        cfg["command"] = " ".join(cmd)
    img = images.resolve(ref)
    try:
        _mount_overlay(rt, img.rootfs)
        _etc_files(rt, cfg, name, None)
        config = bundle.build(cfg, img, rt, None)
        config["process"]["terminal"] = sys.stdin.isatty() and not detach
        with open(os.path.join(rt, "config.json"), "w") as f:
            json.dump(config, f)
        argv = ["crun", "run", "--bundle", rt]
        if detach:
            argv.append("--detach")
        argv.append(name)
        return subprocess.call(argv)
    finally:
        if not detach:
            sh("crun", "delete", "-f", name, check=False, quiet=True)
            _umount(os.path.join(rt, "rootfs"))
            shutil.rmtree(rt, ignore_errors=True)


def find(ifname, inner_pid=None):
    """PID of the container holding ifname, or its name inside the container."""
    if inner_pid:
        name = net.inner_ifname(inner_pid, ifname)
        if name:
            print(name)
        return 0
    st = net.owner(ifname)
    if not st:
        return 1
    p = pid(st["name"])
    if not p:
        return 1
    print(p)
    return 0


def list_(what, all_=False):
    if what == "images":
        for d in images.builtin():
            print(os.path.join(paths.BUILTIN, d))
        for d in images.cached():
            print(os.path.join(paths.IMAGES, d))
    elif what == "oci":
        for d in (paths.BUILTIN, paths.IMAGES):
            if os.path.isdir(d):
                for entry in sorted(os.listdir(d)):
                    print(os.path.join(d, entry))
    else:
        for name in conf_names():
            if all_ or running(name):
                print(name)


def _du(path):
    res = sh("du", "-sk", path, check=False, quiet=True)
    return int(res.stdout.split()[0]) if res.returncode == 0 and res.stdout else 0


def show(what):
    if what == "images":
        print(f"{'IMAGE':<60} {'ID':<14} SIZE")
        for d in images.builtin() + images.cached():
            base = paths.BUILTIN if d in images.builtin() else paths.IMAGES
            path = os.path.join(base, d)
            with open(os.path.join(path, "digest")) as f:
                digest = f.read().strip()
            print(f"{path:<60} {digest[:12]:<14} {_du(path)}K")
    elif what == "volumes":
        print(f"{'VOLUME':<30} CONTAINER")
        if os.path.isdir(paths.VOLUMES):
            names = conf_names()
            for v in sorted(os.listdir(paths.VOLUMES)):
                owner = next((n for n in names if v.startswith(n + "-")), "")
                print(f"{v:<30} {owner}")
    else:
        print(f"{'NAME':<24} {'STATUS':<16} {'IMAGE':<40} NETWORK")
        for st in all_states():
            status = "running" if running(st["name"]) else st.get("status", "")
            nets = ", ".join(n["name"] for n in st.get("networks") or [])
            print(f"{st['name']:<24} {status:<16} {st.get('image', ''):<40} {nets}")


def prune():
    keep = {s["image-path"] for s in all_states()}
    for path in images.prune(keep):
        print(f"Removed image {path}")
    names = conf_names()
    if os.path.isdir(paths.VOLUMES):
        for v in sorted(os.listdir(paths.VOLUMES)):
            if not any(v.startswith(n + "-") for n in names):
                shutil.rmtree(os.path.join(paths.VOLUMES, v), ignore_errors=True)
                print(f"Removed volume {v}")


def usage():
    for sub in ("images", "volumes"):
        print(f"{sub:<10} {_du(os.path.join(paths.VAR, sub))}K")


def _cgroup_read(name, fn):
    try:
        with open(os.path.join(paths.CGROUP_ROOT, paths.cgroup(name), fn)) as f:
            return f.read().strip()
    except OSError:
        return ""


def stat():
    print(f"{'NAME':<24} {'MEM (KiB)':>10} {'CPU (s)':>10} {'PIDS':>6}")
    for name in conf_names():
        if not running(name):
            continue
        mem = int(_cgroup_read(name, "memory.current") or 0) // 1024
        cpu = 0
        for line in _cgroup_read(name, "cpu.stat").splitlines():
            if line.startswith("usage_usec"):
                cpu = int(line.split()[1]) / 1e6
        pids = _cgroup_read(name, "pids.current") or "0"
        print(f"{name:<24} {mem:>10} {cpu:>10.2f} {pids:>6}")


def gateway(subnet):
    """Address of the container bridge in subnet, first host with prefix."""
    import ipaddress
    net = ipaddress.ip_network(subnet, strict=False)
    print(f"{next(net.hosts())}/{net.prefixlen}")


def netchange(ifname):
    """confd changed a container network, restart the containers using it."""
    for name in net.users(ifname):
        initctl("touch", f"container@{name}")


def netdel(ifname):
    """confd removes a container network: return the interface, then restart.

    The container may be going away in the same commit, with Finit's
    cleanup racing this; a state file must not be recreated for it.
    """
    st = net.owner(ifname)
    if st and os.path.exists(net.netns_path(st["name"])):
        st = net.detach(st, ifname)
        if os.path.isfile(paths.state(st["name"])):
            save_state(st["name"], st)
    netchange(ifname)


def reconcile():
    """Boot: drop what no configured container refers to."""
    for d in (paths.IMAGES, paths.VOLUMES, paths.RUN, paths.NETCONF):
        os.makedirs(d, exist_ok=True)
    names = conf_names()
    for entry in os.listdir(paths.RUN):
        path = os.path.join(paths.RUN, entry)
        if os.path.isdir(path) and entry not in ("net", "ipam") and entry not in names:
            _teardown_runtime(entry, load_state(entry))
            shutil.rmtree(path, ignore_errors=True)
    keep = set()
    for name in names:
        try:
            path = images.cached_path(load_conf(name)["image"])
        except ContainerError:
            continue
        if path:
            keep.add(path)
    for path in images.prune(keep):
        log(f"Removed unreferenced image {path}")
    net.apply_nft()


# --- entry ----------------------------------------------------------------

def _parser():
    p = argparse.ArgumentParser(prog="ixcont", description="Infix container runtime")
    p.add_argument("-a", "--all", action="store_true", help="include stopped containers")
    p.add_argument("-R", "--reset-volumes", action="store_true", help="upgrade: reset named volumes")
    p.add_argument("--net", metavar="IFNAME", help="find: interface to look up")
    p.add_argument("-d", "--detach", action="store_true", help="run: detach")
    p.add_argument("-c", "--creds", metavar="USER[:PASS]", help="pull/load: registry credentials")
    sub = p.add_subparsers(dest="cmd")

    for cmd, help_ in (("setup", "Finit pre: create bundle"), ("post", "Finit post: tear down"),
                       ("cleanup", "Finit cleanup: remove all state"), ("start", "start container"),
                       ("stop", "stop container"), ("restart", "restart container"),
                       ("upgrade", "re-fetch image and recreate"), ("pid", "init PID of container")):
        sub.add_parser(cmd, help=help_).add_argument("name")

    s = sub.add_parser("exec", help="run command in container")
    s.add_argument("name")
    s.add_argument("command", nargs=argparse.REMAINDER)
    s = sub.add_parser("shell", help="shell in container")
    s.add_argument("name")
    s.add_argument("command", nargs=argparse.REMAINDER)
    s = sub.add_parser("run", help="run image interactively")
    s.add_argument("image")
    s.add_argument("command", nargs=argparse.REMAINDER)
    s = sub.add_parser("find", help="container PID for --net IFNAME, or 'ifname PID'")
    s.add_argument("args", nargs="*")
    s = sub.add_parser("list", help="list containers, images or oci archives")
    s.add_argument("what", nargs="?", default="containers")
    s = sub.add_parser("show", help="show containers, images or volumes")
    s.add_argument("what", nargs="?", default="containers")
    s = sub.add_parser("load", help="load OCI archive or URL as image")
    s.add_argument("src")
    s.add_argument("name", nargs="?")
    sub.add_parser("pull", help="pull image from a registry").add_argument("image")
    sub.add_parser("prune", help="remove unused images and volumes")
    sub.add_parser("usage", help="disk usage")
    sub.add_parser("stat", help="resource usage of running containers")
    sub.add_parser("gateway", help="bridge address for a subnet").add_argument("subnet")
    sub.add_parser("netchange", help="container network changed").add_argument("ifname")
    sub.add_parser("netdel", help="container network removed").add_argument("ifname")
    sub.add_parser("reconcile", help="drop state of unconfigured containers")
    sub.add_parser("poststart", help="OCI poststart hook")
    return p


def main(argv=None):
    syslog.openlog("container", syslog.LOG_PID, syslog.LOG_LOCAL1)
    argv = sys.argv[1:] if argv is None else argv
    if not argv and hook():
        return 0

    args = _parser().parse_args(argv)
    try:
        if args.cmd == "setup":
            setup(args.name)
        elif args.cmd == "post":
            post(args.name)
        elif args.cmd == "cleanup":
            cleanup(args.name)
        elif args.cmd == "start":
            start(args.name)
        elif args.cmd == "stop":
            stop(args.name)
        elif args.cmd == "restart":
            restart(args.name)
        elif args.cmd == "upgrade":
            upgrade(args.name, args.reset_volumes)
        elif args.cmd == "pid":
            p = pid(args.name)
            if not p:
                return 1
            print(p)
        elif args.cmd == "exec":
            exec_(args.name, args.command, sys.stdin.isatty())
        elif args.cmd == "shell":
            shell(args.name, args.command)
        elif args.cmd == "run":
            return run(args.image, args.command, args.detach)
        elif args.cmd == "find":
            if not args.net:
                raise ContainerError("find needs --net IFNAME")
            if len(args.args) == 2 and args.args[0] == "ifname":
                return find(args.net, int(args.args[1]))
            return find(args.net)
        elif args.cmd == "list":
            list_(args.what, args.all)
        elif args.cmd == "show":
            show(args.what)
        elif args.cmd == "load":
            img = images.load(args.src, args.name, args.creds)
            print(f"{img.path} {img.id[:12]}")
        elif args.cmd == "pull":
            ref = args.image if args.image.startswith(images.REGISTRY) else images.REGISTRY + args.image
            img = images.resolve(ref, force=True, creds=args.creds)
            print(f"{img.path} {img.id[:12]}")
        elif args.cmd == "prune":
            prune()
        elif args.cmd == "usage":
            usage()
        elif args.cmd == "stat":
            stat()
        elif args.cmd == "gateway":
            gateway(args.subnet)
        elif args.cmd == "netchange":
            netchange(args.ifname)
        elif args.cmd == "netdel":
            netdel(args.ifname)
        elif args.cmd == "reconcile":
            reconcile()
        elif args.cmd == "poststart":
            poststart()
        else:
            _parser().print_help()
            return 1
    except ContainerError as e:
        err(str(e))
        print(f"ixcont: {e}", file=sys.stderr)
        return 1
    except Exception as e:  # noqa: BLE001 - anything else is a bug, name it in the log
        err(f"{args.cmd or 'hook'}: {type(e).__name__}: {e}")
        raise
    return 0


if __name__ == "__main__":
    sys.exit(main())
