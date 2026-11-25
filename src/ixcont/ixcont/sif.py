"""Singularity Image Format (SIF), the one kind of image ixcont runs.

A SIF file is a header, a table of descriptors and the data objects
they describe.  The root file system of the container is a SquashFS
partition, mounted read-only straight from the file.  The images made
here also carry the OCI image config, verbatim, and where the image
came from, as JSON objects.

Only what is needed to find those objects, and to wrap a SquashFS made
by sqfstar, is implemented.  The layout follows github.com/sylabs/sif.
"""
import json
import os
import struct

LAUNCH = b"#!/usr/bin/env run-singularity\n"
MAGIC = b"SIF_MAGIC\0"
VERSION = b"01\0"

HEADER = struct.Struct("<32s10s3s3s16s8q")
DESCRIPTOR = struct.Struct("<i?III7q128s384s")
PARTITION = struct.Struct("<ii3s")

DESCRIPTORS = 48
GROUP = 0xf0000001

DATA_PARTITION = 0x4004
DATA_JSON = 0x4006
FS_SQUASH = 1
PART_PRIMSYS = 2

ARCH = {
    "386": b"01\0", "amd64": b"02\0", "arm": b"03\0", "arm64": b"04\0",
    "riscv64": b"12\0",
}

# Objects of the images made here
CONFIG = "oci-config.json"
SOURCE = "source.json"

# Where sqfstar writes the file system, past header and descriptors
PARTITION_OFFSET = -(-(HEADER.size + DESCRIPTORS * DESCRIPTOR.size) // 4096) * 4096


class SifError(Exception):
    pass


class Sif:
    """An image file: where its root file system is, and what it says of itself."""

    def __init__(self, path):
        self.path = path
        self.offset = None
        self._json = {}

        size = os.path.getsize(path)
        with open(path, "rb") as f:
            head = f.read(HEADER.size)
            if len(head) < HEADER.size:
                raise SifError(f"{path}: not a SIF image")
            (_, magic, version, _, uuid, _, _, _, total, descoff, _, _, _) = HEADER.unpack(head)
            if magic != MAGIC or version != VERSION:
                raise SifError(f"{path}: not a SIF image")
            if not 0 < total <= 4096 or descoff + total * DESCRIPTOR.size > size:
                raise SifError(f"{path}: damaged descriptor table")
            self.id = uuid.hex()

            f.seek(descoff)
            for _ in range(total):
                (kind, used, _, _, _, off, length, _, _, _, _, _, name, extra) = \
                    DESCRIPTOR.unpack(f.read(DESCRIPTOR.size))
                if not used:
                    continue
                if off < 0 or length < 0 or off + length > size:
                    raise SifError(f"{path}: data object outside the file")
                if kind == DATA_PARTITION:
                    fstype, parttype, _ = PARTITION.unpack_from(extra)
                    if parttype == PART_PRIMSYS:
                        if fstype != FS_SQUASH:
                            raise SifError(f"{path}: root file system is not SquashFS")
                        self.offset = off
                elif kind == DATA_JSON:
                    self._json[name.rstrip(b"\0").decode(errors="replace")] = (off, length)

        if self.offset is None:
            raise SifError(f"{path}: no root file system")

    def json(self, name):
        """The JSON object called name, or an empty one."""
        if name not in self._json:
            return {}
        off, length = self._json[name]
        with open(self.path, "rb") as f:
            f.seek(off)
            try:
                return json.loads(f.read(length))
            except ValueError:
                raise SifError(f"{self.path}: damaged {name}")


def is_sif(path):
    try:
        with open(path, "rb") as f:
            return f.read(HEADER.size)[32:42] == MAGIC
    except OSError:
        return False


def wrap(path, arch, uuid, created, objects):
    """Make a SIF of path, where sqfstar left a file system at PARTITION_OFFSET.

    objects maps the name of each JSON object to add to its content.
    """
    arch = ARCH.get(arch, b"00\0")
    end = os.path.getsize(path)
    descs = [(DATA_PARTITION, PARTITION_OFFSET, end - PARTITION_OFFSET, b"",
              PARTITION.pack(FS_SQUASH, PART_PRIMSYS, arch))]

    with open(path, "r+b") as f:
        f.seek(end)
        for name, data in objects.items():
            data = json.dumps(data).encode()
            descs.append((DATA_JSON, f.tell(), len(data), name.encode(), b""))
            f.write(data)
        size = f.tell() - (HEADER.size + DESCRIPTORS * DESCRIPTOR.size)

        f.seek(0)
        f.write(HEADER.pack(LAUNCH, MAGIC, VERSION, arch, uuid, created, created,
                            DESCRIPTORS - len(descs), DESCRIPTORS, HEADER.size,
                            DESCRIPTORS * DESCRIPTOR.size,
                            HEADER.size + DESCRIPTORS * DESCRIPTOR.size, size))
        for num, (kind, off, length, name, extra) in enumerate(descs, 1):
            f.write(DESCRIPTOR.pack(kind, True, num, GROUP, 0, off, length, length,
                                    created, created, 0, 0, name, extra))
