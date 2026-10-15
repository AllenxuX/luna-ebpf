# Operations

Socket-trace ownership capacity includes all host IPv4/IPv6 sockets, including unmonitored workloads. The default is65,536 entries, derived as min(8 × map_max_entries,1,048,576). Host socket pressure can therefore prevent new container socket tracking. Monitor map_insert_failures_total and size for total host socket concurrency, not only container cgroup count. This pressure case has not been load-tested.

Since 0.1.2, all running Docker containers are discovered regardless of network mode, including the exporter itself. A valid dedicated cgroup is required; mapping failures remain visible via discovery errors. Containers with no IP traffic expose zero counters. No network_mode metric label is added. Plan map capacity by cgroup count including descendants. Existing 0.1.1 dashboards and alerts remain compatible.

Run `--check` as the intended service identity before deployment, and
`scripts/preflight.sh` to record uname, architecture, Go if installed,
cgroup filesystem, Docker version/cgroup driver, capabilities, existing attachments
and optional bpftool helper probes. Actual object load/attach/sample is the
authoritative capability test. Failed attachment rolls back owned links.
Exclusive/conflicting attachments are diagnosed; never detach another agent's
program to work around errors without a separate operator decision.

`--collector-backend auto` uses cgroup-skb on v2 and socket-trace on v1.
`cgroup-skb` requires v2; `socket-trace` can be forced on either version.
Socket-trace requires readable kernel BTF (normally `/sys/kernel/btf/vmlinux`)
and the required socket creation/clone, local IP output and TCP/UDP receive
kprobes. It derives layouts from BTF without guessed offsets. Old kernels
without usable BTF are unsupported by this backend. If running in Docker,
ensure the host BTF is readable inside the exporter as well. Missing BTF,
probe symbols or permissions must be resolved before using this backend;
kernel version alone is insufficient. Privileged v1 runtime tests are NOT RUN.

Linux must enable the selected backend's BPF/cgroup/socket features, notably
BPF syscall and socket cgroup association; cgroup-skb also requires cgroup BPF
and cgroup v2. Kernel configurations,
vendor backports, lockdown, seccomp, BPF LSM policies, and capability handling
matter more than a version number. Full verifier logs are emitted on rejection.
Permission errors are distinguished from unsupported BPF capabilities; verify
with `bpftool feature probe kernel` and `bpftool cgroup tree /sys/fs/cgroup`.

Recent kernels typically need CAP_BPF and CAP_NET_ADMIN for these programs;
CAP_PERFMON requirements vary by kernel/program restrictions. Older kernels
may need CAP_SYS_ADMIN and memory-lock changes via CAP_SYS_RESOURCE. Directory
access can require CAP_DAC_READ_SEARCH. Reading host init's namespace symlink
is ptrace-gated and the restricted systemd service needs CAP_SYS_PTRACE on the
validated Rocky Linux host. The systemd unit intentionally uses a
compatibility capability set and an unlimited memlock limit. Test narrower
bounds with `--check` on your exact kernel. It does not use PrivateUsers,
ProtectProc, a private cgroup namespace or socket namespaces that would hide
host identities. journald receives JSON slog output; manage rotation/retention
through journald's normal configuration.

Docker reference deployment is a privileged PoC, not a hardened production
permission claim. It needs host PID and cgroup namespace views, host cgroup
and proc mounts and the Docker socket. Use current Docker Compose's
`cgroup: host` and `pid: host`; older Compose/Engine compatibility is untested.
The daemon socket remains effectively a high-privilege API even when mounted
`:ro`. On a real target, test a restricted capability set, a seccomp profile
allowing bpf/name_to_handle_at and required syscalls, and filesystem access.
Rootless Docker and user-namespace remapping are not validated or supported
as a deployment assumption.

For v2, the configured cgroup root must be an actual mountpoint of the host
hierarchy with filesystem-root `/`. V1 selects a real mounted controller
hierarchy beneath that root (memory preferred), also requiring filesystem-root
`/` and the cgroup v1 filesystem. Binding just a container subtree is rejected.
`proc_root` must expose host PID 1 at `/` or `/init.scope` and the agent's
cgroup namespace must match the host init namespace. Systems with an unusual
host-init cgroup layout will fail this conservative check; validate and adapt
explicitly rather than guessing identity. Multiple cgroup2 mounts and mount
paths using mountinfo-escaped whitespace are not validated target layouts.

The default HTTP listener is localhost, with no auth or TLS. Put a controlled
reverse proxy/firewall in front of an explicitly remote listener if necessary.
For containerized Prometheus, localhost refers to its own namespace unless
it also uses host networking; configure the actual accessible exporter target.

Diagnostics:

| Signal | Meaning / action |
| --- | --- |
| `up=0`, poll errors rising | Latest map iteration/diagnostic read or lifecycle map operation failed. Inspect wrapped BPF errors. |
| `docker_connected=0` | Events or inventory API failed. Last view is retained; lifecycle can be stale. Check socket and daemon. |
| discovery errors rising | PID exited, handle/path/permission failure, scan bound exceeded or inventory error. Review container ID logs. |
| map insert failures rising | Too many active/child cgroup direction keys or kernel allocation failure. Increase capacity after sizing CPU memory. |
| lookup failures rising | Key absent after insertion; packets remain accepted but accounting is incomplete. |
| unattributed rising | cgroup-skb helper returned zero cgroup ID, or socket-trace has an unknown socket owner. This is neither other-container traffic nor a missed-traffic total. Validate kernel/socket coverage. |
| stale last-success timestamp | Map poll is not progressing. Compare expected interval and process logs. |

Container exit freezes the final sampled counters. HTTP grace defaults to 60s,
while kernel entries are removed immediately. Set grace >= scrape interval
if a final sample matters; even then event/sample windows can miss bytes.
Normal reconciliation uses one full running inventory and bounded per-container
subtree scans. An incomplete subtree is rejected and counted. A confirmed-running
container keeps its last verified mapping until resolution recovers, so a
restarted container's new-cgroup traffic may be missed during that window. Keep the max-node
bound appropriate for your workload instead of scanning the whole host.

Socket-trace additionally scans cgroup process lists and `/proc/PID/fd` to seed
existing sockets, so proc access and workload FD counts affect discovery cost.
Owner maps are bounded to `min(8 * map_max_entries, 1048576)`. Measure discovery
latency, CPU, memory and map pressure on the target host; existing v2 resource
observations do not establish a socket-trace budget. Its MVP covers TCP/UDP
IPv4/IPv6, without guaranteed RAW/ICMP, AF_UNIX, forwarding or tunnel coverage.

All BPF objects/links are unpinned. Normal stop closes HTTP, links and maps;
on process death file descriptors close. Agent restart resets counter epochs.
Check no owned program/map IDs remain after SIGTERM using the integration
harness/bpftool. Do not infer absence of leaks until tested on your kernel.
No automatic cleanup affects programs owned by other processes.
