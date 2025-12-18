#!/usr/bin/env python3

import collections
import datetime
import os
import random
import string
import struct

DATE=int(datetime.datetime.now().timestamp())

class UUID:
    def __init__(self):
        b = random.choices(string.ascii_letters + string.digits, k=32)
        self.bytes = "".join(b).encode()

    def dashed(self):
        return    self.bytes[0:6].decode()   + \
            "-" + self.bytes[6:10].decode()  + \
            "-" + self.bytes[10:14].decode() + \
            "-" + self.bytes[14:18].decode() + \
            "-" + self.bytes[18:22].decode() + \
            "-" + self.bytes[22:26].decode() + \
            "-" + self.bytes[26:32].decode()

def crc32(data: bytes) -> int:
    crc = 0xf597a6cf

    for byte in data:
        crc ^= byte
        for _ in range(8):
            if crc & 1:
                crc = (crc >> 1) ^ 0xedb88320
            else:
                crc >>= 1

    return crc & 0xffffffff

def sendfile(dst, src, count):
    while count > 0:
        chunk = os.sendfile(dst, src, None, count)
        if chunk < 0:
            raise OSError("sendfile")

        count -= chunk

class Linear:
    def __init__(self, start, align, path, name=None):
        self.uuid = UUID()
        self.name = name if name else os.path.basename(path)
        self.path = path
        self.start = start
        self.align = align
        self.size = os.path.getsize(path)
        self.asize = (self.size + align - 1) & ~(align - 1)

    def sendfile(self, fd):
        pad = self.asize - self.size

        with open(self.path, "rb") as f:
            sendfile(fd, f.fileno(), self.size)

            if pad:
                os.write(fd, b"\0" * pad)

    def meta(self):
        return f"""        {self.name} {{
            id = "{self.uuid.dashed()}"
            status = ["READ", "WRITE", "VISIBLE"]
            segment_count = 1

            segment1 {{
                start_extent = 0
                extent_count = {self.asize // self.align }
                type = "striped"
                stripe_count = 1
                stripes = [
                    "pv0", {self.start // self.align }
                ]
            }}
        }}"""

class InternalPV:
    def __init__(self, images=[], align=(1 << 20)):
        self.pv_uuid = UUID()
        self.vg_uuid = UUID()
        self.align = align

        self.lvs = []
        self.dsize = 0
        for img in images:
            lv = Linear(self.dsize, align, img)
            self.lvs.append(lv)
            self.dsize += lv.asize

        # Reserve `align` bytes for primary metadata area, the size
        # used by all LVs, and another `aligned` bytes for the backup
        # metadata area
        self.size = align + self.dsize + align

        # self.labels = [self.label(i) for i in range(4)]
        # self.meta = self._meta()

    def write(self, dst):
        for i in range(4):
            dst.write(self._label(i))

        dst.write(b"\0" * 0x800)

        dst.write(self._metadata(True))

        for lv in self.lvs:
            lv.sendfile(dst.fileno())

        dst.write(self._metadata(False))

    def _label(self, sector: int):
        headfmt, tailfmt = "<8s  Q  I", "<I8s  32sQ  QQ QQ  QQ QQ QQ"
        padsize = 512 - struct.calcsize(headfmt) - struct.calcsize(tailfmt)

        tail = struct.pack(tailfmt,
                           32, b"LVM2 001",
                           self.pv_uuid.bytes, self.size,
                           self.align, self.dsize,
                           0, 0,
                           0x1000, self.align - 0x1000,
                           self.align + self.dsize, self.align,
                           0, 0) + b"\0" * padsize

        head = struct.pack(headfmt, b"LABELONE", sector, crc32(tail))

        return head + tail

    def _metadata(self, primary):
        headfmt, tailfmt = "<I", "<16sI QQ QQII QQII"
        padsize = 512 - struct.calcsize(headfmt) - struct.calcsize(tailfmt)

        offs = 0x1000 if primary else (self.align + self.dsize)
        size = self.align - 0x1000 if primary else self.align

        txt = self._meta_txt()
        txtpadsize = size - 512 - len(txt)

        tail = struct.pack(tailfmt,
                           b" LVM2 x[5A%r0N*>", 1,
                           offs, size,
                           512, len(txt), crc32(txt), 0,
                           0, 0, 0, 0) + b"\0" * padsize

        head = struct.pack(headfmt, crc32(tail))

        return head + tail + txt + bytes([0] * txtpadsize)

    def _meta_txt(self):
        return f"""internal {{
    id = "{self.vg_uuid.dashed()}"
    seqno = 1
    status = ["RESIZEABLE", "READ", "WRITE"]
    extent_size = {self.align >> 9}
    max_pv = 0
    max_lv = 0
    metadata_copies = 2

    physical_volumes {{
        pv0 {{
            id = "{self.pv_uuid.dashed()}"
            dev = "/dev/internal"
            status = ["ALLOCATABLE"]
            dev_size = {self.size >> 9}
            pe_start = {self.align >> 9}
            pe_count = {self.dsize // self.align }
        }}
    }}

    logical_volumes {{
{"\n".join([lv.meta() for lv in self.lvs])}
    }}
}}

contents = "Text Format Volume Group"
version = 1
description = "Internal image storage"
creation_host = "infix-build-system"
creation_time = {DATE}
""".encode()


def main():
    import sys

    pv = InternalPV(sys.argv[1:])
    pv.write(sys.stdout.buffer)

if __name__ == "__main__":
    main()
