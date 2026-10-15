# Metric semantics

The four `luna_container_{rx,tx}_{bytes,packets}_total` metrics are cumulative
counters, labeled by full `container_id` and the name without `/`.
Since 0.1.2, discovery includes every running Docker container regardless of network mode. Running
containers with no traffic expose zero values. No IP, PID, cgroup path,
start-time, Compose/custom labels, ports or five-tuples are exported.

RX is attributed to the receiving socket and TX to the sending socket.
With cgroup-skb, bytes are `skb->len` at root cgroup ingress/egress hooks.
With socket-trace, TX observes `ip_output` / `ip6_output` and RX
observes TCP/UDP receive probes. Bytes are skb length at those observation
points (IP/skb length), not application payload or NIC Ethernet wire bytes.
Packets count hook invocations, not physical wire packets. GRO/GSO and other
offloads can combine or segment traffic outside this observation point.
Loopback counts sender TX and receiver RX independently. TCP ACKs/control
traffic can increase the reverse direction too. IPv4/IPv6 TCP/UDP take the
backend-specific hooks; actual kernel/helper/socket coverage must be verified
per host. Socket-trace is an IPv4/IPv6 TCP/UDP MVP and does not guarantee
RAW/ICMP coverage. Different observation points can yield different byte and
packet totals: packet-exact equality between backends is not a contract.
Metric names and labels remain unchanged; no backend label is added.

This does not cover AF_UNIX, host forwarding, or guarantee all RAW sockets,
tunnels, proxies and skbs lacking valid local socket association. Proxies can
own sockets themselves; their bytes belong to the proxy's cgroup. Nonzero IDs
not in the allowlist are skipped and allocate no traffic map entries.

`luna_exporter_unattributed_{bytes,packets}_total{direction="rx|tx"}` only
counts helper results of ID=0 at cgroup-skb root hooks, or unknown socket
owners at socket-trace observation points. It is neither other-container
traffic nor a missed-traffic total. It cannot estimate
traffic missed before a legitimate container or child was discovered. Map
insertion failures exclude normal EEXIST races; lookup failures are separately
observable. No packet is intentionally dropped by this exporter.

Map values are sampled, not transactionally locked against traffic. CPU sums
are approximate at the sampling instant; bytes and packets can be read at
slightly different instants. Complete read errors keep the previous snapshot.
The HTTP snapshot itself is consistent and immutable.

New-container and child-cgroup traffic before allowlisting is not recoverable.
Docker event delivery, the bounded filesystem scan and the reconciliation
interval create measurement windows. Retirement freezes the latest sample,
then removes maps; traffic between that sample and removal can be lost.
Discovery failure or Docker disconnect can make lifecycle information stale.
Last known containers remain allowlisted until a successful inventory replaces
them; this is visible via docker_connected=0/discovery errors.

Agent restarts rebuild unpinned maps and reset counters. A container with the
same Docker ID restarting within retained state keeps cumulative totals across
new cgroup IDs. Restart after grace expiry starts a new counter epoch on the
same labels. Renaming changes the name label and creates a new series with the
current cumulative count. No started-at label means repeated restarts do not
create unlimited new series. Prometheus handles counter decreases with rate;
rename and brief restarts still need dashboard awareness.

uint64 additions wrap at 2^64. A detected raw map decrease is treated as a new
raw epoch and adds its current value; increments spanning an actual wrap can
be lost. Prometheus float64 is exact only for integers through 2^53, so huge
counters lose unit precision. This MVP does not persist totals across agent
restarts or provide billing-grade accounting.

```promql
# TX Mbps
sum by (instance, container_name) (rate(luna_container_tx_bytes_total[5m])) * 8 / 1000000
# RX Mbps
sum by (instance, container_name) (rate(luna_container_rx_bytes_total[5m])) * 8 / 1000000
# TX and RX skb/s (not NIC wire PPS)
sum by (instance, container_name) (rate(luna_container_tx_packets_total[5m]))
sum by (instance, container_name) (rate(luna_container_rx_packets_total[5m]))
```
