# Contributing

Bug reports, documentation improvements and pull requests are welcome.

## Build and test

Use Go 1.25.6+, Linux, make, Clang with the BPF target and llvm-strip.

```bash
make generate
make test race lint
make build ARCH=amd64
make build ARCH=arm64
python3 scripts/check-integration_test.py
```

Generated BPF ELF and bindings are committed. Regenerate them when C code or the map ABI changes; never edit them manually. Windows can run portable Go tests, but runtime and privileged integration require Linux.

## Integration

Use a dedicated root-accessible Linux Docker host. Validate cgroup v2 / cgroup-skb and cgroup v1 / socket-trace separately; socket-trace can also be forced on v2. It requires readable kernel BTF and required kprobes; kernels without BTF are unsupported by this backend. Tests create containers/networks, attach BPF and generate traffic. Docker daemon restart is opt-in because it disrupts existing containers.

Version 0.1.3 adds `--collector-backend auto|cgroup-skb|socket-trace`; source builds use the local `luna-ebpf:0.1.3` image. Auto preserves cgroup-skb on v2 and selects socket-trace on v1. Its current scope is TCP/UDP IPv4/IPv6; RAW/ICMP coverage is not guaranteed. Compare attribution and isolation within each backend rather than requiring packet-exact equality across different hook observations. Privileged v1 validation remains NOT RUN until actual Linux results are recorded.

The base integration script requires docker, curl, Python3 and host iperf3; bpftool/promtool enable additional checks.

```bash
sudo make integration
printf 'FROM alpine:3.21.3\nRUN apk add --no-cache iperf3\n' | docker build -t luna-ebpf-iperf-test:alpine3.21.3 -
make docker
sudo TEST_VIRTUAL_DRIVERS=1 python3 scripts/test-all-networks.py
```

The all-network script also needs Python3, iproute2 and bpftool. The virtual-driver option uses two temporary dummy interfaces and fixed synthetic subnets; use an isolated host without conflicting routes. Tests preserve existing exporters, which count independently if attached concurrently. Evidence is written to ignored `test-results/`.

## Pull requests

- Explain the problem, resulting behavior and verification performed.
- Keep socket/cgroup attribution, explicit allowlisting and accepting packet verdicts.
- Add meaningful tests for behavioral changes. Mark kernel/deployment checks not performed as unverified.
- Preserve metric compatibility or document breaking changes.
- Do not submit credentials, private registry addresses, production IDs, raw traffic captures or local evidence archives.

For runtime bugs, include sanitized kernel/Docker/cgroup details, exporter logs and reproduction commands. Review logs for private data before posting.
