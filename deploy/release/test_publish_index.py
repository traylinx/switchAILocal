import copy
from pathlib import Path
import sys
import unittest
sys.path.insert(0, str(Path(__file__).resolve().parent))
from publish_index import command

SOURCE = 'a' * 40
RECORDS = [dict(arch=a, source=SOURCE, version='0.6.2', digest='sha256:' + d * 64) for a, d in [('amd64', '1'), ('arm64', '2')]]


class PublicationTests(unittest.TestCase):
    def test_exact_source_and_version_tags_only(self):
        argv = command(RECORDS, SOURCE)
        self.assertIn('ghcr.io/traylinx/switchailocal:sha-' + SOURCE, argv)
        self.assertIn('ghcr.io/traylinx/switchailocal:0.6.2', argv)
        self.assertNotIn('ghcr.io/traylinx/switchailocal:latest', argv)
        self.assertNotIn('ghcr.io/traylinx/switchailocal:stable', argv)

    def test_native_workflow_preserves_scan_and_never_uses_emulation(self):
        workflow = (Path(__file__).resolve().parents[2] / '.github/workflows/build.yml').read_text()
        self.assertIn('runner: ubuntu-24.04-arm', workflow)
        self.assertIn('platforms: linux/${{ matrix.arch }}', workflow)
        self.assertIn('needs: [security-gate, build]', workflow)
        self.assertIn('ref: ${{ needs.security-gate.outputs.source_commit }}', workflow)
        self.assertIn('Native image startup smoke', workflow)
        self.assertIn('--entrypoint /app/switchAILocal', workflow)
        self.assertNotIn('setup-qemu', workflow)

    def test_rejects_mixed_incomplete_or_unscanned_metadata(self):
        for field, value in [('source', 'b' * 40), ('version', '0.6.3'), ('digest', 'latest'), ('arch', 'amd64')]:
            records = copy.deepcopy(RECORDS)
            records[1][field] = value
            with self.assertRaises(ValueError):
                command(records, SOURCE)
        with self.assertRaises(ValueError):
            command(RECORDS[:1], SOURCE)


if __name__ == '__main__':
    unittest.main()
