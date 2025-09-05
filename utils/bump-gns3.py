#!/usr/bin/env python3
import argparse
import hashlib
import json
import re
import shutil
import sys
import urllib.request
from datetime import datetime

BIOS_IMAGE = "OVMF-edk2-stable202305.fd"
REPO = "https://github.com/kernelkit/infix"

def check_version_format(ver):
    if not re.fullmatch(r"\d{2}\.\d{2}\.\d+", ver):
        sys.exit("Error: version must be full form like 25.08.0")
    return ver

def compute_md5_and_size(url):
    md5 = hashlib.md5()
    size = 0
    with urllib.request.urlopen(url) as resp:
        while True:
            chunk = resp.read(1024 * 1024)
            if not chunk:
                break
            md5.update(chunk)
            size += len(chunk)
    return md5.hexdigest(), size

def main():
    parser = argparse.ArgumentParser(description="Add a new Infix version to a GNS3 appliance file")
    parser.add_argument("version", help="Infix version (e.g. 25.08.0)")
    parser.add_argument("appliance", help="Path to appliance JSON (.gns3a)")
    args = parser.parse_args()

    version = check_version_format(args.version)
    appliance_path = args.appliance

    # Load JSON
    with open(appliance_path, "r", encoding="utf-8") as f:
        data = json.load(f)

    # Skip if version already exists
    for v in data.get("versions", []):
        if v.get("name") == version:
            print(f"Version {version} already exists.")
            return

    # Build qcow2 URL
    filename = f"infix-x86_64-disk-{version}.qcow2"
    url = f"{REPO}/releases/download/v{version}/{filename}"

    print(f"Downloading {url} to compute MD5 and size...")
    md5sum, size = compute_md5_and_size(url)

    # Add image entry
    image_entry = {
        "filename": filename,
        "filesize": size,
        "md5sum": md5sum,
        "version": version,
        "direct_download_url": url
    }
    data.setdefault("images", []).append(image_entry)

    # Add version entry (newest first)
    version_entry = {
        "name": version,
        "images": {
            "bios_image": BIOS_IMAGE,
            "hda_disk_image": filename
        }
    }
    data.setdefault("versions", []).insert(0, version_entry)

    # Backup
    backup = appliance_path + ".bak-" + datetime.now().strftime("%Y%m%d-%H%M%S")
    shutil.copy2(appliance_path, backup)

    # Save updated JSON
    with open(appliance_path, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=4)

    print(f"✅ Added version {version}")
    print(f"   File: {appliance_path}")
    print(f"   Backup: {backup}")
    print(f"   Disk: {filename} (size={size} bytes, md5={md5sum})")

if __name__ == "__main__":
    main()
