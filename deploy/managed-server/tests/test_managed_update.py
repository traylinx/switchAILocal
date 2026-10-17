import copy
import fcntl
import json
import os
import shutil
import subprocess
import time
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from managed_update import Engine, Profile, REPOSITORY, UpdateError, atomic_json, load_profile, validate_manifest, Runner, NoRedirects, valid_https

OLD = "sha256:" + "1" * 64
NEW = "sha256:" + "2" * 64
IMAGE = REPOSITORY + "@sha256:" + "a" * 64


def manifest(**changes):
    value = {"schema_version": 1, "channel": "canary", "generation": 1, "version": "0.5.27", "image": IMAGE, "source_commit": "b" * 40, "promoted_at": "2026-10-17T10:00:00Z", "verification_ref": ""}
    value.update(changes)
    return value


class FakeRunner:
    def __init__(self, profile):
        self.profile = profile
        self.current = OLD
        self.calls = []
        self.fail_pull = False
        self.fail_new = False
        self.fail_rollback = False
        self.fail_baseline = False
        self.crash = False
        self.pre_modify = False
        self.creates = []
        self.verify_count = 0
        self.guards = {}
        self.missing_old = False
        self.bad_guard = False

    def run(self, argv, timeout):
        self.calls.append((list(argv), timeout))
        if argv[0] == self.profile.verify_command[0]:
            self.verify_count += 1
            if (self.fail_new and self.current == NEW) or (self.fail_baseline and self.current == OLD):
                raise UpdateError("COMMAND_FAILED")
            return "private verification output is never stored"
        if self.profile.pre_update_command and argv[0] == self.profile.pre_update_command[0]:
            if self.pre_modify:
                self.profile.compose_files[0].write_text("changed by competing operation")
            return ""
        args = argv[3:]
        if args[:1] == ["pull"]:
            if self.fail_pull:
                raise UpdateError("COMMAND_FAILED")
            return ""
        if args[:2] == ["image", "inspect"]:
            if "Config.Volumes" in args[3]:
                return "null"
            if "org.opencontainers.image.revision" in args[3]:
                return "b" * 40
            if args[-1] == OLD:
                if self.missing_old:
                    raise UpdateError("COMMAND_FAILED")
                return OLD
            return NEW
        if args[:1] == ["ps"]:
            if "name=^/" in " ".join(args):
                return ""
            return "\n".join(self.guards)
        if args[:1] == ["create"]:
            ident = str(len(self.guards) + 6) * 64
            self.guards[ident] = args[-1]
            return ident
        if args[:1] == ["rm"]:
            del self.guards[args[-1]]
            return ""
        if args[:1] == ["inspect"]:
            if args[-1] in self.guards:
                image = self.guards[args[-1]]
                return json.dumps({'image': image, 'owner': self.profile.name, 'name': '/customer-container' if self.bad_guard else '/ail-rollback-' + self.profile.name + '-' + image[7:23], 'started': '0001-01-01T00:00:00Z', 'mounts': 0, 'compose': ''})
            return json.dumps({"id": "3" * 64, "image": self.current, "running": True, "health": "healthy"})
        if args[:1] != ["compose"]:
            raise AssertionError(argv)
        rest = args[1:]
        files = []
        while rest and rest[0] in ("--project-name", "--project-directory", "--file", "--env-file"):
            if rest[0] == "--file":
                files.append(rest[1])
            rest = rest[2:]
        if rest[:1] == ["ps"]:
            return "3" * 64
        if rest[:1] == ["config"]:
            return self.profile.service + "\nuntouched-database\n"
        if rest[:1] == ["up"]:
            assert rest[-1] == self.profile.service
            assert "--no-deps" in rest and "--no-build" in rest
            assert rest[rest.index("--pull") + 1] == "never"
            override = json.loads(Path(files[-1]).read_text())
            assert set(override["services"]) == {self.profile.service}
            image = override["services"][self.profile.service]["image"]
            target = NEW if image == IMAGE else image
            if target == OLD and self.fail_rollback:
                raise UpdateError("COMMAND_FAILED")
            self.current = target
            self.creates.append(target)
            if self.crash and target == NEW:
                self.crash = False
                raise SystemExit("simulated power loss")
            return ""
        raise AssertionError(argv)


class UpdateTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.root = Path(self.temporary.name).resolve()
        self.compose = self.root / "compose.json"
        self.compose.write_text('{"services":{"ail":{"image":"old"}}}')
        self.verify = self.root / "verify"
        self.verify.write_text("#!/bin/sh\nexit 0\n")
        self.verify.chmod(0o700)
        self.profile = Profile(name="test", channel="canary", manifest_url="https://example.invalid/canary.json", compose_files=(self.compose,), project="test-project", service="ail", verify_command=(str(self.verify),), lock_file=self.root / "update.lock", allow_recreate=True)
        self.runner = FakeRunner(self.profile)
        self.engine = Engine(self.profile, runner=self.runner, state_root=self.root / "state", uid=os.getuid(), anchor=self.root, enroll_root=self.root / "config" / "enrolled")
        self.engine.enroll()
        self.runner.calls.clear()
        self.runner.verify_count = 0

    def tearDown(self):
        self.temporary.cleanup()

    def test_check_is_runtime_and_filesystem_read_only(self):
        before = {str(p): p.read_bytes() for p in self.root.rglob("*") if p.is_file()}
        result = self.engine.check(manifest())
        self.assertEqual(result["running"]["image"], OLD)
        self.assertEqual(before, {str(p): p.read_bytes() for p in self.root.rglob("*") if p.is_file()})
        self.assertEqual(self.runner.creates, [])
        self.assertEqual(self.runner.verify_count, 0)

    def test_success_changes_only_selected_service_image(self):
        before = self.compose.read_bytes()
        result = self.engine.apply(manifest())
        self.assertEqual(result["result"], "updated")
        self.assertEqual(self.runner.current, NEW)
        self.assertEqual(self.runner.creates, [NEW])
        self.assertEqual(self.compose.read_bytes(), before)
        self.assertEqual(self.runner.verify_count, 2)
        self.assertFalse((self.engine.directory / "transaction.json").exists())
        self.assertEqual(self.engine.saved("previous.json")["previous_image"], OLD)
        self.assertEqual((self.engine.directory / "status.json").stat().st_mode & 0o777, 0o600)

    def test_unchanged_tick_has_no_paid_probe_pull_or_restart(self):
        self.engine.apply(manifest())
        self.runner.calls.clear()
        self.runner.verify_count = 0
        result = self.engine.apply(manifest())
        self.assertEqual(result["result"], "up_to_date")
        self.assertEqual(self.runner.verify_count, 0)
        self.assertFalse(any("pull" in call for call, _ in self.runner.calls))
        self.assertEqual(self.runner.creates, [NEW])

    def test_pull_failure_preserves_original_container(self):
        self.runner.fail_pull = True
        with self.assertRaisesRegex(UpdateError, "COMMAND_FAILED"):
            self.engine.apply(manifest())
        self.assertEqual(self.runner.current, OLD)
        self.assertEqual(self.runner.creates, [])
        self.assertFalse((self.engine.directory / "transaction.json").exists())

    def test_failed_baseline_does_not_pull_or_restart(self):
        self.runner.fail_baseline = True
        with self.assertRaisesRegex(UpdateError, "APPLICATION_VERIFICATION_FAILED"):
            self.engine.apply(manifest())
        self.assertEqual(self.runner.creates, [])
        self.assertFalse(any("pull" in call for call, _ in self.runner.calls))

    def test_failed_application_probe_restores_exact_previous_id(self):
        self.runner.fail_new = True
        with self.assertRaisesRegex(UpdateError, "UPDATE_FAILED_ROLLBACK_OK"):
            self.engine.apply(manifest())
        self.assertEqual(self.runner.creates, [NEW, OLD])
        self.assertEqual(self.runner.current, OLD)
        self.assertEqual(self.engine.saved("status.json")["quarantined_image"], IMAGE)
        with self.assertRaisesRegex(UpdateError, "IMAGE_QUARANTINED"):
            self.engine.apply(manifest())
        self.assertEqual(self.runner.creates, [NEW, OLD])

    def test_explicit_retry_retests_quarantined_image(self):
        self.runner.fail_new = True
        with self.assertRaises(UpdateError):
            self.engine.apply(manifest())
        self.runner.fail_new = False
        self.assertEqual(self.engine.apply(manifest(), retry=True)["result"], "updated")

    def test_rollback_failure_keeps_recovery_transaction(self):
        self.runner.fail_new = True
        self.runner.fail_rollback = True
        with self.assertRaisesRegex(UpdateError, "UPDATE_FAILED_ROLLBACK_FAILED"):
            self.engine.apply(manifest())
        self.assertEqual(self.engine.saved("status.json")["result"], "rollback_failed")
        self.assertTrue(self.engine.saved("transaction.json"))
        with self.assertRaisesRegex(UpdateError, "INTERRUPTED_UPDATE_RECOVERY_FAILED"):
            self.engine.apply(manifest())

    def test_interrupted_update_is_recovered_before_any_new_update(self):
        self.runner.crash = True
        with self.assertRaises(SystemExit):
            self.engine.apply(manifest())
        self.assertEqual(self.runner.current, NEW)
        self.assertTrue(self.engine.saved("transaction.json"))
        with self.assertRaisesRegex(UpdateError, "INTERRUPTED_UPDATE_ROLLED_BACK"):
            self.engine.apply(manifest())
        self.assertEqual(self.runner.current, OLD)
        self.assertFalse(self.engine.saved("transaction.json"))
        self.assertEqual(self.runner.creates, [NEW, OLD])

    def test_manual_rollback_is_pinned_and_quarantines_desired_image(self):
        self.engine.apply(manifest())
        self.assertEqual(self.engine.rollback()["result"], "manual_rollback")
        self.assertEqual(self.runner.current, OLD)
        with self.assertRaisesRegex(UpdateError, "IMAGE_QUARANTINED"):
            self.engine.apply(manifest())

    def test_lock_contention_causes_no_mutation(self):
        with self.engine.locked():
            with self.assertRaisesRegex(UpdateError, "UPDATE_LOCKED"):
                self.engine.apply(manifest())
        self.assertEqual(self.runner.calls, [])

    def test_legacy_root_owned_readable_lock_is_compatible(self):
        self.profile.lock_file.touch(mode=0o644)
        self.profile.lock_file.chmod(0o644)
        with self.engine.locked():
            pass

    def test_recreate_requires_explicit_profile_acknowledgement(self):
        object.__setattr__(self.profile, "allow_recreate", False)
        with self.assertRaisesRegex(UpdateError, "RECREATE_NOT_ACKNOWLEDGED"):
            self.engine.apply(manifest())
        self.assertEqual(self.runner.calls, [])

    def test_stale_and_equivocating_manifest_rejected(self):
        self.engine.apply(manifest(generation=2))
        with self.assertRaisesRegex(UpdateError, "STALE_GENERATION_REFUSED"):
            self.engine.check(manifest())
        with self.assertRaisesRegex(UpdateError, "GENERATION_EQUIVOCATION_REFUSED"):
            self.engine.check(manifest(generation=2, version="0.5.28"))

    def test_image_override_cannot_add_other_services_or_options(self):
        self.engine.save("image.override.json", {"services": {"database": {"image": OLD}}})
        with self.assertRaisesRegex(UpdateError, "OVERRIDE_SCOPE_INVALID"):
            self.engine.identity()

    def test_composition_drift_before_mutation_does_not_restart(self):
        pre = self.root / "pre"
        pre.write_text("#!/bin/sh\nexit 0\n")
        pre.chmod(0o700)
        object.__setattr__(self.profile, "pre_update_command", (str(pre),))
        self.runner.pre_modify = True
        with self.assertRaisesRegex(UpdateError, "UPDATE_FAILED_ROLLBACK_OK"):
            self.engine.apply(manifest())
        self.assertEqual(self.runner.creates, [])

    def test_apply_requires_explicit_enrollment(self):
        self.engine.marker.unlink()
        with self.assertRaisesRegex(UpdateError, "EXPLICIT_ENROLLMENT_REQUIRED"):
            self.engine.apply(manifest())

    def test_enrollment_marker_detects_state_directory_loss(self):
        shutil.rmtree(self.engine.directory)
        with self.assertRaisesRegex(UpdateError, "ENROLLED_STATE_MISSING"):
            self.engine.check(manifest())
        with self.assertRaises(UpdateError):
            self.engine.enroll()

    def test_old_or_corrupt_schema_holds_before_docker(self):
        path = self.engine.directory / "status.json"
        value = json.loads(path.read_text())
        value['schema_version'] = 2
        atomic_json(path, value)
        with self.assertRaisesRegex(UpdateError, "STATE_SCHEMA_OR_BINDING_INVALID"):
            self.engine.apply(manifest())
        self.assertEqual(self.runner.calls, [])

    def test_manual_hold_blocks_even_different_digest(self):
        self.engine.apply(manifest())
        self.engine.rollback()
        with self.assertRaisesRegex(UpdateError, 'OPERATOR_HOLD'):
            self.engine.apply(manifest(generation=2, image=REPOSITORY + '@sha256:' + 'c' * 64))

    def test_quarantine_survives_new_generation_of_same_digest(self):
        self.runner.fail_new = True
        with self.assertRaises(UpdateError):
            self.engine.apply(manifest())
        with self.assertRaisesRegex(UpdateError, 'IMAGE_QUARANTINED'):
            self.engine.apply(manifest(generation=2))

    def test_accepted_generation_survives_failed_preflight(self):
        self.runner.fail_pull = True
        with self.assertRaises(UpdateError):
            self.engine.apply(manifest(generation=3))
        with self.assertRaisesRegex(UpdateError, 'STALE_GENERATION_REFUSED'):
            self.engine.check(manifest(generation=2))

    def test_missing_prior_image_cannot_mutate(self):
        self.runner.missing_old = True
        with self.assertRaises(UpdateError):
            self.engine.apply(manifest())
        self.assertEqual(self.runner.creates, [])

    def test_guard_never_started_no_compose_labels_no_mounts(self):
        self.engine.apply(manifest())
        guard = next(call for call, _ in self.runner.calls if 'create' in call)
        self.assertIn('--network', guard)
        self.assertIn('none', guard)
        self.assertNotIn('start', guard)
        self.assertFalse(any('com.docker.compose' in arg for arg in guard))
        self.assertNotIn('--mount', guard)
        self.assertEqual(list(self.runner.guards.values()), [OLD])

    def test_same_label_on_unrelated_container_never_authorizes_removal(self):
        self.runner.guards['6' * 64] = OLD
        self.runner.bad_guard = True
        with self.assertRaisesRegex(UpdateError, 'ROLLBACK_GUARD_INVALID'):
            self.engine.cleanup_guards(NEW)
        self.assertEqual(list(self.runner.guards.values()), [OLD])
        self.assertFalse(any('rm' in call for call, _ in self.runner.calls))

    def test_lost_high_water_file_fails_closed(self):
        (self.engine.directory / 'accepted.json').unlink()
        with self.assertRaisesRegex(UpdateError, 'ENROLLED_STATE_MISSING'):
            self.engine.apply(manifest())

    def test_env_file_bytes_hashed_not_copied_and_order_preserved(self):
        env = self.root / 'private.env'
        env.write_text('PRIVATE=not-exported-to-evidence')
        env.chmod(0o600)
        object.__setattr__(self.profile, 'env_files', (env,))
        # Changing a binding after enrollment requires explicit operator reconciliation.
        with self.assertRaisesRegex(UpdateError, 'ENROLLMENT_BINDING_MISMATCH'):
            self.engine.check(manifest())
        self.assertNotIn('not-exported', json.dumps(self.engine.composition_hashes()))

    def test_override_drift_detected_before_pull(self):
        self.engine.apply(manifest())
        self.engine.image_override(OLD)
        with self.assertRaisesRegex(UpdateError, 'OVERRIDE_DRIFT_REFUSED'):
            self.engine.apply(manifest())



class ContractTests(unittest.TestCase):
    def test_manifest_rejects_untrusted_or_mutable_targets(self):
        cases = [
            {"image": REPOSITORY + ":latest"},
            {"image": "evil.invalid/router@sha256:" + "a" * 64},
            {"source_commit": "main"}, {"generation": True}, {"generation": 0},
            {"version": "bad"}, {"channel": "stable"}, {"command": "run-me"},
            {"promoted_at": "2026-10-17"}, {"verification_ref": "https://user:secret@example.invalid/x"},
        ]
        for change in cases:
            with self.subTest(change=change), self.assertRaises(UpdateError):
                validate_manifest(manifest(**change), "canary")

    def test_stable_requires_non_prerelease_and_verification_reference(self):
        with self.assertRaisesRegex(UpdateError, "STABLE_VERIFICATION_REQUIRED"):
            validate_manifest(manifest(channel="stable"), "stable")
        with self.assertRaisesRegex(UpdateError, "STABLE_PRERELEASE_REFUSED"):
            validate_manifest(manifest(channel="stable", version="0.5.27-rc.1", verification_ref="https://example.invalid/proof"), "stable")
        self.assertEqual(validate_manifest(manifest(channel="stable", verification_ref="https://example.invalid/proof"), "stable")["channel"], "stable")

    def test_profile_is_private_root_trusted_and_has_no_secret_fields(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d).resolve()
            compose = root / "compose.json"
            compose.write_text("{}")
            docker = root / "docker"
            docker.write_text("#!/bin/sh\n")
            docker.chmod(0o700)
            path = root / "profile.json"
            data = {"schema_version": 1, "name": "one", "channel": "stable", "manifest_url": "https://example.invalid/stable.json", "compose_files": [str(compose)], "project": "p", "service": "ail", "verify_command": [str(docker), "verify"], "lock_file": str(root / "lock"), "docker_binary": str(docker)}
            atomic_json(path, data)
            self.assertEqual(load_profile(path, os.getuid(), root).name, "one")
            for field, value in [("api_key", "not-allowed"), ("docker_host", "tcp://other-server:2375"), ("allow_recreate", "yes")]:
                bad = dict(data, **{field: value})
                atomic_json(path, bad)
                with self.subTest(field=field), self.assertRaises(UpdateError):
                    load_profile(path, os.getuid(), root)
            atomic_json(path, data)
            path.chmod(0o644)
            with self.assertRaisesRegex(UpdateError, "PRIVATE_FILE_REQUIRED"):
                load_profile(path, os.getuid(), root)

    def test_symlinked_profile_refused(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d).resolve()
            real = root / "real.json"
            atomic_json(real, {})
            link = root / "profile.json"
            link.symlink_to(real)
            with self.assertRaisesRegex(UpdateError, "SYMLINK_REFUSED"):
                load_profile(link, os.getuid(), root)


class ProcessTests(unittest.TestCase):
    def test_environment_and_cwd_are_controlled(self):
        os.environ['UNTRUSTED_ENV_TEST'] = 'must-not-inherit'
        try:
            output = Runner(Path('/')).run([sys.executable, '-c', 'import os,json; print(json.dumps([os.getcwd(),os.environ.get("UNTRUSTED_ENV_TEST")]))'], 5)
            self.assertEqual(json.loads(output), ['/', None])
        finally:
            del os.environ['UNTRUSTED_ENV_TEST']

    def test_timeout_kills_process_group_without_raw_output(self):
        start = time.monotonic()
        with self.assertRaisesRegex(UpdateError, 'COMMAND_TIMEOUT'):
            Runner().run([sys.executable, '-c', 'import time; print("secret",flush=True); time.sleep(30)'], 1)
        self.assertLess(time.monotonic() - start, 4)

    def test_output_limit_has_safe_error(self):
        with self.assertRaisesRegex(UpdateError, 'COMMAND_OUTPUT_LIMIT'):
            Runner().run([sys.executable, '-c', 'import sys; sys.stdout.write("x"*2000000)'], 5)

    def test_manifest_zero_redirects(self):
        with self.assertRaisesRegex(UpdateError, 'MANIFEST_REDIRECT_REFUSED'):
            NoRedirects().redirect_request(None, None, None, None, None, None)
        for url in ['http://example.com/x', 'https://user:password@example.com/x', 'https://example.com/x?token=a']:
            with self.assertRaises(UpdateError):
                valid_https(url, 'REFUSED')


if __name__ == "__main__":
    unittest.main()
