import copy
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from managed_update import UpdateError, REPOSITORY, atomic_json
from promote import prepare
from probe import Probe, config

HERE = Path(__file__).resolve().parents[1]
SHA = 'b' * 40
IMAGE = REPOSITORY + '@sha256:' + 'a' * 64


class PackageTests(unittest.TestCase):
    def test_promotion_is_explicit_monotonic_and_stable_matches_canary(self):
        canary = prepare('canary', 2, '0.5.27', IMAGE, SHA, '')
        stable = prepare('stable', 1, '0.5.27', IMAGE, SHA, 'https://example.invalid/evidence', canary=canary)
        self.assertEqual(stable['image'], canary['image'])
        with self.assertRaisesRegex(UpdateError, 'CANARY_MANIFEST_REQUIRED'):
            prepare('stable', 1, '0.5.27', IMAGE, SHA, 'https://example.invalid/evidence')
        with self.assertRaisesRegex(UpdateError, 'STABLE_MUST_MATCH_CANARY'):
            prepare('stable', 1, '0.5.28', IMAGE, SHA, 'https://example.invalid/evidence', canary=canary)
        with self.assertRaisesRegex(UpdateError, 'PROMOTION_GENERATION_MUST_INCREASE'):
            prepare('canary', 2, '0.5.27', IMAGE, SHA, '', previous=canary)
        rollback = prepare('canary', 3, '0.5.26', IMAGE, SHA, '', previous=canary)
        self.assertEqual(rollback['version'], '0.5.26')

    def test_inert_installer_in_temporary_root(self):
        with tempfile.TemporaryDirectory() as d:
            result = subprocess.run(['bash', str(HERE / 'install.sh'), d], capture_output=True, text=True, timeout=10)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertTrue((Path(d) / 'opt/switchailocal-updater/v1/managed_update.py').exists())
            self.assertFalse((Path(d) / 'etc/systemd/system/timers.target.wants').exists())
            self.assertEqual(list((Path(d) / 'etc/switchailocal-updater/enrolled').iterdir()), [])
            self.assertNotIn('systemctl', (HERE / 'install.sh').read_text())
            again = subprocess.run(['bash', str(HERE / 'install.sh'), d], capture_output=True, timeout=10)
            self.assertEqual(again.returncode, 0)

    def test_install_refuses_symlink_destination(self):
        with tempfile.TemporaryDirectory() as d:
            (Path(d) / 'opt').symlink_to('/tmp')
            result = subprocess.run(['bash', str(HERE / 'install.sh'), d], capture_output=True, timeout=10)
            self.assertNotEqual(result.returncode, 0)

    def test_no_tag_to_fleet_pin_writer(self):
        workflows = HERE.parents[1] / '.github/workflows'
        self.assertFalse((workflows / 'bump-infra-version.yml').exists())
        for path in workflows.glob('*.yml'):
            self.assertNotIn('INFRA_REPO_PAT', path.read_text())
            self.assertNotIn('switchailocal-version.txt', path.read_text())
        build = (workflows / 'build.yml').read_text()
        self.assertIn('ref: ${{ inputs.ref || github.ref }}', build)
        self.assertIn('COMMIT=${{ steps.source.outputs.sha }}', build)
        self.assertNotIn('value=stable', build)
        self.assertNotIn('value=latest', build)

    def test_contract_ci_avoids_zero_base_on_new_branch_push(self):
        workflow = (HERE.parents[1] / '.github/workflows/managed-server.yml').read_text()
        self.assertIn('push:\n    branches: [main]', workflow)
        self.assertIn('pull_request:', workflow)

    def test_profile_examples_do_not_enable_recreation(self):
        profile = json.loads((HERE / 'examples/profile.json').read_text())
        self.assertFalse(profile['allow_recreate'])
        self.assertEqual(profile['lock_file'], '/run/lock/wannolot-gitops.lock')

    def test_probe_config_requires_private_key_and_trusted_transport(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d).resolve()
            key = root / 'key'
            key.write_text('synthetic-key')
            key.chmod(0o600)
            path = root / 'probe.json'
            settings = {'schema_version': 1, 'base_url': 'http://127.0.0.1:18080/v1', 'key_file': str(key), 'chat_model': 'operator-chat', 'vision_model': 'operator-vision'}
            atomic_json(path, settings)
            self.assertEqual(config(path, os.getuid(), root)['chat_model'], 'operator-chat')
            atomic_json(path, dict(settings, base_url='http://example.com/v1'))
            with self.assertRaisesRegex(UpdateError, 'PROBE_TLS_REQUIRED'):
                config(path, os.getuid(), root)
            atomic_json(path, settings)
            key.chmod(0o644)
            with self.assertRaisesRegex(UpdateError, 'PRIVATE_FILE_REQUIRED'):
                config(path, os.getuid(), root)

    def test_probe_exercises_replay_vision_stream_and_embeddings(self):
        class Fake(Probe):
            def __init__(self):
                self.settings = {'chat_model': 'chat', 'vision_model': 'vision', 'max_tokens': 2048, 'embedding_model': 'local', 'embedding_dimensions': 2}
                self.requests = []
            def request(self, endpoint, payload, stream=False):
                self.requests.append((endpoint, payload))
                if endpoint == '/embeddings':
                    return {'data': [{'embedding': [0.0, 1.0]}]}
                if payload.get('stream'):
                    return 'data: {"choices":[{"delta":{"content":"AIL_OK"}}]}\n\ndata: [DONE]\n\n'
                if payload.get('tool_choice'):
                    message = {'role': 'assistant', 'tool_calls': [{'id': 'call_synthetic', 'type': 'function', 'function': {'name': 'ail_echo', 'arguments': '{"text":"AIL_TOOL_OK"}'}}]}
                else:
                    message = {'role': 'assistant', 'content': 'red' if payload['model'] == 'vision' else ('AIL_TOOL_OK' if payload['messages'][-1]['role'] == 'tool' else 'AIL_OK')}
                return {'choices': [{'message': message}]}
        probe = Fake()
        self.assertTrue(probe.run()['ok'])
        self.assertEqual(len(probe.requests), 7)
        self.assertEqual(probe.requests[4][1]['model'], 'vision')


if __name__ == '__main__':
    unittest.main()
