#!/usr/bin/env python3
"""Writes the release manifest for a folder of Zonn Node installers.

    scripts/release-manifest.py VERSION FOLDER

Creates FOLDER/zonn-node-release.json (the version, and each installer's
operating system, architecture, variant, format, size, and SHA-256) and
FOLDER/SHA256SUMS (for `sha256sum -c`). Copy the folder's contents to the
server's downloads folder: the backend offers the installers to nodes as
updates, after checking each file against this manifest, and Zonn Console's
Downloads page shows the checksums.
"""
import hashlib
import json
import os
import re
import sys

# Installer file names, as the release workflow builds them.
PATTERNS = [
    (r"^Zonn-Node-Setup-Windows-x64-NVIDIA\.exe$", "windows", "amd64", "nvidia", "exe"),
    (r"^Zonn-Node-Setup-Windows-x64\.exe$", "windows", "amd64", "cpu", "exe"),
    (r"^zonn-node-nvidia_[^_]+_amd64\.deb$", "linux", "amd64", "nvidia", "deb"),
    (r"^zonn-node_[^_]+_amd64\.deb$", "linux", "amd64", "cpu", "deb"),
    (r"^Zonn-Node-.+-macOS-apple-silicon\.dmg$", "darwin", "arm64", "cpu", "dmg"),
    (r"^Zonn-Node-.+-macOS-intel\.dmg$", "darwin", "amd64", "cpu", "dmg"),
]


def describe(name):
    for pattern, os_name, arch, variant, fmt in PATTERNS:
        if re.match(pattern, name):
            return {"os": os_name, "arch": arch, "variant": variant, "format": fmt}
    return None


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for block in iter(lambda: f.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()


def main():
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    version, folder = sys.argv[1].lstrip("v"), sys.argv[2]
    if not re.match(r"^\d+\.\d+\.\d+", version):
        sys.exit(f"invalid version {version}")
    files = []
    for name in sorted(os.listdir(folder)):
        path = os.path.join(folder, name)
        meta = describe(name)
        if meta is None or not os.path.isfile(path):
            continue
        if meta["format"] == "deb" and f"_{version}_" not in name:
            sys.exit(f"{name} is not version {version}")
        files.append({"name": name, **meta, "size": os.path.getsize(path), "sha256": sha256(path)})
    if not files:
        sys.exit(f"no installers in {folder}")
    with open(os.path.join(folder, "zonn-node-release.json"), "w") as f:
        json.dump({"version": version, "files": files}, f, indent=2)
        f.write("\n")
    with open(os.path.join(folder, "SHA256SUMS"), "w") as f:
        for entry in files:
            f.write(f"{entry['sha256']}  {entry['name']}\n")
    for entry in files:
        print(f"{entry['sha256']}  {entry['name']}  ({entry['os']} {entry['arch']} {entry['variant']})")


if __name__ == "__main__":
    main()
