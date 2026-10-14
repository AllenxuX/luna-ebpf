# luna-ebpf

[![CI](https://github.com/AllenxuX/luna-ebpf/actions/workflows/test.yaml/badge.svg)](https://github.com/AllenxuX/luna-ebpf/actions/workflows/test.yaml)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE)

**An eBPF-based Prometheus exporter for per-container Docker network traffic, including host networking.**

[中文](README.md) | English

Built with Go, cilium/ebpf and bpf2go. Socket cgroup identity attributes traffic to containers even when they share a network namespace. No Cilium installation or Kubernetes cluster is required.

## Features

- Discover running Docker containers regardless of network mode.
- Export cumulative RX/TX bytes and packets with full container ID and name.
- Reconcile starts, renames, restarts and exits; clean retired BPF map entries.
- Docker Compose and systemd deployment, Prometheus config, Grafana dashboard and n9e alert examples.

## Quick start

Requires Linux, Docker Engine, Docker Compose v2 and cgroup v2.

```bash
git clone https://github.com/AllenxuX/luna-ebpf.git
cd luna-ebpf
docker compose up -d --build
curl -fsS http://127.0.0.1:9808/readyz
curl -fsS http://127.0.0.1:9808/metrics
```

Compose builds `luna-ebpf:0.1.2` locally from source. No published project image is required. The first build downloads base images, dependencies and compiler tools. Stop with `docker compose down`.

The default listener is localhost. Compose uses privileged host PID/cgroup/network access. A read-only Docker socket mount does not restrict Engine API access. HTTP has no built-in authentication or TLS; secure remote scrapes with your network controls.

## Metrics

Container counters have `container_id` and `container_name` labels:

| Metric | Meaning |
|---|---|
| `luna_container_rx_bytes_total` | Socket ingress skb bytes |
| `luna_container_tx_bytes_total` | Socket egress skb bytes |
| `luna_container_rx_packets_total` | Ingress hook invocations |
| `luna_container_tx_packets_total` | Egress hook invocations |

Exporter metrics include `luna_exporter_up`, `luna_exporter_docker_connected`, `luna_exporter_containers` and discovery/map diagnostics. Containers with no IP traffic expose zero counters.

```promql
sum by (instance, container_id, container_name) (
  rate(luna_container_tx_bytes_total[5m])
) * 8 / 1000000
```

Import the [Grafana dashboard](deploy/grafana/luna-ebpf-dashboard.json) and select your Prometheus data source. See [monitoring](docs/monitoring.md) for alert examples.

## Architecture and scope

Docker list/inspect/events → host PID cgroup path → `name_to_handle_at()` → cgroup allowlist → root `cgroup_skb` ingress/egress → Per-CPU maps → Go aggregation → `/metrics`.

All BPF paths accept packets. Host, default/custom bridge and shared netns TCP/UDP attribution have been validated. `none` is discovered with zero IP traffic. Macvlan/ipvlan tests cover same-host communication using dummy parents, not external networks or ipvlan L3. Other network drivers are discovered but their paths have not been verified.

Bytes are skb length and packets are hook invocations, not application payload or physical wire packets. AF_UNIX, pure forwarding and all RAW/tunnel/socket-transfer paths are outside guaranteed coverage. Discovery windows can miss traffic; agent restarts reset counters.

Runtime was validated on Rocky Linux 9.7, kernel 5.14, Docker 29.5.2 and cgroup v2. Kernel version alone does not guarantee compatibility. Run `--check` and real traffic tests on your environment. See [validation](docs/validation.md).

## Development

Go 1.25.6+, Clang with a BPF target, LLVM and make are required to regenerate BPF. Generated ELF/bindings are included; ordinary Go builds do not require Clang. Linux amd64 and arm64 builds are supported; arm64 runtime has not been tested.

```bash
make generate build
make test race lint
sudo ./bin/luna-ebpf --check
```

See [deployment](docs/deployment.md), [architecture](docs/architecture.md), [metric semantics](docs/metric-semantics.md), [operations](docs/operations.md) and [contributing](CONTRIBUTING.md).

## License

[Apache-2.0](LICENSE). Vendored libbpf headers retain their original licenses and notices; see [third-party notices](THIRD_PARTY_NOTICES.md).
