#!/usr/bin/env python3
"""Privileged Linux acceptance: isolated containers; leaves existing services untouched."""
import ctypes, json, os, pathlib, re, subprocess, time, urllib.request

root = pathlib.Path(os.environ.get('RESULT_DIR', 'test-results/all-networks'))
root.mkdir(parents=True, exist_ok=True)
image = os.environ.get('TEST_IMAGE', 'luna-ebpf-iperf-test:alpine3.21.3')
agent_image = os.environ.get('AGENT_IMAGE', 'luna-ebpf:0.1.3')
collector_backend = os.environ.get('COLLECTOR_BACKEND', 'auto')
prefix = 'luna-all-' + str(os.getpid())
names, networks, ids, results = [], [], {}, []
agent = None
dummy = 'luna' + str(os.getpid())
dummy_created = False
parents = []
url = 'http://127.0.0.1:19814'

def run(*args):
    return subprocess.check_output(args, text=True).strip()

def container(role, mode, *command, extra=()):
    name = prefix + '-' + role
    names.append(name)
    ids[role] = run('docker', 'run', '-d', '--name', name, '--network', mode,
                    *extra, image, *command)
    return name

def snapshot(label):
    time.sleep(3)
    text = urllib.request.urlopen(url + '/metrics', timeout=10).read().decode()
    (root / (label + '.metrics')).write_text(text)
    reference = os.environ.get('REFERENCE_URL')
    if reference:
        ref = urllib.request.urlopen(reference + '/metrics', timeout=10).read().decode()
        (root / (label + '-reference.metrics')).write_text(ref)
    assert 'luna_exporter_up 1' in text
    rows = {}
    for line in text.splitlines():
        m = re.fullmatch(r'(luna_container_\w+_total)\{container_id="([^"]+)",container_name="[^"]+"\} ([\d.eE+\-]+)', line)
        if m:
            rows[m[2], m[1]] = int(float(m[3]))
    return rows

def value(rows, role, direction, unit):
    return rows[ids[role], f'luna_container_{direction}_{unit}_total']

def cgroup_id(path):
    class Handle(ctypes.Structure):
        _fields_ = [('size', ctypes.c_uint), ('kind', ctypes.c_int), ('data', ctypes.c_ubyte * 8)]
    handle, mount = Handle(), ctypes.c_int()
    handle.size = 8
    libc = ctypes.CDLL(None, use_errno=True)
    assert libc.name_to_handle_at(-100, os.fsencode(path), ctypes.byref(handle), ctypes.byref(mount), 0) == 0, ctypes.get_errno()
    assert handle.size == 8
    return int.from_bytes(bytes(handle.data), 'little')

def cgroup_hierarchy(version, mountinfo):
    if version not in ('1', '2'):
        raise ValueError('unsupported Docker cgroup version: ' + version)
    mounts = []
    for line in mountinfo.splitlines():
        before, separator, after = line.partition(' - ')
        fields, filesystem = before.split(), after.split()
        if not separator or len(fields) < 5 or len(filesystem) < 3 or fields[3] != '/':
            continue
        mount = re.sub(r'\\([0-7]{3})', lambda m: chr(int(m[1], 8)), fields[4])
        path = pathlib.Path(mount)
        if path != pathlib.Path('/sys/fs/cgroup') and pathlib.Path('/sys/fs/cgroup') not in path.parents:
            continue
        mounts.append((path, filesystem[0], filesystem[2].split(',')))
    if version == '2':
        for path, kind, _ in mounts:
            if kind == 'cgroup2':
                return path, ''
    else:
        for controller in ('memory', 'pids', 'cpuset', 'cpu', 'cpuacct', 'blkio', 'devices', 'freezer', 'net_cls', 'net_prio', 'hugetlb', 'perf_event'):
            for path, kind, options in mounts:
                if kind == 'cgroup' and controller in options:
                    try:
                        cgroup_id(path)  # Match the agent's usable-hierarchy selection.
                    except (AssertionError, OSError):
                        continue
                    return path, controller
    raise RuntimeError('no full-root cgroup hierarchy found for Docker version ' + version)


def container_cgroups(name):
    pid = run('docker', 'inspect', name, '--format', '{{.State.Pid}}')
    paths = []
    for line in pathlib.Path('/proc/' + pid + '/cgroup').read_text().splitlines():
        hierarchy, controllers, path = line.split(':', 2)
        if (not cgroup_controller and hierarchy == '0' and not controllers) or (cgroup_controller and cgroup_controller in controllers.split(',')):
            paths.append(path)
    assert len(paths) == 1, ('ambiguous/missing container cgroup', name, paths)
    relative = pathlib.PurePosixPath(paths[0])
    assert relative.is_absolute() and '..' not in relative.parts, paths[0]
    base = cgroup_mount.joinpath(*relative.parts[1:])
    return {cgroup_id(base)} | {cgroup_id(child) for child in base.rglob('*') if child.is_dir()}


def traffic(label, sender, receiver, address, port, udp=False, reverse=False, parallel=1):
    before = snapshot(label + '-before')
    args = ['docker', 'exec', prefix + '-' + sender, 'iperf3', '-c', address,
            '-p', str(port), '-t', '6', '-b', '5M' if udp else '20M', '-J']
    if udp: args.append('-u')
    if reverse: args.append('-R')
    if parallel > 1: args.extend(['-P', str(parallel)])
    payload = json.loads(run(*args))
    assert 'error' not in payload, payload
    (root / (label + '-payload.json')).write_text(json.dumps(payload, indent=2))
    summary = payload['end'].get('sum_sent') or payload['end'].get('sum')
    assert summary and summary['bytes'] > 0, (label, 'missing transferred bytes', payload['end'])
    transferred = summary['bytes']
    after = snapshot(label + '-after')
    tx, rx = (receiver, sender) if reverse else (sender, receiver)
    delta = {}
    for role, direction in [(tx, 'tx'), (rx, 'rx')]:
        for unit in ['bytes', 'packets']:
            d = value(after, role, direction, unit) - value(before, role, direction, unit)
            assert d > 0, (label, role, direction, unit, d)
            if unit == 'bytes':
                assert d >= transferred * 0.9, (label, role, direction, 'missing payload attribution', d, transferred)
                assert d <= transferred * 1.25, (label, role, direction, 'possible duplicate accounting', d, transferred)
            delta[role + '_' + direction + '_' + unit] = d
    for role in ids:
        if role not in (sender, receiver):
            for direction in ['rx', 'tx']:
                for unit in ['bytes', 'packets']:
                    assert value(after, role, direction, unit) == value(before, role, direction, unit), (label, 'cross attribution', role)
    results.append(dict(test=label, transferred_bytes=transferred, delta=delta))
    print('PASS', label, delta, flush=True)

try:
    (root / 'environment.json').write_text(json.dumps({
        'kernel': run('uname', '-r'),
        'docker_cgroup_version': run('docker', 'info', '--format', '{{.CgroupVersion}}'),
        'requested_backend': collector_backend, 'agent_image': agent_image,
    }, indent=2))
    cgroup_version = run('docker', 'info', '--format', '{{.CgroupVersion}}')
    cgroup_mount, cgroup_controller = cgroup_hierarchy(cgroup_version, pathlib.Path('/proc/self/mountinfo').read_text())
    (root / 'environment.json').write_text(json.dumps(dict(cgroup_version=cgroup_version, cgroup_mount=str(cgroup_mount), cgroup_controller=cgroup_controller, collector_backend=collector_backend, agent_image=agent_image), indent=2))
    network = prefix + '-net'
    networks.append(network)
    run('docker', 'network', 'create', network)
    host = container('host-server', 'host', 'iperf3', '-s', '-p', '15341')
    container('host-client', 'host', 'sleep', '3600')
    bridge = container('bridge', 'bridge', 'sleep', '3600')
    custom = container('custom-server', network, 'iperf3', '-s', '-p', '15342')
    container('custom-client', network, 'sleep', '3600')
    container('shared', 'container:' + custom, 'sleep', '3600')
    container('none', 'none', 'sleep', '3600')
    if os.environ.get('TEST_VIRTUAL_DRIVERS') == '1':
        for driver, subnet, port in [('macvlan', '10.239.41.0/24', 15343), ('ipvlan', '10.239.42.0/24', 15344)]:
            parent = dummy + ('m' if driver == 'macvlan' else 'i')
            run('ip', 'link', 'add', parent, 'type', 'dummy')
            parents.append(parent)
            dummy_created = True
            run('ip', 'link', 'set', parent, 'up')
            net = prefix + '-' + driver
            networks.append(net)
            run('docker', 'network', 'create', '-d', driver, '--subnet', subnet, '-o', 'parent=' + parent, net)
            container(driver + '-server', net, 'iperf3', '-s', '-p', str(port))
            container(driver + '-client', net, 'sleep', '3600')
    agent = prefix + '-agent'
    names.append(agent)
    run('docker', 'run', '-d', '--name', agent, '--privileged', '--network', 'host',
        '--pid', 'host', '--cgroupns', 'host', '-v', '/sys/fs/cgroup:/sys/fs/cgroup',
        '-v', '/proc:/host/proc:ro', '-v', '/var/run/docker.sock:/var/run/docker.sock:ro',
        agent_image, '--proc-root', '/host/proc', '--listen-address', '127.0.0.1:19814',
        '--metrics-poll-interval', '1s', '--docker-reconcile-interval', '2s',
        '--container-stale-grace-period', '3s', '--collector-backend', collector_backend)
    for _ in range(30):
        try:
            urllib.request.urlopen(url + '/readyz', timeout=2)
            break
        except Exception: time.sleep(1)
    else:
        raise RuntimeError('exporter did not become ready within 30 attempts')
    baseline = snapshot('baseline')
    for role in ids:
        # A reused Docker IP can receive legitimate late TCP packets from a
        # previous test. Compare stage deltas, not assumed zero initial totals.
        assert value(baseline, role, 'tx', 'bytes') >= 0
    for direction in ['rx', 'tx']:
        for unit in ['bytes', 'packets']:
            assert value(baseline, 'none', direction, unit) == 0, ('none must start at zero', direction, unit)
    gateway = run('docker', 'inspect', bridge, '--format', '{{range .NetworkSettings.Networks}}{{.Gateway}}{{end}}')
    address = run('docker', 'inspect', custom, '--format', '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
    traffic('host-tcp', 'host-client', 'host-server', '127.0.0.1', 15341)
    traffic('dual-stack-ipv4-parallel', 'host-client', 'host-server', '127.0.0.1', 15341, parallel=8)
    traffic('host-ipv6-tcp-reverse', 'host-client', 'host-server', '::1', 15341, reverse=True)
    traffic('host-ipv6-udp-reverse', 'host-client', 'host-server', '::1', 15341, udp=True, reverse=True)
    traffic('bridge-to-host-tcp', 'bridge', 'host-server', gateway, 15341)
    traffic('bridge-to-host-udp-reverse', 'bridge', 'host-server', gateway, 15341, udp=True, reverse=True)
    traffic('custom-bridge-tcp', 'custom-client', 'custom-server', address, 15342)
    traffic('custom-bridge-udp-reverse', 'custom-client', 'custom-server', address, 15342, udp=True, reverse=True)
    traffic('shared-netns-tcp', 'shared', 'custom-server', '127.0.0.1', 15342)
    traffic('shared-netns-udp-reverse', 'shared', 'custom-server', '127.0.0.1', 15342, udp=True, reverse=True)
    if dummy_created:
        for driver, port in [('macvlan', 15343), ('ipvlan', 15344)]:
            address = run('docker', 'inspect', prefix + '-' + driver + '-server', '--format', '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
            traffic(driver + '-tcp', driver + '-client', driver + '-server', address, port)
            traffic(driver + '-udp-reverse', driver + '-client', driver + '-server', address, port, udp=True, reverse=True)
    previous = snapshot('before-bridge-restart')
    retired_ids = container_cgroups(bridge)
    run('docker', 'restart', bridge)
    current = snapshot('bridge-restarted')
    retired_ids |= container_cgroups(bridge)
    for direction in ['rx', 'tx']:
        for unit in ['bytes', 'packets']:
            assert value(current, 'bridge', direction, unit) >= value(previous, 'bridge', direction, unit)
    traffic('bridge-after-restart-tcp', 'bridge', 'host-server', gateway, 15341)
    run('docker', 'rm', '-f', bridge)
    time.sleep(6)
    retired = snapshot('bridge-retired')
    assert not any(cid == ids['bridge'] for cid, metric in retired)
    # Inspect this agent's maps, avoiding other exporters' maps.
    agent_pid = run('docker', 'inspect', agent, '--format', '{{.State.Pid}}')
    map_ids = []
    for fd in pathlib.Path('/proc/' + agent_pid + '/fdinfo').iterdir():
        m = re.search(r'^map_id:\s+(\d+)', fd.read_text(), re.M)
        if m: map_ids.append(int(m[1]))
    dumps = {}
    for mid in set(map_ids):
        info = json.loads(run('bpftool', '-j', 'map', 'show', 'id', str(mid)))
        if isinstance(info, list): info = info[0]
        owner_map = info['name'] in ('socket_owners', 'inode_owners')
        if info['name'] != 'traffic' and not info['name'].startswith('monitored_cg') and not owner_map: continue
        raw = run('bpftool', '-j', 'map', 'dump', 'id', str(mid))
        dumps[info['name']] = json.loads(raw) if raw else []
        for row in dumps[info['name']]:
            if owner_map:
                value = row['value']
                if isinstance(value, list):
                    value = int.from_bytes(bytes(int(x, 16) for x in value[:8]), 'little')
                assert isinstance(value, int) and value not in retired_ids, ('retired socket owner', value)
                continue
            key = row.get('key')
            if isinstance(key, list):
                assert int.from_bytes(bytes(int(x, 16) for x in key[:8]), 'little') not in retired_ids
            elif isinstance(key, dict):
                assert key.get('cgroup_id') not in retired_ids
            elif isinstance(key, int): assert key not in retired_ids
            else: raise AssertionError(('unsupported map key', key))
    assert len(dumps) in (2, 4), dumps.keys()
    if 'socket_owners' in dumps: assert 'inode_owners' in dumps
    (root / 'retired-maps.json').write_text(json.dumps(dumps, indent=2))
    results.append(dict(test='bridge-restart-retire-map-cleanup', retired_cgroups=sorted(retired_ids), passed=True))
    print('PASS bridge restart, retirement and map cleanup', flush=True)
finally:
    if agent:
        log = subprocess.run(['docker', 'logs', agent], text=True, capture_output=True)
        (root / 'agent.log').write_text(log.stdout + log.stderr)
    for name in reversed(names):
        subprocess.run(['docker', 'rm', '-f', name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    for network in networks:
        subprocess.run(['docker', 'network', 'rm', network], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    for parent in parents:
        subprocess.run(['ip', 'link', 'del', parent], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    (root / 'results.json').write_text(json.dumps(results, indent=2))
