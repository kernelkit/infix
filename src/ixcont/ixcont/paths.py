"""Where ixcont keeps things.

Per-container runtime state lives in tmpfs and is rebuilt on every start.
Only images and named volumes live on flash.
"""
import os

RUN = "/run/containers"               # confd writes <name>.json here
NETCONF = os.path.join(RUN, "net")    # confd writes <ifname>.json here
NETNS = "/run/netns"

VAR = "/var/lib/containers"
IMAGES = os.path.join(VAR, "images")  # fetched and converted images
VOLUMES = os.path.join(VAR, "volumes")
BUILTIN = "/lib/oci"                  # images in the system image

CGROUP_ROOT = "/sys/fs/cgroup"


def conf(name):
    return os.path.join(RUN, name + ".json")


def rundir(name):
    return os.path.join(RUN, name)


def state(name):
    return os.path.join(rundir(name), "state.json")


def cgroup(name):
    return f"system/container@{name}/payload"
