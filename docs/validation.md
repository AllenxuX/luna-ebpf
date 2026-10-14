# Validation scope

Tested on Rocky Linux 9.7, kernel 5.14, Docker 29.5.2, amd64 and cgroup v2. Results describe one environment, not universal kernel/driver compatibility. Private host identities and raw environment evidence are not distributed.

| Check | Result |
|---|---|
| Go tests, Linux race/vet, bpf2go regeneration | Passed |
| amd64/arm64 builds | Passed; arm64 runtime not tested |
| Actual BPF load, dual attachment, sampling | Passed |
| Host TCP/UDP IPv4/IPv6 | Independent RX/TX growth verified |
| Default/custom bridge TCP/UDP | Independent RX/TX growth verified |
| Shared netns TCP/UDP | Independent socket cgroup attribution verified |
| none | Discovered; zero IP counters |
| macvlan/ipvlan TCP/UDP | Same-host dummy-parent communication verified |
| Idle-container isolation | No growth in unrelated test containers |
| Rename, restart, recreation, grace expiry | Passed |
| Pre/post-restart cgroup map cleanup | Retired IDs absent in both maps |
| Agent restart and resource cleanup | Passed; existing exporter preserved |
| Docker Compose / systemd | Validated on test host |
| Grafana10.4.3 / Prometheus3.2.1 | 18 panels imported, 21 queries returned data |
| Alert examples | Nine rules and four fixtures passed promtool |

Tests compare actual /metrics before and after traffic, checking bytes and hook invocations. Hook bytes need not equal iperf payload. Integration scripts are provided.

## Not verified

- Cross-host Swarm overlay, external NIC/offloads and production-scale churn.
- Macvlan/ipvlan external networks, VLANs and ipvlan L3.
- Docker daemon restart, arm64 runtime and minimum-capability deployment.
- All RAW sockets, forwarding, tunnels/proxies and socket-transfer paths.
- Real n9e notification delivery and Grafana browser rendering.

## Resource sizing

A previous small same-host test with5-second polling and30-second reconciliation reported about11MiB container-accounted memory and18MiB RSS. Map memlock reporting was about17MiB with128 possible CPUs. These accounting measures cannot be summed as exact physical memory.

This is not a startup or all-container/high-PPS benchmark. Kernel hook cost was not separately measured. A64–128MiB budget is a starting estimate for small deployments, not a tested limit; measure actual CPU, memory and map pressure.
