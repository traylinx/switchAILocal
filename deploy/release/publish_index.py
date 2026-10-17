#!/usr/bin/env python3
"""Publish a native-build index only after both artifacts agree on scanned source."""
import argparse
import json
from pathlib import Path
import re
import subprocess

REPO = 'ghcr.io/traylinx/switchailocal'


def require(condition, code):
    if not condition:
        raise ValueError(code)


def command(records, scanned):
    require(re.fullmatch(r'[0-9a-f]{40}', scanned), 'SCANNED_SHA_INVALID')
    require(len(records) == 2 and {r['arch'] for r in records} == {'amd64', 'arm64'}, 'NATIVE_PLATFORMS_REQUIRED')
    require(all(r['source'] == scanned for r in records), 'SCANNED_SOURCE_MISMATCH')
    versions = {r['version'] for r in records}
    require(len(versions) == 1, 'NATIVE_VERSION_MISMATCH')
    version = versions.pop()
    require(version == 'dev' or re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?', version), 'VERSION_INVALID')
    require(all(re.fullmatch(r'sha256:[0-9a-f]{64}', r['digest']) for r in records), 'DIGEST_INVALID')
    require(len({r['digest'] for r in records}) == 2, 'NATIVE_DIGESTS_MUST_DIFFER')
    argv = ['docker', 'buildx', 'imagetools', 'create', '--tag', f'{REPO}:sha-{scanned}']
    if version != 'dev':
        argv += ['--tag', f'{REPO}:{version}']
    return argv + [f"{REPO}@{r['digest']}" for r in sorted(records, key=lambda r: r['arch'])]


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--metadata', type=Path, required=True)
    p.add_argument('--source', required=True)
    args = p.parse_args()
    records = [json.loads(f.read_text()) for f in sorted(args.metadata.glob('*.json'))]
    subprocess.run(command(records, args.source), check=True, timeout=180)


if __name__ == '__main__':
    main()
