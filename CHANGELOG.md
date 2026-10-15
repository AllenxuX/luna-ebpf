# Changelog

## 0.1.3 — cgroup v1 socket-trace MVP

- Add `--collector-backend auto|cgroup-skb|socket-trace`: auto keeps cgroup-skb on v2 and selects socket-trace on v1; socket-trace can be forced on v2.
- Resolve v1 identity from a mounted controller hierarchy, preferring memory, using native kernel handles and an explicit cgroup allowlist.
- Read socket layouts from kernel BTF, observe socket creation/clone, and seed existing sockets from cgroup process file descriptors. Require readable BTF and required kprobes; no guessed offsets or packet verdict changes.
- Retain metric names and labels. Hook observations differ across backends. The v1 MVP covers IPv4/IPv6 TCP/UDP, without guaranteed RAW/ICMP coverage.
- Build local source images as `luna-ebpf:0.1.3`. Privileged v1 and v2 Linux traffic, lifecycle and deployment validation passed; see docs/validation.md.

## 0.1.2 — Initial public release

- Go + cilium/ebpf + bpf2go implementation with root cgroup_skb hooks and Per-CPU counters.
- Discovery of all running Docker containers using Docker Engine API and cgroup v2 kernel handles.
- Per-container RX/TX bytes and packets, lifecycle reconciliation and map cleanup.
- Docker Compose, systemd, Prometheus, Grafana and n9e examples.
- Unit tests, Linux CI and privileged integration scripts.

This is an MVP. Validation scope and unverified deployment paths are documented in docs/validation.md.
