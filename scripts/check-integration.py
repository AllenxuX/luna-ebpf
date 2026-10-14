#!/usr/bin/env python3
"""Assertions on actual scrape artifacts; standard-library only."""
import json
import pathlib
import re
import sys

root = pathlib.Path(sys.argv[1])
mode, *args = sys.argv[2:]


def read_json(name):
    text = (root / name).read_text().strip()
    return json.loads(text) if text else []


def read(name):
    text = (root / (name + '.metrics')).read_text()
    rows = {}
    for line in text.splitlines():
        match = re.fullmatch(r'(luna_container_\w+_total)\{container_id="([^"]+)",container_name="((?:\\.|[^"\\])*)"\} ([\d.eE+\-]+)', line)
        if match:
            rows[(match[2], match[1])] = (float(match[4]), match[3])
    assert rows, f'no container metrics in {name}'
    assert 'luna_exporter_up 1' in text, f'not healthy: {name}'
    return rows


if mode == 'growth':
    before, after, *pairs = args
    a, b = read(before), read(after)
    for cid, direction in zip(pairs[::2], pairs[1::2]):
        for unit in ('bytes', 'packets'):
            key = (cid, f'luna_container_{direction}_{unit}_total')
            delta = b[key][0] - a[key][0]
            assert delta > 0, f'{cid} {direction} {unit} did not grow'
            print(f'{after}: {cid} {direction} {unit} delta={delta} (hook units)')
elif mode == 'idle':
    before, after, cid = args
    a, b = read(before), read(after)
    for key in a:
        if key[0] == cid:
            tolerance = 4096 if '_bytes_' in key[1] else 32
            assert b[key][0] - a[key][0] <= tolerance, f'idle container grew: {key}'
elif mode in ('absent', 'present'):
    name, cid = args
    found = any(key[0] == cid for key in read(name))
    assert found == (mode == 'present'), f'{mode} failed for {cid}'
elif mode == 'name':
    name, cid, expected = args
    assert all(v[1] == expected for k, v in read(name).items() if k[0] == cid)
elif mode == 'monotonic':
    before, after, cid = args
    a, b = read(before), read(after)
    for key in a:
        if key[0] == cid:
            assert b[key][0] >= a[key][0], f'counter dropped after restart: {key}'
elif mode == 'detached':
    before = read_json('attached-before.json')
    after = read_json('detached.json')
    baseline = read_json('baseline-links.json')
    ids_before = {row['id'] for row in before} - {row['id'] for row in baseline}
    ids_after = {row['id'] for row in after}
    assert ids_before and not ids_before.intersection(ids_after), 'agent attachment leaked'
    assert {row['id'] for row in baseline} <= ids_after, 'pre-existing attachment disappeared'
    maps_baseline = read_json('baseline-maps.json')
    maps_attached = read_json('attached-maps.json')
    maps_after = read_json('detached-maps.json')
    own_maps = {r['id'] for r in maps_attached if r.get('name', '').startswith(('traffic', 'monitored_cg', 'diagnostic_co'))} - {r['id'] for r in maps_baseline}
    assert own_maps and not own_maps.intersection({r['id'] for r in maps_after}), 'agent map leaked'
else:
    raise AssertionError(f'unknown check {mode}')
print(f'PASS: {mode}')
