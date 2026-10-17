#!/usr/bin/env bash
# Inert installation. Does not enroll, apply, reload systemd or enable timers.
set -euo pipefail
SOURCE="$(cd "$(dirname "$0")" && pwd)"
ROOT="${1:-/}"
[ "${ROOT}" != / ] || [ "$(id -u)" -eq 0 ] || { echo 'ROOT_REQUIRED' >&2; exit 1; }
python3 - "${SOURCE}" "${ROOT}" <<'PY'
import os, pathlib, shutil, stat, sys
source, root = map(pathlib.Path, sys.argv[1:])
root = root.resolve(strict=True)
version = 'v1'
def directory(relative, mode):
    path = root
    for part in pathlib.Path(relative).parts:
        path /= part
        if not path.exists() and not path.is_symlink():
            path.mkdir(mode=mode)
        info = path.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o022:
            raise SystemExit('UNTRUSTED_INSTALL_DIRECTORY')
    return path
binary = directory('opt/switchailocal-updater/' + version, 0o755)
units = directory('etc/systemd/system', 0o755)
directory('etc/switchailocal-updater/profiles', 0o700)
directory('etc/switchailocal-updater/enrolled', 0o700)
directory('var/lib/switchailocal-updater', 0o700)
for src, dst, mode in [(source / name, binary / name, 0o755) for name in ('managed_update.py', 'probe.py', 'promote.py')] + [(src, units / src.name, 0o644) for src in (source / 'systemd').glob('*')]:
    if dst.exists() or dst.is_symlink():
        if dst.is_symlink() or dst.read_bytes() != src.read_bytes():
            raise SystemExit('EXISTING_INSTALL_DIFFERS: install a reviewed new tool version separately')
        continue
    fd = os.open(dst, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, mode)
    with os.fdopen(fd, 'wb') as out:
        out.write(src.read_bytes())
print('Installed v1. Timer inactive. Configure, validate, check, explicitly enroll/apply, then separately enable timer.')
PY
