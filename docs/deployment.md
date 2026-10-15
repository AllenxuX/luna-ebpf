# 部署与配置

## Docker Compose

Linux + Docker Engine + Compose v2 + cgroup v1/v2。既有 cgroup v2 / cgroup-skb 验证环境为 Rocky Linux 9.7 / 内核5.14 / Docker29.5.2。amd64已实测，arm64仅构建验证；v1 socket-trace 已在真实 v1 启动环境验证。

```bash
docker compose up -d --build
docker compose logs -f
curl -fsS http://127.0.0.1:9808/readyz
docker compose down
```

从源码构建本地 `luna-ebpf:0.1.3`，无预发布镜像依赖。9808须未被占用。Exporter使用host network/PID/cgroup视图，这不限制被监控容器的网络模式。

默认后端 `auto` 在 v2 使用 cgroup-skb，在 v1 使用 socket-trace。v1 从实际挂载的控制器层级获取内核句柄，优先 memory；须暴露完整宿主机 cgroup 根，不能只挂载容器子树。socket-trace 要求容器内可读宿主机 `/sys/kernel/btf/vmlinux` 及所需 kprobe；按实际宿主机提供 BTF 挂载。没有 BTF 的旧内核不支持该后端，不能靠指定偏移绕过。v2 也可显式使用 socket-trace。部署前以实际权限运行 `--check`。

Compose以privileged为兼容基线。Docker Socket的`:ro`不等于只读API。HTTP默认localhost，无认证/TLS；远程访问请显式调整监听参数并控制访问。

## systemd

安装Go 1.25.6+。源码包含BPF生成文件，直接构建不需要Clang。

```bash
make build
sudo ./bin/luna-ebpf --check
sudo install -m 0755 bin/luna-ebpf /usr/local/bin/luna-ebpf
sudo install -d /etc/luna-ebpf
sudo install -m 0644 configs/config.example.yaml /etc/luna-ebpf/config.yaml
sudo install -m 0644 deploy/systemd/luna-ebpf.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now luna-ebpf
sudo journalctl -u luna-ebpf -f
```

`--check`执行实际BPF加载、附加和采样，随后关闭自身资源，不能替代真实流量测试。systemd提供兼容权限集合，未验证所有内核上的最小权限。

## 配置

CLI > YAML > 默认值；对应YAML名称把连字符换为下划线。

| CLI参数 | 默认值 |
|---|---|
| `--listen-address` | `127.0.0.1:9808` |
| `--docker-socket` | `/var/run/docker.sock` |
| `--cgroup-root` | `/sys/fs/cgroup` |
| `--collector-backend` | `auto`；可选 `cgroup-skb`、`socket-trace` |
| `--proc-root` | `/proc`；Compose为`/host/proc` |
| `--metrics-poll-interval` | `5s` |
| `--docker-reconcile-interval` | `30s` |
| `--container-stale-grace-period` | `60s` |
| `--map-max-entries` | `8192` |
| `--cgroup-subtree-max-nodes` | `1024` |
| `--log-level` | `info` |

每个双向cgroup占两个traffic Key，默认最多4096个双向cgroup，包含子cgroup。容量按possible CPU和Map大小消耗内存。

socket-trace 的 socket owner Map 有界，容量为 `min(8 * map_max_entries, 1048576)`。它还在发现时通过 `cgroup.procs` 和 `/proc/PID/fd` 为已有 Socket 建立映射，增加发现工作量与权限要求。当前没有该后端的实测资源预算，请在目标环境测量。

退出后内核条目立即清理，指标冻结保留60秒。Agent重启或容器状态过期后重新启动会开始新计数周期。

| HTTP路径 | 用途 |
|---|---|
| `/metrics` | 最近完成的不可变采样快照 |
| `/healthz` | HTTP进程存活 |
| `/readyz` | BPF附加、采样与Docker健康 |

内核与权限故障说明见[运维](operations.md)。
