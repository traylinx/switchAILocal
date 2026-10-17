#!/usr/bin/env python3
"""Prepare a local channel manifest; never pushes, builds or deploys."""
import argparse
import json
from pathlib import Path

from managed_update import UpdateError, atomic_json, require, utc, validate_manifest


def prepare(channel, generation, version, image, source, evidence, previous=None, canary=None):
    value = validate_manifest({'schema_version': 1, 'channel': channel, 'generation': generation, 'version': version, 'image': image, 'source_commit': source, 'promoted_at': utc(), 'verification_ref': evidence}, channel)
    if previous:
        validate_manifest(previous, channel)
        require(generation > previous['generation'], 'PROMOTION_GENERATION_MUST_INCREASE')
    if channel == 'stable':
        require(bool(canary), 'CANARY_MANIFEST_REQUIRED')
        validate_manifest(canary, 'canary')
        require(all(canary[k] == value[k] for k in ('image', 'source_commit', 'version')), 'STABLE_MUST_MATCH_CANARY')
    return value


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--channel', choices=['canary', 'stable'], required=True)
    p.add_argument('--generation', type=int, required=True)
    p.add_argument('--version', required=True)
    p.add_argument('--image', required=True)
    p.add_argument('--source', required=True)
    p.add_argument('--evidence', default='')
    p.add_argument('--canary', type=Path)
    p.add_argument('--out', type=Path, required=True)
    args = p.parse_args()
    try:
        # Existing destination is the previous feed, never a caller-supplied substitute.
        previous = json.loads(args.out.read_text()) if args.out.exists() else None
        canary = json.loads(args.canary.read_text()) if args.canary else None
        value = prepare(args.channel, args.generation, args.version, args.image, args.source, args.evidence, previous, canary)
        atomic_json(args.out, value)
        print(json.dumps({'prepared': str(args.out), 'channel': args.channel, 'generation': args.generation, 'published': False}))
        return 0
    except (UpdateError, ValueError, OSError) as error:
        print(json.dumps({'ok': False, 'code': str(error) if isinstance(error, UpdateError) else 'PROMOTION_INPUT_INVALID'}))
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
