#!/usr/bin/env python3
"""Verify the exact files in the redacted historical benchmark distribution."""
import argparse
from dataclasses import dataclass
import hashlib
import json
from pathlib import Path, PurePosixPath
import re
import sys


@dataclass(frozen=True)
class EvidenceFile:
    path: str
    size: int
    sha256: str


def parse_inventory(raw):
    if not isinstance(raw, dict) or raw.get("schema_version") != 1 or raw.get("artifact_type") != "sanitized-benchmark-distribution":
        raise ValueError("unsupported publication manifest")
    rows = raw.get("files")
    if not isinstance(rows, list) or not rows:
        raise ValueError("publication inventory is empty or malformed")
    entries = []
    for row in rows:
        if not isinstance(row, dict):
            raise ValueError("invalid file entry")
        name, size, digest = row.get("path"), row.get("bytes"), row.get("sha256")
        if not isinstance(name, str) or not name or "\\" in name:
            raise ValueError("invalid evidence path")
        relative = PurePosixPath(name)
        if relative.is_absolute() or ".." in relative.parts or relative.as_posix() != name or name == ".":
            raise ValueError("evidence path escapes or is not canonical")
        if type(size) is not int or size < 0 or not isinstance(digest, str) or not re.fullmatch(r"[0-9a-f]{64}", digest):
            raise ValueError("invalid evidence size or digest")
        entries.append(EvidenceFile(name, size, digest))
    names = [entry.path for entry in entries]
    if names != sorted(set(names)):
        raise ValueError("inventory paths must be unique and sorted")
    return tuple(entries)


def verify(root):
    manifest = root / "PUBLICATION_MANIFEST.json"
    entries = parse_inventory(json.loads(manifest.read_text()))
    actual = set()
    for path in root.rglob("*"):
        if path.is_symlink():
            raise ValueError("symlinks are not allowed in the distribution")
        if path.is_file() and path != manifest:
            actual.add(path.relative_to(root).as_posix())
    if actual != {entry.path for entry in entries}:
        raise ValueError("publication file inventory differs from the manifest")
    for entry in entries:
        data = (root / entry.path).read_bytes()
        if len(data) != entry.size or hashlib.sha256(data).hexdigest() != entry.sha256:
            raise ValueError("evidence bytes differ: " + entry.path)
    return len(entries)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parent)
    args = parser.parse_args()
    try:
        count = verify(args.root.resolve())
    except (OSError, ValueError) as error:
        print("verification failed: " + str(error), file=sys.stderr)
        return 1
    print(f"Verified {count} public benchmark files; this verifies the redacted distribution, not a new measurement.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
