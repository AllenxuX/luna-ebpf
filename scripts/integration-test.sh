#!/usr/bin/env bash
# Run on a dedicated Linux Docker cgroup-v2 host. Does not restart Docker unless opted in.
set -euo pipefail
[[ $(uname -s) == Linux ]] || { echo 'NOT RUN: Linux required'; exit 77; }
for tool in docker curl python3 iperf3; do command -v "$tool" >/dev/null || { echo "NOT RUN: missing $tool"; exit 77; }; done
binary=${LUNA_EBPF_BINARY:-${HOSTNET_BINARY:-./bin/luna-ebpf}}
[[ -x $binary ]] || { echo "NOT RUN: missing $binary"; exit 77; }
out=${RESULT_DIR:-test-results/integration-$(date +%Y%m%d-%H%M%S)}
mkdir -p "$out"
port=${IPERF_PORT:-15201}
tcp_bandwidth=${TCP_BANDWIDTH:-100M}
http_port=${EXPORTER_PORT:-19808}
collector_backend=${COLLECTOR_BACKEND:-auto}
cgroup_version=$(docker info --format '{{.CgroupVersion}}')
attachment_backend=$collector_backend
if [[ $cgroup_version == 1 && $collector_backend == auto ]]; then attachment_backend=socket-trace; fi
url="http://127.0.0.1:$http_port"
prefix="luna-test-$$"
server="$prefix-server"; client="$prefix-client"; idle="$prefix-idle"; bridge="$prefix-bridge"
image=${TEST_IMAGE:-luna-ebpf-iperf-test:alpine3.21.3}
agent_pid=''
cleanup() {
  if [[ -n $agent_pid ]]; then kill -TERM "$agent_pid" 2>/dev/null || true; wait "$agent_pid" 2>/dev/null || true; fi
  docker rm -f "$server" "$client" "$idle" "$bridge" >/dev/null 2>&1 || true
}
trap cleanup EXIT
if [[ -z ${TEST_IMAGE:-} ]]; then
  printf 'FROM alpine:3.21.3\nRUN apk add --no-cache iperf3\n' | docker build -t "$image" - >"$out/image-build.log" 2>&1
fi
docker run -d --name "$server" --network host "$image" iperf3 -s -p "$port" >"$out/server.id"
docker run -d --name "$client" --network host "$image" sleep 3600 >"$out/client.id"
docker run -d --name "$idle" --network host "$image" sleep 3600 >"$out/idle.id"
docker run -d --name "$bridge" "$image" sleep 3600 >"$out/bridge.id"
start_agent() {
  "$binary" --collector-backend "$collector_backend" --listen-address "127.0.0.1:$http_port" --metrics-poll-interval 1s --docker-reconcile-interval 2s --container-stale-grace-period 3s >"$out/agent.log" 2>&1 & agent_pid=$!
  for ((i=0;i<30;i++)); do if curl -fsS "$url/readyz" >/dev/null; then return; fi; sleep 1; done
  echo 'FAIL: exporter not ready'; cat "$out/agent.log"; exit 1
}
snapshot() { sleep 2; curl -fsS "$url/metrics" >"$out/$1.metrics"; }
check() { python3 scripts/check-integration.py "$out" "$@"; }
dump_cgroup_links() {
  if [[ $cgroup_version == 1 ]]; then printf '[]\n'; else bpftool -j cgroup show /sys/fs/cgroup; fi
}
if command -v bpftool >/dev/null; then
  dump_cgroup_links >"$out/baseline-links.json"
  bpftool -j map show >"$out/baseline-maps.json"
fi
start_agent
snapshot baseline
docker exec "$client" iperf3 -c 127.0.0.1 -p "$port" -t 10 -b "$tcp_bandwidth" -J >"$out/tcp-payload.json"
snapshot tcp
check growth baseline tcp "$(cat "$out/server.id")" rx "$(cat "$out/client.id")" tx
check payload baseline tcp tcp-payload.json "$(cat "$out/client.id")" tx
check payload baseline tcp tcp-payload.json "$(cat "$out/server.id")" rx
check idle baseline tcp "$(cat "$out/idle.id")"
docker exec "$client" iperf3 -c 127.0.0.1 -p "$port" -t 10 -b "$tcp_bandwidth" -R -J >"$out/tcp-reverse-payload.json"
snapshot reverse
check growth tcp reverse "$(cat "$out/client.id")" rx "$(cat "$out/server.id")" tx
check payload tcp reverse tcp-reverse-payload.json "$(cat "$out/server.id")" tx
check payload tcp reverse tcp-reverse-payload.json "$(cat "$out/client.id")" rx
docker exec "$client" iperf3 -c 127.0.0.1 -p "$port" -t 10 -u -b 10M -J >"$out/udp-payload.json"
snapshot udp
check growth reverse udp "$(cat "$out/server.id")" rx "$(cat "$out/client.id")" tx
check payload reverse udp udp-payload.json "$(cat "$out/client.id")" tx
check payload reverse udp udp-payload.json "$(cat "$out/server.id")" rx
docker exec "$client" iperf3 -c 127.0.0.1 -p "$port" -t 10 -u -R -b 10M -J >"$out/udp-reverse-payload.json"
snapshot udp-reverse
check growth udp udp-reverse "$(cat "$out/client.id")" rx "$(cat "$out/server.id")" tx
check payload udp udp-reverse udp-reverse-payload.json "$(cat "$out/server.id")" tx
check payload udp udp-reverse udp-reverse-payload.json "$(cat "$out/client.id")" rx
check present udp-reverse "$(cat "$out/bridge.id")"
# Host traffic legitimately reaches server RX, but must not grow the idle client counters.
iperf3 -c 127.0.0.1 -p "$port" -t 3 -b "$tcp_bandwidth" -J >"$out/host-payload.json"
snapshot host
check idle udp-reverse host "$(cat "$out/client.id")"
gateway=$(docker inspect "$bridge" --format '{{range .NetworkSettings.Networks}}{{.Gateway}}{{end}}')
docker exec "$bridge" iperf3 -c "$gateway" -p "$port" -t 3 -b "$tcp_bandwidth" -J >"$out/bridge-payload.json"
snapshot bridge
check growth host bridge "$(cat "$out/server.id")" rx "$(cat "$out/bridge.id")" tx
check idle host bridge "$(cat "$out/client.id")"
docker rename "$client" "$client-renamed"; client="$client-renamed"
snapshot rename
check name rename "$(cat "$out/client.id")" "$client"
# Same Docker ID restart: cgroup ID may change; cumulative counters must not drop.
docker restart "$client" >/dev/null
snapshot restart
check monotonic bridge restart "$(cat "$out/client.id")"
docker exec "$client" iperf3 -c 127.0.0.1 -p "$port" -t 3 -b "$tcp_bandwidth" -J >"$out/restart-payload.json"
snapshot restart-traffic
check growth restart restart-traffic "$(cat "$out/server.id")" rx "$(cat "$out/client.id")" tx
old=$(cat "$out/client.id")
docker rm -f "$client" >/dev/null
sleep 6
snapshot stopped
check absent stopped "$old"
docker run -d --name "$client" --network host "$image" sleep 3600 >"$out/client.id"
snapshot recreated
check present recreated "$(cat "$out/client.id")"
if command -v bpftool >/dev/null; then
  dump_cgroup_links >"$out/attached-before.json"
  bpftool -j map show >"$out/attached-maps.json"
fi
kill -TERM "$agent_pid"; wait "$agent_pid"; agent_pid=''
if command -v bpftool >/dev/null; then
  dump_cgroup_links >"$out/detached.json"
  bpftool -j map show >"$out/detached-maps.json"
  check detached "$attachment_backend"
else echo 'NOT RUN: bpftool attachment leak check'; fi
start_agent
snapshot agent-restart
check present agent-restart "$(cat "$out/client.id")"
if [[ ${TEST_RESTART_DOCKER:-0} == 1 ]]; then
  # Explicit opt-in only. This disrupts every container on the host.
  sudo systemctl restart docker
  docker start "$server" "$client" "$idle" "$bridge" >/dev/null
  sleep 3
  for ((i=0;i<60;i++)); do if curl -fsS "$url/readyz" >/dev/null; then break; fi; sleep 1; done
  curl -fsS "$url/readyz" >/dev/null
  snapshot daemon-restart
  check present daemon-restart "$(cat "$out/client.id")"
else echo 'NOT RUN: Docker daemon restart (enable TEST_RESTART_DOCKER=1 on a dedicated test host)'; fi
if command -v promtool >/dev/null; then promtool check metrics <"$out/agent-restart.metrics"; else echo 'NOT RUN: promtool format validation'; fi
echo 'NOT RUN: IPv6, child-cgroup privileged traffic, NIC capture/GRO/GSO comparison and external Prometheus scrape (manual acceptance cases).'
echo "PASS: TCP/UDP loopback RX/TX, idle isolation, host isolation and bridge attribution, rename, restart, recreation and agent restart. Artifacts: $out"
