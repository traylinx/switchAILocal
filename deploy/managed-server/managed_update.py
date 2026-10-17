#!/usr/bin/env python3
"""Image-only switchAILocal reconciliation. Python stdlib; no Git dependency."""
from __future__ import annotations

import argparse
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import datetime, timezone
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import selectors
import signal
import stat
import subprocess
import sys
import time
import urllib.parse
import urllib.request

UPDATER_VERSION = 1
REPOSITORY = "ghcr.io/traylinx/switchailocal"
STATE_ROOT = Path("/var/lib/switchailocal-updater")
ENROLL_ROOT = Path("/etc/switchailocal-updater/enrolled")
DIGEST = re.compile(r"sha256:[a-f0-9]{64}\Z")
SHA = re.compile(r"[a-f0-9]{40}\Z")
NAME = re.compile(r"[a-zA-Z0-9][a-zA-Z0-9_.-]{0,79}\Z")
VERSION = re.compile(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?\Z")


class UpdateError(Exception):
    """Safe public error code; never attach subprocess stderr or a response body."""


def require(condition: bool, code: str) -> None:
    if not condition:
        raise UpdateError(code)


def utc() -> str:
    return datetime.now(timezone.utc).isoformat()


def canonical(value: object) -> bytes:
    return json.dumps(value, sort_keys=True, separators=(",", ":")).encode()


def fingerprint(value: object) -> str:
    return hashlib.sha256(canonical(value)).hexdigest()


def secure_file(path: Path, uid: int = 0, anchor: Path = Path("/"), private: bool = False) -> None:
    require(path.is_absolute() and path != anchor and path.is_relative_to(anchor), "UNTRUSTED_PATH")
    require(".." not in path.parts, "UNTRUSTED_PATH")
    current = anchor
    for part in path.relative_to(anchor).parts:
        current = current / part
        try:
            info = current.lstat()
        except OSError:
            raise UpdateError("TRUSTED_FILE_MISSING") from None
        require(not stat.S_ISLNK(info.st_mode), "SYMLINK_REFUSED")
        require(info.st_uid == uid and not info.st_mode & 0o022, "UNTRUSTED_FILE_PERMISSIONS")
    require(stat.S_ISREG(info.st_mode), "REGULAR_FILE_REQUIRED")
    if private:
        require(not info.st_mode & 0o077, "PRIVATE_FILE_REQUIRED")


def read_json(path: Path, uid: int = 0, anchor: Path = Path("/"), private: bool = False) -> dict:
    secure_file(path, uid, anchor, private)
    try:
        require(path.stat().st_size <= 65536, "JSON_TOO_LARGE")
        data = json.loads(path.read_bytes())
    except (OSError, ValueError):
        raise UpdateError("INVALID_JSON") from None
    require(isinstance(data, dict), "JSON_OBJECT_REQUIRED")
    return data


def atomic_json(path: Path, data: dict) -> None:
    require(not path.is_symlink(), "SYMLINK_REFUSED")
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    temporary = path.with_name(path.name + f".{os.getpid()}.tmp")
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    try:
        with os.fdopen(fd, "wb") as stream:
            stream.write(canonical(data) + b"\n")
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
        directory = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        temporary.unlink(missing_ok=True)


def private_directory(path: Path, uid: int, anchor: Path) -> None:
    require(path.is_absolute() and path.is_relative_to(anchor) and ".." not in path.parts, "UNTRUSTED_PATH")
    current = anchor
    for part in path.relative_to(anchor).parts:
        current /= part
        if not current.exists() and not current.is_symlink():
            current.mkdir(mode=0o700)
        info = current.lstat()
        require(stat.S_ISDIR(info.st_mode) and info.st_uid == uid and not info.st_mode & 0o022, "UNTRUSTED_DIRECTORY")


def validate_manifest(data: dict, channel: str) -> dict:
    require(isinstance(data, dict), "MANIFEST_INVALID")
    expected = {"schema_version", "channel", "generation", "version", "image", "source_commit", "promoted_at", "verification_ref"}
    require(set(data) == expected, "MANIFEST_FIELDS_INVALID")
    require(type(data["schema_version"]) is int and data["schema_version"] == UPDATER_VERSION, "MANIFEST_SCHEMA_UNSUPPORTED")
    require(channel in ("canary", "stable") and data["channel"] == channel, "CHANNEL_MISMATCH")
    require(type(data["generation"]) is int and 0 < data["generation"] < 2**53, "GENERATION_INVALID")
    require(isinstance(data["version"], str) and VERSION.fullmatch(data["version"]), "VERSION_INVALID")
    require(channel != "stable" or "-" not in data["version"], "STABLE_PRERELEASE_REFUSED")
    require(isinstance(data["image"], str) and data["image"].startswith(REPOSITORY + "@"), "IMAGE_REPOSITORY_REFUSED")
    require(DIGEST.fullmatch(data["image"].removeprefix(REPOSITORY + "@")), "IMMUTABLE_IMAGE_REQUIRED")
    require(isinstance(data["source_commit"], str) and SHA.fullmatch(data["source_commit"]), "SOURCE_SHA_INVALID")
    require(isinstance(data["promoted_at"], str) and len(data["promoted_at"]) <= 40, "PROMOTION_TIME_INVALID")
    try:
        require(datetime.fromisoformat(data["promoted_at"].replace("Z", "+00:00")).utcoffset() is not None, "PROMOTION_TIME_INVALID")
    except ValueError:
        raise UpdateError("PROMOTION_TIME_INVALID") from None
    ref = data["verification_ref"]
    require(isinstance(ref, str) and len(ref) <= 2048, "VERIFICATION_REFERENCE_INVALID")
    require(channel != "stable" or bool(ref), "STABLE_VERIFICATION_REQUIRED")
    if ref:
        valid_https(ref, "VERIFICATION_REFERENCE_INVALID")
    return dict(data)


def valid_https(value: str, code: str) -> None:
    require(isinstance(value, str) and len(value) <= 4096, code)
    try:
        parsed = urllib.parse.urlsplit(value)
        require(parsed.scheme == "https" and bool(parsed.hostname) and parsed.username is None and parsed.password is None and not parsed.query and not parsed.fragment, code)
    except ValueError:
        raise UpdateError(code) from None


class NoRedirects(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise UpdateError("MANIFEST_REDIRECT_REFUSED")


def fetch_manifest(url: str, channel: str) -> dict:
    valid_https(url, "MANIFEST_URL_INVALID")
    try:
        request = urllib.request.Request(url, headers={"User-Agent": "switchailocal-updater/1", "Cache-Control": "no-cache"})
        with urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirects).open(request, timeout=20) as response:
            raw = response.read(65537)
        require(len(raw) <= 65536, "MANIFEST_TOO_LARGE")
        data = json.loads(raw)
        require(isinstance(data, dict), "MANIFEST_INVALID")
    except UpdateError:
        raise
    except Exception:
        raise UpdateError("MANIFEST_FETCH_FAILED") from None
    return validate_manifest(data, channel)


def command(value: object, required: bool = False) -> list[str]:
    require(isinstance(value, list) and len(value) <= 64 and all(isinstance(v, str) and 0 < len(v) <= 4096 and "\x00" not in v for v in value), "COMMAND_INVALID")
    require(not required or bool(value), "VERIFICATION_COMMAND_REQUIRED")
    if value:
        require(Path(value[0]).is_absolute(), "ABSOLUTE_EXECUTABLE_REQUIRED")
    return list(value)


@dataclass(frozen=True)
class Profile:
    name: str
    channel: str
    manifest_url: str
    compose_files: tuple[Path, ...]
    project: str
    service: str
    verify_command: tuple[str, ...]
    lock_file: Path
    env_files: tuple[Path, ...] = ()
    pre_update_command: tuple[str, ...] = ()
    post_update_command: tuple[str, ...] = ()
    allow_recreate: bool = False
    docker_binary: str = "/usr/bin/docker"
    docker_host: str = "unix:///var/run/docker.sock"
    pull_timeout: int = 300
    compose_timeout: int = 120
    health_timeout: int = 60
    verify_timeout: int = 90
    hook_timeout: int = 60


def load_profile(path: Path, uid: int = 0, anchor: Path = Path("/")) -> Profile:
    data = read_json(path, uid, anchor, private=True)
    required = {"schema_version", "name", "channel", "manifest_url", "compose_files", "project", "service", "verify_command", "lock_file"}
    optional = {"env_files", "pre_update_command", "post_update_command", "allow_recreate", "docker_binary", "docker_host", "pull_timeout", "compose_timeout", "health_timeout", "verify_timeout", "hook_timeout"}
    require(required <= set(data) <= required | optional, "PROFILE_FIELDS_INVALID")
    schema = data.pop("schema_version")
    require(type(schema) is int and schema == 1, "PROFILE_SCHEMA_UNSUPPORTED")
    for field in ("name", "project", "service"):
        require(isinstance(data[field], str) and NAME.fullmatch(data[field]), "PROFILE_NAME_INVALID")
    require(data["channel"] in ("stable", "canary"), "CHANNEL_INVALID")
    valid_https(data["manifest_url"], "MANIFEST_URL_INVALID")
    files = data["compose_files"]
    require(isinstance(files, list) and 0 < len(files) <= 8 and all(isinstance(f, str) for f in files), "COMPOSE_FILES_INVALID")
    data["compose_files"] = tuple(Path(f) for f in files)
    for file in data["compose_files"]:
        secure_file(file, uid, anchor)
    env_files = data.get("env_files", [])
    require(isinstance(env_files, list) and len(env_files) <= 8 and all(isinstance(f, str) for f in env_files), "ENV_FILES_INVALID")
    data["env_files"] = tuple(Path(f) for f in env_files)
    for file in data["env_files"]:
        secure_file(file, uid, anchor, private=True)
    for field in ("verify_command", "pre_update_command", "post_update_command"):
        data[field] = tuple(command(data.get(field, []), field == "verify_command"))
    require(type(data.get("allow_recreate", False)) is bool, "RECREATE_ACK_INVALID")
    require(isinstance(data["lock_file"], str), "LOCK_PATH_INVALID")
    data["lock_file"] = Path(data["lock_file"])
    require(data["lock_file"].is_absolute() and ".." not in data["lock_file"].parts, "LOCK_PATH_INVALID")
    require(isinstance(data.get("docker_binary", "/usr/bin/docker"), str), "DOCKER_EXECUTABLE_INVALID")
    secure_file(Path(data.get("docker_binary", "/usr/bin/docker")), uid, anchor)
    host = data.get("docker_host", "unix:///var/run/docker.sock")
    require(isinstance(host, str) and host.startswith("unix:///") and ".." not in Path(host.removeprefix("unix://")).parts and not any(c in host for c in "?#\n\x00"), "LOCAL_DOCKER_REQUIRED")
    for field in ("pull_timeout", "compose_timeout", "health_timeout", "verify_timeout", "hook_timeout"):
        if field in data:
            require(type(data[field]) is int and 1 <= data[field] <= (900 if field == "pull_timeout" else 300), "TIMEOUT_INVALID")
    return Profile(**data)


class Runner:
    """Discard stderr, bound stdout, kill the entire process group on failure."""
    def __init__(self, cwd: Path = Path("/")):
        self.cwd = cwd

    def run(self, argv: list[str], timeout: int) -> str:
        process = None
        try:
            env = {"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "HOME": "/root", "LANG": "C.UTF-8", "COMPOSE_DISABLE_ENV_FILE": "true"}
            process = subprocess.Popen(argv, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, stdin=subprocess.DEVNULL, cwd=self.cwd, env=env, start_new_session=True)
            output = bytearray()
            deadline = time.monotonic() + timeout
            with selectors.DefaultSelector() as selector:
                selector.register(process.stdout, selectors.EVENT_READ)
                while selector.get_map():
                    require(time.monotonic() < deadline, "COMMAND_TIMEOUT")
                    for key, _ in selector.select(min(0.1, max(0, deadline - time.monotonic()))):
                        chunk = os.read(key.fileobj.fileno(), 65536)
                        if chunk:
                            output.extend(chunk)
                            require(len(output) <= 1048576, "COMMAND_OUTPUT_LIMIT")
                        else:
                            selector.unregister(key.fileobj)
            require(process.wait(timeout=max(0.01, deadline - time.monotonic())) == 0, "COMMAND_FAILED")
            return output.decode("utf-8", errors="strict")
        except (OSError, UnicodeError, subprocess.TimeoutExpired):
            raise UpdateError("COMMAND_START_OR_TIMEOUT_FAILED") from None
        finally:
            if process is not None:
                # Also reap children that inherited pipes or outlived their parent.
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                process.wait()
                process.stdout.close()


class Engine:
    def __init__(self, profile: Profile, runner=None, state_root: Path = STATE_ROOT, uid: int = 0, anchor: Path = Path("/"), sleep=time.sleep, clock=time.monotonic, enroll_root: Path = ENROLL_ROOT):
        self.profile = profile
        self.runner = runner or Runner(profile.compose_files[0].parent)
        self.root = state_root
        self.directory = state_root / profile.name
        self.uid = uid
        self.anchor = anchor
        self.sleep = sleep
        self.clock = clock
        self.marker = enroll_root / (profile.name + ".json")

    def binding(self) -> str:
        return fingerprint({"name": self.profile.name, "channel": self.profile.channel, "manifest_url": self.profile.manifest_url, "project": self.profile.project, "service": self.profile.service, "files": [str(f) for f in self.profile.compose_files], "env_files": [str(f) for f in self.profile.env_files], "lock": str(self.profile.lock_file)})

    def enrollment(self, required=False) -> bool:
        exists = self.marker.exists() or self.marker.is_symlink()
        if exists:
            marker = read_json(self.marker, self.uid, self.anchor, private=True)
            require(marker == {"schema_version": 1, "binding": self.binding()}, "ENROLLMENT_BINDING_MISMATCH")
            require(all(bool(self.saved(name)) for name in ("status.json", "accepted.json", "quarantine.json")), "ENROLLED_STATE_MISSING")
        require(exists or not required, "EXPLICIT_ENROLLMENT_REQUIRED")
        return exists

    def enroll(self) -> dict:
        with self.locked():
            require(not self.enrollment(), "ALREADY_ENROLLED")
            require(not any(self.saved(f) for f in ("status.json", "transaction.json", "previous.json", "image.override.json")), "ORPHANED_STATE_REFUSED")
            current = self.identity()
            self.verify(current["image"])
            self.save("status.json", {"result": "enrolled", "running_image": current["image"], "checked_at": utc()})
            self.save("accepted.json", {})
            self.save("quarantine.json", {"images": []})
            private_directory(self.marker.parent, self.uid, self.anchor)
            atomic_json(self.marker, {"schema_version": 1, "binding": self.binding()})
            return {"profile": self.profile.name, "enrolled": True, "running_image": current["image"]}

    def saved(self, name: str) -> dict:
        path = self.directory / name
        value = read_json(path, self.uid, self.anchor, private=True) if path.exists() or path.is_symlink() else {}
        if value and name != "image.override.json":
            require(type(value.get("schema_version")) is int and value.get("schema_version") == 1 and value.get("binding") == self.binding(), "STATE_SCHEMA_OR_BINDING_INVALID")
            if "manifest" in value:
                validate_manifest(value["manifest"], self.profile.channel)
            if name in ("transaction.json", "previous.json"):
                require(DIGEST.fullmatch(value.get("previous_image", "")) and type(value.get("mutation_started")) is bool and isinstance(value.get("composition_hashes"), dict), "TRANSACTION_INVALID")
                validate_manifest(value.get("desired"), self.profile.channel)
        return value

    def quarantine(self, image: str) -> None:
        images = self.saved("quarantine.json").get("images", [])
        require(isinstance(images, list) and len(images) < 1024, "QUARANTINE_INVALID")
        self.save("quarantine.json", {"images": sorted(set(images + [image]))})

    def save(self, name: str, data: dict) -> None:
        private_directory(self.directory, self.uid, self.anchor)
        if name != "image.override.json":
            data = dict(data, schema_version=1, binding=self.binding())
        atomic_json(self.directory / name, data)

    def docker(self, *args: str, timeout: int = 30) -> str:
        return self.runner.run([self.profile.docker_binary, "--host", self.profile.docker_host, *args], timeout)

    def compose(self, *args: str, override: bool = True) -> str:
        argv = ["compose", "--project-name", self.profile.project, "--project-directory", str(self.profile.compose_files[0].parent)]
        for file in self.profile.compose_files:
            argv += ["--file", str(file)]
        for file in self.profile.env_files:
            argv += ["--env-file", str(file)]
        image_file = self.directory / "image.override.json"
        if override and image_file.exists():
            secure_file(image_file, self.uid, self.anchor, private=True)
            value = read_json(image_file, self.uid, self.anchor, private=True)
            require(set(value) == {"services"} and set(value["services"]) == {self.profile.service}, "OVERRIDE_SCOPE_INVALID")
            require(set(value["services"][self.profile.service]) == {"image"}, "OVERRIDE_SCOPE_INVALID")
            self.valid_image(value["services"][self.profile.service]["image"])
            argv += ["--file", str(image_file)]
        return self.docker(*argv, *args, timeout=self.profile.compose_timeout)

    def identity(self) -> dict:
        ids = self.compose("ps", "--all", "--quiet", self.profile.service).split()
        require(len(ids) == 1 and re.fullmatch(r"[a-f0-9]{12,64}", ids[0]), "EXACTLY_ONE_CONTAINER_REQUIRED")
        template = '{"id":{{json .Id}},"image":{{json .Image}},"running":{{json .State.Running}},"health":{{if .State.Health}}{{json .State.Health.Status}}{{else}}null{{end}}}'
        try:
            value = json.loads(self.docker("inspect", "--format", template, ids[0]))
        except ValueError:
            raise UpdateError("CONTAINER_IDENTITY_INVALID") from None
        require(isinstance(value, dict) and DIGEST.fullmatch(value.get("image", "")), "CONTAINER_IDENTITY_INVALID")
        return value

    def composition_hashes(self) -> dict:
        files = (*self.profile.compose_files, *self.profile.env_files)
        for file in files:
            secure_file(file, self.uid, self.anchor)
        return {str(file): hashlib.sha256(file.read_bytes()).hexdigest() for file in files}

    def hook(self, argv: tuple[str, ...], timeout: int) -> None:
        if argv:
            secure_file(Path(argv[0]), self.uid, self.anchor)
            self.runner.run(list(argv), timeout)

    def verify(self, expected: str) -> None:
        deadline = self.clock() + self.profile.health_timeout
        while True:
            actual = self.identity()
            require(actual["image"] == expected, "RUNNING_IMAGE_MISMATCH")
            if actual.get("running") and actual.get("health") in (None, "healthy"):
                break
            require(self.clock() < deadline, "CONTAINER_HEALTH_FAILED")
            self.sleep(1)
        try:
            self.hook(self.profile.verify_command, self.profile.verify_timeout)
        except UpdateError:
            raise UpdateError("APPLICATION_VERIFICATION_FAILED") from None

    def check(self, manifest: dict) -> dict:
        enrolled = self.enrollment()
        manifest = validate_manifest(manifest, self.profile.channel)
        old = self.saved("status.json")
        seen = self.saved("accepted.json").get("manifest", old.get("manifest", {}))
        if seen:
            require(manifest["generation"] >= seen["generation"], "STALE_GENERATION_REFUSED")
            if manifest["generation"] == seen["generation"]:
                require(fingerprint(manifest) == fingerprint(seen), "GENERATION_EQUIVOCATION_REFUSED")
        current = self.identity()
        if old.get("running_image") and old.get("result") in ("updated", "up_to_date", "manual_rollback"):
            require(current["image"] == old["running_image"], "RUNNING_IMAGE_DRIFT_REFUSED")
            if old.get("composition_hashes"):
                require(old["composition_hashes"] == self.composition_hashes(), "COMPOSITION_DRIFT_REFUSED")
            if old.get("override"):
                require(self.saved("image.override.json") == old["override"], "OVERRIDE_DRIFT_REFUSED")
        images = self.saved("quarantine.json").get("images", [])
        require(isinstance(images, list) and all(isinstance(v, str) and v.startswith(REPOSITORY + "@sha256:") for v in images), "QUARANTINE_INVALID")
        return {"profile": self.profile.name, "channel": self.profile.channel, "enrolled": enrolled, "desired": manifest, "running": current, "operator_hold": old.get("operator_hold", False), "quarantined": manifest["image"] in images or old.get("quarantined_image") == manifest["image"], "transaction_pending": bool(self.saved("transaction.json"))}

    @contextmanager
    def locked(self):
        path = self.profile.lock_file
        require(not path.is_symlink(), "SYMLINK_REFUSED")
        # Validate every parent, but don't mkdir system lock paths implicitly.
        require(path.parent.is_dir(), "LOCK_DIRECTORY_INVALID")
        parent = path.parent
        while parent != self.anchor:
            info = parent.lstat()
            # /run/lock can legitimately be root-owned 1777. Sticky ownership
            # protects an existing root lock; O_NOFOLLOW + fstat reject squats.
            sticky_lock_directory = parent == path.parent and bool(info.st_mode & stat.S_ISVTX)
            require(stat.S_ISDIR(info.st_mode) and info.st_uid == self.uid and (not info.st_mode & 0o022 or sticky_lock_directory), "LOCK_DIRECTORY_INVALID")
            require(parent.is_relative_to(self.anchor), "LOCK_DIRECTORY_INVALID")
            parent = parent.parent
        fd = os.open(path, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
        try:
            info = os.fstat(fd)
            require(info.st_uid == self.uid and stat.S_ISREG(info.st_mode) and not info.st_mode & 0o022, "LOCK_PERMISSIONS_INVALID")
            try:
                fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError:
                raise UpdateError("UPDATE_LOCKED") from None
            yield
        finally:
            os.close(fd)

    def recreate(self) -> None:
        self.compose("up", "--detach", "--no-deps", "--no-build", "--pull", "never", "--force-recreate", self.profile.service)

    def valid_image(self, image: str) -> None:
        require(isinstance(image, str), "IMAGE_REFERENCE_INVALID")
        require(DIGEST.fullmatch(image) or (image.startswith(REPOSITORY + "@") and DIGEST.fullmatch(image.removeprefix(REPOSITORY + "@"))), "IMAGE_REFERENCE_INVALID")

    def image_override(self, image: str) -> None:
        self.valid_image(image)
        self.save("image.override.json", {"services": {self.profile.service: {"image": image}}})

    def local_image(self, image: str) -> str:
        value = self.docker("image", "inspect", "--format", "{{.Id}}", image).strip()
        require(value == image, "ROLLBACK_IMAGE_NOT_CACHED")
        return value

    def retain(self, image: str) -> None:
        self.local_image(image)
        name = "ail-rollback-" + self.profile.name + "-" + image[7:23]
        ids = self.docker("ps", "--all", "--filter", "name=^/" + re.escape(name) + "$", "--format", "{{.ID}}").split()
        if not ids:
            volumes = self.docker("image", "inspect", "--format", "{{json .Config.Volumes}}", image).strip()
            require(volumes in ("null", "{}"), "ROLLBACK_GUARD_IMAGE_VOLUMES_REFUSED")
            container = self.docker("create", "--name", name, "--label", "io.switchailocal.rollback=" + self.profile.name, "--network", "none", "--entrypoint", "/bin/true", "--pull", "never", image).strip()
            require(self.guard_image(container) == image, "ROLLBACK_GUARD_INVALID")
        else:
            require(len(ids) == 1, "ROLLBACK_GUARD_INVALID")
            require(self.guard_image(ids[0]) == image, "ROLLBACK_GUARD_INVALID")

    def guard_image(self, container: str) -> str:
        require(re.fullmatch(r"[a-f0-9]{12,64}", container), "ROLLBACK_GUARD_INVALID")
        template = '{"image":{{json .Image}},"owner":{{json (index .Config.Labels "io.switchailocal.rollback")}},"name":{{json .Name}},"started":{{json .State.StartedAt}},"mounts":{{len .Mounts}},"compose":{{json (index .Config.Labels "com.docker.compose.project")}}}'
        try:
            value = json.loads(self.docker("inspect", "--format", template, container))
        except ValueError:
            raise UpdateError("ROLLBACK_GUARD_INVALID") from None
        image = value.get("image", "")
        require(DIGEST.fullmatch(image) and value.get("owner") == self.profile.name and value.get("name") == "/ail-rollback-" + self.profile.name + "-" + image[7:23], "ROLLBACK_GUARD_INVALID")
        require(value.get("started") == "0001-01-01T00:00:00Z" and value.get("mounts") == 0 and not value.get("compose"), "ROLLBACK_GUARD_INVALID")
        return image

    def cleanup_guards(self, keep: str) -> None:
        ids = self.docker("ps", "--all", "--filter", "label=io.switchailocal.rollback=" + self.profile.name, "--format", "{{.ID}}").split()
        for container in ids:
            if self.guard_image(container) != keep:
                self.docker("rm", container)

    def restore(self, transaction: dict) -> None:
        require(transaction["composition_hashes"] == self.composition_hashes(), "COMPOSITION_DRIFT_ROLLBACK_HELD")
        require(DIGEST.fullmatch(transaction["previous_image"]), "ROLLBACK_IMAGE_INVALID")
        self.local_image(transaction["previous_image"])
        self.image_override(transaction["previous_image"])
        self.recreate()
        self.verify(transaction["previous_image"])

    def recover(self) -> None:
        transaction = self.saved("transaction.json")
        if not transaction:
            return
        self.quarantine(transaction["desired"]["image"])
        try:
            if transaction.get("mutation_started", True):
                self.restore(transaction)
            self.hook(self.profile.post_update_command, self.profile.hook_timeout)
        except UpdateError:
            self.save("status.json", {"result": "recovery_failed", "checked_at": utc(), "quarantined_image": transaction["desired"]["image"], "manifest": transaction["desired"]})
            raise UpdateError("INTERRUPTED_UPDATE_RECOVERY_FAILED") from None
        self.save("status.json", {"result": "interrupted_update_rolled_back", "checked_at": utc(), "operator_hold": transaction.get("operator_hold", False), "quarantined_image": transaction["desired"]["image"], "manifest": transaction["desired"]})
        (self.directory / "transaction.json").unlink()
        raise UpdateError("INTERRUPTED_UPDATE_ROLLED_BACK")

    def apply(self, manifest: dict | None = None, retry: bool = False) -> dict:
        require(self.profile.allow_recreate, "RECREATE_NOT_ACKNOWLEDGED")
        with self.locked():
            self.enrollment(required=True)
            self.recover()
            if manifest is None:
                manifest = fetch_manifest(self.profile.manifest_url, self.profile.channel)
            result = self.check(manifest)
            require(retry or not result["quarantined"], "IMAGE_QUARANTINED")
            require(retry or not result["operator_hold"], "OPERATOR_HOLD")
            self.save("accepted.json", {"manifest": manifest})
            services = self.compose("config", "--services").split()
            require(self.profile.service in services, "COMPOSE_SERVICE_MISSING")
            current = result["running"]
            old = self.saved("status.json")
            if (old.get("manifest", {}).get("image") == manifest["image"]
                    and old.get("running_image") == current["image"]
                    and current.get("running") and current.get("health") in (None, "healthy")
                    and old.get("result") in ("updated", "up_to_date")):
                status = dict(old, result="up_to_date", checked_at=utc(), manifest=manifest)
                self.save("status.json", status)
                return status
            self.verify(current["image"])
            self.docker("pull", manifest["image"], timeout=self.profile.pull_timeout)
            target = self.docker("image", "inspect", "--format", "{{.Id}}", manifest["image"]).strip()
            require(DIGEST.fullmatch(target), "TARGET_IMAGE_INVALID")
            revision = self.docker("image", "inspect", "--format", '{{index .Config.Labels "org.opencontainers.image.revision"}}', manifest["image"]).strip()
            require(revision == manifest["source_commit"], "TARGET_SOURCE_MISMATCH")
            if current["image"] == target:
                status = {"result": "up_to_date", "checked_at": utc(), "manifest": manifest, "running_image": target, "profile": self.profile.name, "composition_hashes": self.composition_hashes(), "override": self.saved("image.override.json")}
                self.save("status.json", status)
                if retry:
                    images = self.saved("quarantine.json").get("images", [])
                    self.save("quarantine.json", {"images": [v for v in images if v != manifest["image"]]})
                return status
            self.retain(current["image"])
            transaction = {"schema_version": 1, "started_at": utc(), "previous_image": current["image"], "desired": manifest, "composition_hashes": self.composition_hashes(), "mutation_started": False}
            self.save("transaction.json", transaction)
            self.save("status.json", {"result": "updating", "checked_at": utc(), "manifest": manifest})
            try:
                self.hook(self.profile.pre_update_command, self.profile.hook_timeout)
                require(transaction["composition_hashes"] == self.composition_hashes(), "COMPOSITION_DRIFT_REFUSED")
                transaction["mutation_started"] = True
                self.save("transaction.json", transaction)
                self.image_override(manifest["image"])
                self.recreate()
                self.verify(target)
                self.hook(self.profile.post_update_command, self.profile.hook_timeout)
            except UpdateError as error:
                self.quarantine(manifest["image"])
                rolled_back = False
                try:
                    if transaction["mutation_started"]:
                        self.restore(transaction)
                    self.hook(self.profile.post_update_command, self.profile.hook_timeout)
                    rolled_back = True
                except UpdateError:
                    pass
                status = {"result": "rolled_back" if rolled_back else "rollback_failed", "checked_at": utc(), "manifest": manifest, "error": str(error), "quarantined_image": manifest["image"], "previous_image": current["image"], "profile": self.profile.name}
                self.save("status.json", status)
                if rolled_back:
                    (self.directory / "transaction.json").unlink()
                raise UpdateError("UPDATE_FAILED_ROLLBACK_OK" if rolled_back else "UPDATE_FAILED_ROLLBACK_FAILED") from None
            self.save("previous.json", transaction)
            status = {"result": "updated", "checked_at": utc(), "manifest": manifest, "running_image": target, "previous_image": current["image"], "profile": self.profile.name, "composition_hashes": self.composition_hashes(), "override": self.saved("image.override.json")}
            self.save("status.json", status)
            (self.directory / "transaction.json").unlink()
            if retry:
                images = self.saved("quarantine.json").get("images", [])
                self.save("quarantine.json", {"images": [v for v in images if v != manifest["image"]]})
            self.cleanup_guards(current["image"])
            return status

    def rollback(self) -> dict:
        require(self.profile.allow_recreate, "RECREATE_NOT_ACKNOWLEDGED")
        with self.locked():
            self.enrollment(required=True)
            self.recover()
            previous = self.saved("previous.json")
            require(bool(previous), "NO_ROLLBACK_TARGET")
            previous = dict(previous, desired=self.saved("accepted.json").get("manifest", previous["desired"]), operator_hold=True)
            # Journal this operation too, so an interrupted rollback is recoverable.
            self.save("transaction.json", previous)
            self.quarantine(previous["desired"]["image"])
            self.hook(self.profile.pre_update_command, self.profile.hook_timeout)
            self.restore(previous)
            self.hook(self.profile.post_update_command, self.profile.hook_timeout)
            result = {"result": "manual_rollback", "checked_at": utc(), "manifest": previous["desired"], "operator_hold": True, "quarantined_image": previous["desired"]["image"], "running_image": previous["previous_image"], "profile": self.profile.name, "composition_hashes": self.composition_hashes(), "override": self.saved("image.override.json")}
            self.save("status.json", result)
            (self.directory / "transaction.json").unlink()
            return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--profile", type=Path, required=True)
    parser.add_argument("command", choices=("check", "enroll", "apply", "retry", "rollback", "status", "validate-profile"))
    args = parser.parse_args()
    try:
        require(os.geteuid() == 0, "ROOT_REQUIRED")
        profile = load_profile(args.profile)
        if args.command == "validate-profile":
            output = {"profile": profile.name, "valid": True, "updater_version": UPDATER_VERSION}
        else:
            engine = Engine(profile)
            if args.command == "status":
                engine.enrollment()
                output = {"profile": profile.name, "saved": engine.saved("status.json"), "running": engine.identity(), "transaction_pending": bool(engine.saved("transaction.json"))}
            elif args.command == "rollback":
                output = engine.rollback()
            elif args.command == "enroll":
                output = engine.enroll()
            else:
                output = engine.check(fetch_manifest(profile.manifest_url, profile.channel)) if args.command == "check" else engine.apply(retry=args.command == "retry")
        print(json.dumps(output, sort_keys=True))
        return 0
    except UpdateError as error:
        print(json.dumps({"ok": False, "code": str(error)}), file=sys.stderr)
        return 1
    except Exception:
        print(json.dumps({"ok": False, "code": "INTERNAL_ERROR"}), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
