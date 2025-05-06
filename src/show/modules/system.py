import json

from common import Decore
from common import JsonData
from dataclasses import dataclass

class System:
    def __init__(self, cfg):
        self.services = self.Services(cfg)
        self.software = self.Software(cfg)

    class Services:
        def __init__(self, cfg):
            self.cfg = cfg

        def get(self):
            return self.cfg.run("/ietf-system:system-state/infix-system:services")

        def pretty(self):
            data = self.get()
            return data if data else {}

    class Software:
        class Slot:
            @dataclass
            class Pad:
                name: int = 10
                state: int = 10
                version: int = 23
                date: int = 25
                hash: int = 64

            def __init__(self, data, version, date): # TODO: why is slot version and date on top level?
                self.pad = self.Pad()
                self.name = data.get('bootname', '')
                self.size = data.get('size', '')
                self.type = data.get('class', '')
                self.hash = data.get('sha256', '')
                self.state = data.get('state', '')
                self.version = version # TODO
                self.date = version # TODO

            def print_detailed(self):
                """Detailed information about one bundle"""
                print(f"Name      : {self.name}")
                print(f"State     : {self.state}")
                print(f"Version   : {self.version}")
                print(f"Size      : {self.size}")
                print(f"SHA-256   : {self.hash}")
                print(f"Installed : {self.date}")

            @classmethod
            def print_hdr(cls):
                pad = cls.Pad()
                hdr = (f"{'NAME':<{pad.name}}"
                       f"{'STATE':<{pad.state}}"
                       f"{'VERSION':<{pad.version}}"
                       f"{'DATE':<{pad.date}}")
                print(Decore.invert(hdr))

            def print_slot_row(self):
                if self.type != "rootfs":
                    return

                """Brief information about one bundle"""
                row  = f"{self.name:<{self.pad.name}}"
                row += f"{self.state:<{self.pad.state}}"
                row += f"{self.version:<{self.pad.version}}"
                row += f"{self.date:<{self.pad.date}}"
                print(row)


        def __init__(self, cfg):
            self.cfg = cfg
            self.data = self.get()

            self.slots = self.data.get("slot", [])
            self.version = JsonData.get('', self.data, 'bundle', 'version')
            self.date = JsonData.get('', self.data, 'installed', 'datetime')
            self.boot_order = self.data.get("boot-order", ["Unknown"])

        def get(self):
            return self.cfg.run("/ietf-system:system-state/infix-system:software")

        def pprint(self):
            software = self.data

            print(Decore.invert("BOOT ORDER"))
            order = " ".join(boot.strip() for boot in self.boot_order)
            print(f"{order}\n")

            self.Slot.print_hdr()
            for _slot in reversed(self.slots):
                slot = self.Slot(_slot, self.version, self.date)
                slot.print_slot_row()

        def pprint_slot(self, name):
            match = next((slot for slot in self.slots if slot.get("bootname") == name), None)
            if match:
                slot = self.Slot(match, self.version, self.date)
                slot.print_detailed()

        def show(self, args):
            if self.cfg.raw_output:
                print(json.dumps(self.data, indent=2))
                return

            if args:
                self.pprint_slot(args[0])
            else:
                self.pprint()
