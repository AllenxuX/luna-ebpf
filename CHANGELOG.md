# Changelog

## 0.1.2 — Initial public release

- Go + cilium/ebpf + bpf2go implementation with root cgroup_skb hooks and Per-CPU counters.
- Discovery of all running Docker containers using Docker Engine API and cgroup v2 kernel handles.
- Per-container RX/TX bytes and packets, lifecycle reconciliation and map cleanup.
- Docker Compose, systemd, Prometheus, Grafana and n9e examples.
- Unit tests, Linux CI and privileged integration scripts.

This is an MVP. Validation scope and unverified deployment paths are documented in docs/validation.md.
