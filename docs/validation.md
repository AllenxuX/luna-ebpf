# Validation scope

The historical 0.1.2 cgroup-skb results below were tested on Rocky Linux 9.7,
kernel 5.14, Docker 29.5.2, amd64 and cgroup v2. Results describe one environment,
not universal kernel/driver compatibility. Private host identities and raw
environment evidence are not distributed. They do not validate socket-trace.

## 0.1.4 Docker API compatibility

The former SDK negotiation floor rejected old daemons before collection.
The new constructor probes the unversioned ping endpoint and selects the
daemon's real API, with exporter minimum 1.41 and SDK maximum 1.52.

| Check | Result |
|---|---|
| Strict HTTP API 1.41/1.42/1.43/1.44/1.52/newer | Passed: version/info/list/inspect/events |
| Invalid/older/missing API, cancellation, ping failure and GET fallback | Passed |
| Real Docker 20.10.24 / API 1.41, cgroup v2 | TCP/UDP, isolation and lifecycle passed |
| Real Docker 23.0.6 / API 1.42, cgroup v2 | TCP/UDP, isolation and lifecycle passed |
| Real Docker 24.0.9 / API 1.43, cgroup v2 | TCP/UDP, isolation and lifecycle passed |
| Docker 24.0.9, forced socket-trace on v2 | TCP/UDP, isolation and lifecycle passed |
| Release-image binary on Docker 20.10.24 and 24.0.9, private netns | TCP/UDP, lifecycle and host bridge preservation passed |
| Docker 23.0.6, private netns | TCP/UDP, lifecycle and host bridge preservation passed |
| Final image on Docker 29.5.2 / API 1.54 selected as 1.52 | All-network IPv4/IPv6 regression and map cleanup passed |
| BPF generation, Go tests, Linux race/vet and amd64/arm64 build | Passed; arm64 runtime not tested |
| Old 0.1.3 binary against these old daemons | Original API 1.44 rejection reproduced |

Real old-daemon tests use separate static binaries, socket, exec/data roots
and VFS storage. The supplied script also uses a private network namespace:
old daemon initialization can delete docker0 even with `--bridge=none`.
Bridge creation and iptables changes are disabled. They
verify host-network IPv4 TCP and reverse UDP, an unrelated idle container,
rename events, restart, grace expiry, event reconnect and daemon recovery.
Example Docker 24 TCP delta: client TX and server RX each 4,069,676 bytes;
the idle container's four counters remained zero.

These tests do not establish every old Docker/kernel/network-mode combination.
Forced socket-trace on v2 does not replace genuine v1 runtime validation;
the genuine v1 results below belong to version 0.1.3. Kernel/BTF requirements
still apply. API 1.40 and older are explicitly unsupported. Raw host evidence
remains private; the reproducible script is `scripts/test-docker-api.py`.

## 0.1.3 actual validation

Tested on Rocky Linux 9.7, kernel 5.14.0-611.55.1.el9_7.x86_64,
Docker 29.5.2, amd64. The same host was rebooted into genuine cgroup v1
(Docker cgroupfs), then restored to v2 (systemd). These are host boot modes,
not a v1-looking directory inside a v2 container.

| Check | Result |
|---|---|
| Go tests, Linux race/vet, bpf2go generation | Passed |
| amd64 / arm64 build | Passed; arm64 runtime not tested |
| v2 auto cgroup-skb | Real multi-container traffic passed |
| v2 forced socket-trace | IPv4/IPv6 TCP/UDP and parallel flows passed |
| v1 auto socket-trace | IPv4/IPv6 TCP/UDP and parallel flows passed |
| host, default/custom bridge, shared netns | Independent RX/TX deltas passed |
| none | Discovered, all four IP counters zero |
| macvlan / ipvlan | Same-host, separate dummy-parent tests passed |
| Isolation | Unrelated test containers had no counter growth |
| Restart, recreation, rename, grace expiry | Passed on v1 |
| Retired IDs | Absent from traffic, allowlist, socket and inode maps |
| Docker daemon restart / agent restart | Actual v1 restart and recovery passed |
| Docker Compose / restricted systemd unit | Actual v1 startup and scraping passed |
| Grafana / Prometheus | 18 panels imported; 21 queries returned data |
| Alerts | Nine rules and four fixtures passed promtool |
| Full inode-map replacement | Capacity regression unit test passed |

Traffic checks compare before/after metrics and require 90–125% of iperf
payload bytes, positive packet deltas, and no unrelated-container growth.
Socket-trace observes IP output and TCP/UDP receive hooks; packet-exact
equality with cgroup-skb or wire captures is not guaranteed. IPv4/IPv6
TCP/UDP are the validated MVP; RAW/ICMP completeness is not guaranteed.
Readable kernel BTF and supported kprobe signatures are required on v1;
old kernels without BTF are unsupported.

Sample v1 deltas: host TCP client TX 15,092,892 bytes and server RX
15,092,788 bytes; IPv6 reverse UDP TX/RX 3,775,162 bytes each.
These are measured test deltas, not static example counters.

Single snapshots during the small traffic test: socket-trace RSS about
30.2 MiB on v1 and 30.4 MiB on v2; container-accounted memory about
47.3 MiB and 36.8 MiB respectively. These accounting values cannot be
added. They are not startup, CPU, high-PPS or production-scale benchmarks.
The kernel hook CPU cost and FD scan latency were not measured separately.

## Historical 0.1.2 cgroup-skb validation

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
- arm64 runtime and capability-minimal deployment across other kernels.
- All RAW sockets, forwarding, tunnels/proxies and socket-transfer paths.
- Real n9e notification delivery and Grafana browser rendering.

## Resource sizing

A previous cgroup v2 / cgroup-skb small same-host test with5-second polling and30-second reconciliation reported about11MiB container-accounted memory and18MiB RSS. Map memlock reporting was about17MiB with128 possible CPUs. These accounting measures cannot be summed as exact physical memory. They are not socket-trace resource measurements.

This is not a startup or all-container/high-PPS benchmark. Kernel hook cost was not separately measured. A64–128MiB budget is a starting estimate for small deployments, not a tested limit; measure actual CPU, memory and map pressure.

Socket-trace owner maps are bounded to `min(8 * trafficEntries, 1048576)`;
seeding via `cgroup.procs` and `/proc/PID/fd` adds discovery work. No production-scale socket-trace CPU or memory budget is established.
