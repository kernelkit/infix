"""Container configuration (from confd) and runtime state (ours)."""
import json
import os
import subprocess
import syslog

from . import paths


def log(msg, prio=syslog.LOG_NOTICE):
    syslog.syslog(prio, msg)


def err(msg):
    syslog.syslog(syslog.LOG_ERR, msg)


def dbg(msg):
    syslog.syslog(syslog.LOG_DEBUG, msg)


class ContainerError(Exception):
    pass


def _read_json(path):
    with open(path) as f:
        return json.load(f)


def _write_json(path, data):
    tmp = path + ".tmp"
    with open(tmp, "w") as f:
        json.dump(data, f, indent=2)
        f.write("\n")
    os.rename(tmp, path)


def load_conf(name):
    """The container's configuration as confd wrote it, the YANG list entry."""
    try:
        doc = _read_json(paths.conf(name))
    except FileNotFoundError:
        raise ContainerError(f"{name}: no configuration in {paths.RUN}")

    entries = doc.get("infix-containers:container")
    if entries is None:
        entries = doc.get("infix-containers:containers", {}).get("container", [])
    for entry in entries:
        if entry.get("name") == name:
            return entry
    raise ContainerError(f"{name}: configuration file does not describe this container")


def conf_names():
    names = []
    for entry in os.listdir(paths.RUN) if os.path.isdir(paths.RUN) else []:
        if entry.endswith(".json"):
            names.append(entry[:-5])
    return sorted(names)


def load_state(name):
    try:
        return _read_json(paths.state(name))
    except (FileNotFoundError, ValueError):
        return None


def save_state(name, data):
    os.makedirs(paths.rundir(name), exist_ok=True)
    _write_json(paths.state(name), data)


def update_state(name, **fields):
    """Merge fields into the state, unless the container has been removed."""
    if not os.path.isdir(paths.rundir(name)):
        return None
    data = load_state(name) or {"name": name}
    data.update(fields)
    save_state(name, data)
    return data


def all_states():
    states = []
    for name in conf_names():
        st = load_state(name)
        if st:
            states.append(st)
    return states


def crun(*args, check=True, **kw):
    return subprocess.run(["crun", *args], check=check, text=True,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE, **kw)


def crun_state(name):
    """crun's view of the container: status (created, running, stopped) and pid."""
    res = crun("state", name, check=False)
    if res.returncode:
        return None
    try:
        return json.loads(res.stdout)
    except ValueError:
        return None


def pid(name):
    st = crun_state(name)
    if st and st.get("status") == "running":
        return int(st.get("pid", 0))
    return 0


def running(name):
    return pid(name) > 0


def initctl(*args):
    subprocess.run(["initctl", "-bnq", *args], check=False)


def cond_name(name):
    return "container-" + name


def sh(*args, check=True, quiet=False):
    """Run a command, log stderr on failure."""
    res = subprocess.run(list(args), text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if res.returncode and not quiet:
        err(f"{' '.join(args)}: {res.stderr.strip()}")
    if res.returncode and check:
        raise ContainerError(f"{args[0]} failed: {res.stderr.strip()}")
    return res
