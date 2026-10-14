"""Test the acceptance assertions using explicit synthetic fixture scrapes."""
import json
import pathlib
import subprocess
import sys
import tempfile
import unittest


class AssertionTests(unittest.TestCase):
    def test_growth_exclusion_and_failures(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            for name, value in [('before', 10), ('after', 100)]:
                rows = ['luna_exporter_up 1']
                for direction in ('rx', 'tx'):
                    for unit in ('bytes', 'packets'):
                        rows.append(f'luna_container_{direction}_{unit}_total{{container_id="a",container_name="name"}} {value}')
                (root / (name + '.metrics')).write_text('\n'.join(rows))

            def run(*args):
                return subprocess.run([sys.executable, str(pathlib.Path(__file__).with_name('check-integration.py')), directory, *args], capture_output=True)

            self.assertEqual(run('growth', 'before', 'after', 'a', 'rx').returncode, 0)
            self.assertNotEqual(run('growth', 'after', 'before', 'a', 'rx').returncode, 0)
            self.assertEqual(run('absent', 'after', 'bridge').returncode, 0)
            self.assertNotEqual(run('absent', 'after', 'a').returncode, 0)
            self.assertNotEqual(run('idle', 'before', 'after', 'a').returncode, 0)
            self.assertEqual(run('name', 'after', 'a', 'name').returncode, 0)
            fixture = {
                'baseline-links': [{'id': 1}], 'attached-before': [{'id': 1}, {'id': 2}, {'id': 3}],
                'detached': [{'id': 1}], 'baseline-maps': [{'id': 9}],
                'attached-maps': [{'id': 9}, {'id': 10, 'name': 'traffic'}], 'detached-maps': [{'id': 9}],
            }
            for name, value in fixture.items():
                (root / (name + '.json')).write_text(json.dumps(value))
            self.assertEqual(run('detached').returncode, 0)
            # Some bpftool versions emit whitespace instead of [] for no links.
            (root / 'baseline-links.json').write_text('\n')
            (root / 'detached.json').write_text('\n')
            self.assertEqual(run('detached').returncode, 0)
            (root / 'detached-maps.json').write_text(json.dumps([{'id': 9}, {'id': 10}]))
            self.assertNotEqual(run('detached').returncode, 0)


if __name__ == '__main__':
    unittest.main()
