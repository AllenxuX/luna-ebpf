#!/usr/bin/env bash
set -euo pipefail
[[ $(uname -s) == Linux ]] || { echo 'FAIL: Linux required' >&2; exit 1; }
echo "kernel=$(uname -r) arch=$(uname -m)"
if command -v go >/dev/null; then go version; else echo 'go: unavailable (only needed to build)'; fi
root=${CGROUP_ROOT:-/sys/fs/cgroup}
docker version
docker info --format 'Cgroup driver={{.CgroupDriver}} version={{.CgroupVersion}}'
version=$(docker info --format '{{.CgroupVersion}}')
case "$version" in
  2)
    [[ $(stat -fc %T "$root") == cgroup2fs ]] || { echo 'FAIL: cgroup2fs required for cgroup-skb' >&2; exit 1; }
    [[ -r "$root/cgroup.controllers" ]] || { echo 'FAIL: cgroup v2 root inaccessible' >&2; exit 1; }
    ;;
  1)
    [[ -r /sys/kernel/btf/vmlinux ]] || { echo 'FAIL: cgroup v1 socket-trace requires readable kernel BTF' >&2; exit 1; }
    echo 'cgroup v1: the agent validates the selected controller and required kprobes.'
    ;;
  *) echo "FAIL: unsupported Docker cgroup version $version" >&2; exit 1 ;;
esac
if command -v bpftool >/dev/null; then
  if [[ $version == 2 ]]; then bpftool cgroup tree "$root"; fi
  bpftool feature probe kernel || echo 'Feature probe failed; real agent load below is authoritative.'
else
  echo 'bpftool absent: install it to inspect existing attachments and helper support.'
fi
grep -E 'Cap(Eff|Bnd)' /proc/self/status
binary=${LUNA_EBPF_BINARY:-${HOSTNET_BINARY:-./bin/luna-ebpf}}
[[ -x "$binary" ]] || { echo "FAIL: build $binary first (make build)" >&2; exit 1; }
"$binary" --cgroup-root "$root" --check
echo 'PASS: selected backend load/attach, discovery and sample completed; owned links detached.'
