#!/usr/bin/env python3
"""CI transition validation. No initialized placeholder channel is permitted."""
import argparse
import json
from pathlib import Path
import re
import subprocess

from managed_update import UpdateError, require, validate_manifest


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--base', required=True)
    args = p.parse_args()
    try:
        require(re.fullmatch(r'[a-f0-9]{40}', args.base) is not None and args.base != '0' * 40, 'VALID_BASE_COMMIT_REQUIRED')
        files = {channel: Path('channels') / (channel + '.json') for channel in ('canary', 'stable')}
        current = {}
        prior = {}
        for channel, file in files.items():
            old = subprocess.run(['git', 'show', args.base + ':' + file.as_posix()], capture_output=True, timeout=10)
            if old.returncode == 0:
                prior[channel] = validate_manifest(json.loads(old.stdout), channel)
            if file.exists():
                current[channel] = validate_manifest(json.loads(file.read_text()), channel)
            require(channel not in prior or channel in current, 'CHANNEL_DELETION_REFUSED')
            if channel in prior and current[channel] != prior[channel]:
                require(current[channel]['generation'] > prior[channel]['generation'], 'CHANNEL_GENERATION_MUST_INCREASE')
        stable = current.get('stable')
        if stable and stable != prior.get('stable'):
            canary = prior.get('canary')
            require(bool(canary), 'PREVIOUSLY_PUBLISHED_CANARY_REQUIRED')
            require(all(stable[k] == canary[k] for k in ('image', 'version', 'source_commit')), 'STABLE_MUST_MATCH_PUBLISHED_CANARY')
        print(json.dumps({'valid': True, 'channels': sorted(current)}))
        return 0
    except (UpdateError, ValueError, OSError, subprocess.TimeoutExpired) as error:
        print(json.dumps({'valid': False, 'code': str(error) if isinstance(error, UpdateError) else 'CHANNEL_VALIDATION_FAILED'}))
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
