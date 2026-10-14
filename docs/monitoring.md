# Prometheus、Grafana与n9e

## Prometheus

合并下列scrape配置；远程抓取需更改目标与Exporter监听地址。

```yaml
scrape_configs:
  - job_name: luna-ebpf
    scrape_interval: 15s
    static_configs:
      - targets: ["127.0.0.1:9808"]
```

独立容器里的127.0.0.1指向容器自身；Prometheus可使用host网络或可达的宿主机地址。

## Grafana

导入[Dashboard JSON](../deploy/grafana/luna-ebpf-dashboard.json)，选择Prometheus数据源和job/instance/容器。包含18个面板：RX/TX速率、skb/s、累计值、健康状态与错误诊断。

速率使用rate处理Counter重置。标签不含network_mode。零流量不代表异常，不应默认告警。

## n9e

在Prometheus数据源告警规则里填写[规则文件](../deploy/prometheus/luna-alerts.yaml)中对应的expr，在UI配置持续时长，建议每30秒评估。文件采用Prometheus规则格式，不是n9e导入文件。

| 规则 | 条件 | 持续 |
|---|---|---|
| LunaScrapeDown | up为0 | 1分钟 |
| LunaMetricsMissing | up为1但luna_exporter_up缺失 | 3分钟 |
| LunaAgentUnhealthy | luna_exporter_up为0 | 1分钟 |
| LunaDockerDisconnected | docker_connected为0 | 2分钟 |
| LunaSampleStale | 端点可达且采样超过60秒未更新 | 1分钟 |
| LunaMapFailures | 5分钟内出现Map错误 | 1分钟 |
| LunaPollFailures | 5分钟内出现采样/Map操作错误 | 3分钟 |
| LunaHighTX / LunaHighRX | 容器TX/RX超过100Mbps | 5分钟 |

常用表达式：

```promql
up{job="luna-ebpf"} == 0

luna_exporter_docker_connected{job="luna-ebpf"} == 0

sum by(job, instance)(increase({__name__=~"luna_exporter_(map_insert_failures|lookup_failures)_total",job="luna-ebpf"}[5m])) > 0

sum by(job, instance, container_id, container_name)(rate(luna_container_tx_bytes_total{job="luna-ebpf"}[5m])) * 8 / 1000000 > 100
```

100Mbps是示例阈值，按业务调整。up由Prometheus产生，某些remote-write数据源可能没有它。目标被删除后up==0不能覆盖所有缺失情形，需维护预期目标清单。按ident路由时需添加目标ident并在聚合中保留。

## 诊断指标

| 指标 | 含义 |
|---|---|
| luna_exporter_last_success_timestamp_seconds | 最后完整成功采样时间 |
| luna_exporter_map_insert_failures_total | 插入失败，排除正常EEXIST |
| luna_exporter_lookup_failures_total | 插入后仍未查到条目 |
| luna_exporter_poll_errors_total | 采样或生命周期Map操作错误 |
| luna_exporter_discovery_errors_total | 发现失败或跳过 |
| luna_exporter_event_reconnects_total | 事件连接尝试，包含首次连接 |
| luna_exporter_unattributed_bytes_total | helper返回ID=0的匿名字节 |
| luna_exporter_unattributed_packets_total | helper返回ID=0的匿名skb次数 |

匿名计数带direction标签，不表示其他容器流量，也不能当作漏采总量。完整口径见[指标语义](metric-semantics.md)。
