"""
What a device under test can actually do.

Tests ask here instead of working it out from sysfs and qdisc dumps
themselves, so a rig whose fabric differs skips with a stated reason
rather than failing a measurement that was never meaningful.
"""


class Port:
    """What one interface of a device under test can do.

    What the port is is read once, over ssh, since it cannot change
    while a test runs.  What the port is currently doing is read every
    time, since the test is what changes it.  An ssh transport is only
    needed for the former.
    """

    def __init__(self, target, name, ssh=None):
        self.target = target
        self.name = name
        self.ssh = ssh
        self._switched = None
        self._driver = None

    @property
    def caps(self):
        """The qos capabilities the port reports, {} when it reports none"""
        data = self.target.get_data(
            f"/ietf-interfaces:interfaces/interface[name='{self.name}']"
            "/infix-interfaces:qos/capabilities")
        iface = next(iter(data["interfaces"]["interface"]), {})
        qos = iface.get("qos") or iface.get("infix-interfaces:qos") or {}
        return qos.get("capabilities", {})

    @property
    def switched(self):
        """Whether the port belongs to a switch fabric.

        Such a port forwards most frames without the CPU seeing them, so
        whatever the kernel alone would do to them does not happen.  A
        fabric port carries the chip's id in sysfs.  DSA ports are asked
        a second way because that id reaches them through devlink, which
        has not been confirmed on every generation we run on.
        """
        if self._switched is None:
            out = self.ssh.runsh(
                f"cat /sys/class/net/{self.name}/phys_switch_id 2>/dev/null; echo ---; "
                f"cat /sys/class/net/{self.name}/uevent").stdout
            switch_id, _, uevent = out.partition("---")
            self._switched = bool(switch_id.strip()) or "DEVTYPE=dsa" in uevent.split()

        return self._switched

    @property
    def driver(self):
        """The driver bound to the port, None when it has no device"""
        if self._driver is None:
            self._driver = self.ssh.runsh(
                f"sed -n 's/^DRIVER=//p' /sys/class/net/{self.name}/device/uevent"
            ).stdout.strip()

        return self._driver or None

    @property
    def offload(self):
        """The stages the port currently runs in hardware"""
        return self.caps.get("offload", [])

    @property
    def traffic_classes(self):
        """How many traffic classes the port has"""
        return self.caps.get("max-traffic-classes", 8)

    @property
    def trust_orders(self):
        """The classification trust orders the driver accepts, [] for none"""
        return self.caps.get("supported-trust-order", [])

    def offloads(self, stage):
        """Whether the port runs @stage in hardware"""
        return stage in self.offload

    def require_offload(self, test, stage):
        """Skip unless a fabric port runs @stage in hardware

        A port the CPU serves needs no offload: the kernel does the work
        and the measurement means what it says.
        """
        if self.switched and not self.offloads(stage):
            test.skip(f"{self.name} forwards in hardware and its driver "
                      f"does not offload {stage}")

    def require_classes(self, test, count):
        """Skip unless the port has at least @count traffic classes"""
        if self.traffic_classes < count:
            test.skip(f"{self.name} has {self.traffic_classes} traffic "
                      f"classes, the test needs {count}")
