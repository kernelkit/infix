#!/usr/bin/env python3
import argparse
import hashlib
import json
import re
import shutil
import sys
import urllib.error
import urllib.request
from datetime import datetime

BIOS_IMAGE = "OVMF-edk2-stable202305.fd"
REPO = "https://github.com/kernelkit/infix"

def check_version_format(ver):
    if not re.fullmatch(r"\d{2}\.\d{2}\.\d+", ver):
        sys.exit("Error: version must be full form like 25.08.0")
    return ver

def download_file(url):
    """Download file from URL, yielding chunks.

    Args:
        url: URL of the file to download

    Yields:
        bytes: 1MB chunks of the file

    Raises:
        urllib.error.HTTPError: If the URL returns an error (e.g., 404)
        urllib.error.URLError: If there's a network error
    """
    with urllib.request.urlopen(url) as resp:
        while True:
            chunk = resp.read(1024 * 1024)
            if not chunk:
                break
            yield chunk

def compute_md5_and_size(chunks):
    """Compute MD5 hash and total size from data chunks.

    Args:
        chunks: Iterable of byte chunks

    Returns:
        tuple: (md5_hexdigest, size_in_bytes)
    """
    md5 = hashlib.md5()
    size = 0
    for chunk in chunks:
        md5.update(chunk)
        size += len(chunk)
    return md5.hexdigest(), size

def main():
    parser = argparse.ArgumentParser(
        description="Add a new Infix version to a GNS3 appliance file",
        epilog="The .gns3a appliance files are typically found in the appliances/ "
               "directory of the gns3-registry project."
    )
    parser.add_argument("version", help="Infix version (e.g. 25.08.0)")
    parser.add_argument("appliance", help="Path to appliance JSON file (.gns3a)")
    args = parser.parse_args()

    version = check_version_format(args.version)
    appliance_path = args.appliance

    # Load JSON
    try:
        with open(appliance_path, "r", encoding="utf-8") as f:
            data = json.load(f)
    except FileNotFoundError:
        sys.exit(f"Error: Appliance file not found: {appliance_path}")
    except json.JSONDecodeError as e:
        sys.exit(f"Error: Invalid JSON in appliance file: {e}")

    # Skip if version already exists
    for v in data.get("versions", []):
        if v.get("name") == version:
            print(f"Version {version} already exists.")
            return

    # Build qcow2 URL
    filename = f"infix-x86_64-v{version}.qcow2"
    url = f"{REPO}/releases/download/v{version}/{filename}"

    print(f"Downloading {url} to compute MD5 and size...")
    try:
        chunks = download_file(url)
        md5sum, size = compute_md5_and_size(chunks)
    except urllib.error.HTTPError as e:
        if e.code == 404:
            sys.exit(f"Error: Version {version} not found. "
                    f"The release v{version} does not exist or the disk image is not available.\n"
                    f"URL: {url}")
        else:
            sys.exit(f"Error: HTTP {e.code} when downloading: {e.reason}\nURL: {url}")
    except urllib.error.URLError as e:
        sys.exit(f"Error: Network error while downloading: {e.reason}\nURL: {url}")

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
