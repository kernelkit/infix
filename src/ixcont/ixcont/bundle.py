"""Build the OCI runtime config.json for crun from image config and YANG."""
import glob
import os
import shlex

from . import oci, paths
from .state import ContainerError

# The same hardening podman and Docker apply to an unprivileged container:
# podman's default capabilities, masked and read-only paths.  crun only
# does what the config says, so without these an unprivileged container
# has none.  Copied lists rot as upstream learns of new kernel interfaces;
# they could be taken from `crun spec` instead, but the real fix is to not
# run the container as root (user namespaces), which Infix never has.
DEFAULT_CAPS = [
    "CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_FOWNER", "CAP_FSETID", "CAP_KILL",
    "CAP_NET_BIND_SERVICE", "CAP_SETFCAP", "CAP_SETGID", "CAP_SETPCAP",
    "CAP_SETUID", "CAP_SYS_CHROOT",
]

ALL_CAPS = DEFAULT_CAPS + [
    "CAP_AUDIT_CONTROL", "CAP_AUDIT_READ", "CAP_AUDIT_WRITE", "CAP_BLOCK_SUSPEND",
    "CAP_BPF", "CAP_CHECKPOINT_RESTORE", "CAP_DAC_READ_SEARCH", "CAP_IPC_LOCK",
    "CAP_IPC_OWNER", "CAP_LEASE", "CAP_LINUX_IMMUTABLE", "CAP_MAC_ADMIN",
    "CAP_MAC_OVERRIDE", "CAP_MKNOD", "CAP_NET_ADMIN", "CAP_NET_BROADCAST",
    "CAP_NET_RAW", "CAP_PERFMON", "CAP_SYS_ADMIN", "CAP_SYS_BOOT", "CAP_SYS_MODULE",
    "CAP_SYS_NICE", "CAP_SYS_PACCT", "CAP_SYS_PTRACE", "CAP_SYS_RAWIO",
    "CAP_SYS_RESOURCE", "CAP_SYS_TIME", "CAP_SYS_TTY_CONFIG", "CAP_SYSLOG",
    "CAP_WAKE_ALARM",
]

MASKED_PATHS = [
    "/proc/acpi", "/proc/kcore", "/proc/keys", "/proc/latency_stats",
    "/proc/sched_debug", "/proc/scsi", "/proc/timer_list", "/proc/timer_stats",
    "/sys/dev/block", "/sys/devices/virtual/powercap", "/sys/firmware",
    "/sys/fs/selinux",
]

READONLY_PATHS = [
    "/proc/asound", "/proc/bus", "/proc/fs", "/proc/irq", "/proc/sys",
    "/proc/sysrq-trigger",
]

DEFAULT_PATH = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"


def _tmpfs(dest, mode):
    return {"destination": dest, "type": "tmpfs", "source": "tmpfs",
            "options": ["nosuid", "nodev", f"mode={mode}"]}


def _bind(src, dest, ro=True, recursive=True):
    opts = ["rbind" if recursive else "bind"]
    if ro:
        opts.append("ro")
    return {"destination": dest, "type": "bind", "source": src, "options": opts}


def _lookup(rootfs, dbfile, key):
    try:
        with open(oci.inside(rootfs, "etc/" + dbfile, follow=True)) as f:
            for line in f:
                fields = line.rstrip("\n").split(":")
                if len(fields) > 2 and fields[0] == key:
                    return int(fields[2])
    except (OSError, ValueError):
        pass
    return None


def _user(image, rootfs):
    spec = str(image.config.get("User") or "0")
    user, _, group = spec.partition(":")
    uid = int(user) if user.isdigit() else _lookup(rootfs, "passwd", user)
    if uid is None:
        raise ContainerError(f"unknown user {user} in image")
    if group:
        gid = int(group) if group.isdigit() else _lookup(rootfs, "group", group)
    else:
        gid = _lookup_gid(rootfs, uid)
    return {"uid": uid, "gid": gid if gid is not None else 0}


def _lookup_gid(rootfs, uid):
    try:
        with open(oci.inside(rootfs, "etc/passwd", follow=True)) as f:
            for line in f:
                fields = line.rstrip("\n").split(":")
                if len(fields) > 3 and fields[2] == str(uid):
                    return int(fields[3])
    except (OSError, ValueError):
        pass
    return 0 if uid == 0 else None


def _args(image, cfg):
    entrypoint = image.config.get("Entrypoint") or []
    command = cfg.get("command")
    if command:
        cmd = shlex.split(command)
    else:
        cmd = image.config.get("Cmd") or []
    args = list(entrypoint) + list(cmd)
    if not args:
        raise ContainerError("image has no entrypoint or command")
    return args


def _env(image, cfg, uid):
    env = {}
    for item in image.config.get("Env") or []:
        key, _, val = item.partition("=")
        env[key] = val
    for item in cfg.get("env", []):
        env[item["key"]] = item["value"]
    env.setdefault("PATH", DEFAULT_PATH)
    env.setdefault("HOME", "/root" if uid == 0 else "/")
    env.setdefault("TERM", "xterm")
    return [f"{k}={v}" for k, v in env.items()]


def _capabilities(cfg, uid):
    """As podman: root holds the allowed set, any other user only what the
    configuration adds, and nothing is inherited across an exec as root."""
    caps = cfg.get("capabilities", {})
    dropped = {"CAP_" + cap.upper() for cap in caps.get("drop", [])}
    added = {"CAP_" + cap.upper() for cap in caps.get("add", [])} - dropped
    if cfg.get("privileged"):
        allowed = set(ALL_CAPS) - dropped
    else:
        allowed = (set(DEFAULT_CAPS) | added) - dropped
    held = sorted(allowed) if uid == 0 else sorted(added)
    kept = [] if uid == 0 else held
    return {"bounding": sorted(allowed), "effective": held, "permitted": held,
            "inheritable": kept, "ambient": kept}


def _user_mounts(cfg, rt):
    mounts = []
    for m in cfg.get("mount", []):
        ro = m.get("read-only", True)
        target = m["target"]
        if "content" in m:
            mounts.append(_bind(os.path.join(rt, "files", m["name"]), target, ro))
            continue

        src = m["source"]
        if m.get("type", "bind") == "glob":
            for match in sorted(glob.glob(src)):
                mounts.append(_bind(match, os.path.join(target, os.path.basename(match)), ro))
        else:
            mounts.append(_bind(src, target, ro))

    for v in cfg.get("volume", []):
        src = os.path.join(paths.VOLUMES, f"{cfg['name']}-{v['name']}")
        mounts.append(_bind(src, v["target"], ro=False))

    # Parents first, so a volume never hides a file mounted below it
    mounts.sort(key=lambda m: m["destination"].count("/"))
    return mounts


def _resources(cfg, privileged):
    res = {"devices": [{"allow": privileged, "access": "rwm"}]}
    limit = cfg.get("resource-limit", {})
    if "memory" in limit:
        res["memory"] = {"limit": int(limit["memory"]) * 1024}
    if "cpu" in limit:
        res["cpu"] = {"quota": int(limit["cpu"]) * 100, "period": 100000}
    return res


def build(cfg, image, rt, netns=None):
    """Return the config.json document for container cfg running image."""
    name = cfg["name"]
    rootfs = os.path.join(rt, "rootfs")
    privileged = bool(cfg.get("privileged"))
    user = _user(image, rootfs)

    process = {
        "terminal": False,
        "user": user,
        "args": _args(image, cfg),
        "env": _env(image, cfg, user["uid"]),
        "cwd": image.config.get("WorkingDir") or "/",
        "capabilities": _capabilities(cfg, user["uid"]),
        "noNewPrivileges": False,
    }

    mounts = [{"destination": "/proc", "type": "proc", "source": "proc"}]
    if privileged:
        mounts.append(_bind("/dev", "/dev", ro=False))
        mounts.append({"destination": "/sys", "type": "sysfs", "source": "sysfs",
                       "options": ["nosuid", "noexec", "nodev"]})
    else:
        mounts.append({"destination": "/dev", "type": "tmpfs", "source": "tmpfs",
                       "options": ["nosuid", "strictatime", "mode=755", "size=65536k"]})
        mounts.append({"destination": "/dev/pts", "type": "devpts", "source": "devpts",
                       "options": ["nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620", "gid=5"]})
        mounts.append({"destination": "/dev/shm", "type": "tmpfs", "source": "shm",
                       "options": ["nosuid", "noexec", "nodev", "mode=1777", "size=65536k"]})
        mounts.append({"destination": "/dev/mqueue", "type": "mqueue", "source": "mqueue",
                       "options": ["nosuid", "noexec", "nodev"]})
        mounts.append({"destination": "/sys", "type": "sysfs", "source": "sysfs",
                       "options": ["nosuid", "noexec", "nodev", "ro"]})
    mounts.append({"destination": "/sys/fs/cgroup", "type": "cgroup", "source": "cgroup",
                   "options": ["nosuid", "noexec", "nodev", "relatime", "ro" if not privileged else "rw"]})
    mounts += [_tmpfs("/run", "755"), _tmpfs("/tmp", "1777"), _tmpfs("/var/tmp", "1777")]

    for fn in ("hosts", "hostname", "resolv.conf"):
        mounts.append(_bind(os.path.join(rt, "etc", fn), "/etc/" + fn, ro=True, recursive=False))
    if os.path.exists("/etc/localtime"):
        mounts.append(_bind("/etc/localtime", "/etc/localtime", ro=True, recursive=False))
    mounts += _user_mounts(cfg, rt)

    namespaces = [{"type": "pid"}, {"type": "ipc"}, {"type": "uts"}, {"type": "mount"},
                  {"type": "cgroup"}]
    if netns:
        namespaces.append({"type": "network", "path": netns})

    linux = {
        "resources": _resources(cfg, privileged),
        "namespaces": namespaces,
        "cgroupsPath": paths.cgroup(name),
    }
    if not privileged:
        linux["maskedPaths"] = MASKED_PATHS
        linux["readonlyPaths"] = READONLY_PATHS

    return {
        "ociVersion": "1.0.2",
        "process": process,
        "root": {"path": rootfs, "readonly": True},
        "hostname": cfg.get("hostname") or name,
        "mounts": mounts,
        "linux": linux,
        # Records the container PID in our state once crun has started it
        "hooks": {"poststart": [{"path": "/usr/sbin/ixcont", "args": ["ixcont", "poststart"]}]},
    }
