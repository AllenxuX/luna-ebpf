package metrics

import (
	"github.com/AllenxuX/luna-ebpf/internal/collector"
	"github.com/prometheus/client_golang/prometheus"
)

type Collector struct {
	store *collector.Store
	desc  map[string]*prometheus.Desc
}

func New(s *collector.Store) *Collector {
	c := &Collector{store: s, desc: map[string]*prometheus.Desc{}}
	for name, help := range map[string]string{
		"luna_container_rx_bytes_total":   "Cumulative socket ingress skb bytes at the selected backend observation point.",
		"luna_container_tx_bytes_total":   "Cumulative socket egress skb bytes at the selected backend observation point.",
		"luna_container_rx_packets_total": "Cumulative socket ingress skb observations, not wire packets.",
		"luna_container_tx_packets_total": "Cumulative socket egress skb observations, not wire packets.",
	} {
		c.desc[name] = prometheus.NewDesc(name, help, []string{"container_id", "container_name"}, nil)
	}
	for name, help := range map[string]string{
		"luna_exporter_up":                             "Selected BPF backend attached and the latest map poll successful.",
		"luna_exporter_docker_connected":               "Latest Docker inventory request and event stream health.",
		"luna_exporter_containers":                     "Running discovered Docker containers across all network modes.",
		"luna_exporter_map_insert_failures_total":      "Traffic map insert failures excluding concurrent EEXIST.",
		"luna_exporter_lookup_failures_total":          "Traffic map lookups still missing after insertion.",
		"luna_exporter_poll_errors_total":              "Failed complete map polls or lifecycle map operations.",
		"luna_exporter_discovery_errors_total":         "Skipped container discoveries and failed inventory requests.",
		"luna_exporter_event_reconnects_total":         "Docker event stream reconnect attempts.",
		"luna_exporter_last_success_timestamp_seconds": "Unix timestamp of the last complete successful map sample.",
	} {
		c.desc[name] = prometheus.NewDesc(name, help, nil, nil)
	}
	c.desc["luna_exporter_unattributed_bytes_total"] = prometheus.NewDesc("luna_exporter_unattributed_bytes_total", "Observed bytes with unresolved socket owner: v2 helper ID zero or socket tracing ownership unavailable.", []string{"direction"}, nil)
	c.desc["luna_exporter_unattributed_packets_total"] = prometheus.NewDesc("luna_exporter_unattributed_packets_total", "Observed skbs with unresolved socket owner: v2 helper ID zero or socket tracing ownership unavailable.", []string{"direction"}, nil)
	return c
}
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range c.desc {
		ch <- d
	}
}
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	s := c.store.Load()
	emit := func(name string, kind prometheus.ValueType, v float64, labels ...string) {
		ch <- prometheus.MustNewConstMetric(c.desc[name], kind, v, labels...)
	}
	for _, v := range s.Containers {
		labels := []string{v.Info.ID, v.Info.Name}
		emit("luna_container_rx_bytes_total", prometheus.CounterValue, float64(v.Totals.RXBytes), labels...)
		emit("luna_container_tx_bytes_total", prometheus.CounterValue, float64(v.Totals.TXBytes), labels...)
		emit("luna_container_rx_packets_total", prometheus.CounterValue, float64(v.Totals.RXPackets), labels...)
		emit("luna_container_tx_packets_total", prometheus.CounterValue, float64(v.Totals.TXPackets), labels...)
	}
	boolean := func(v bool) float64 {
		if v {
			return 1
		}
		return 0
	}
	emit("luna_exporter_up", prometheus.GaugeValue, boolean(s.Up))
	emit("luna_exporter_docker_connected", prometheus.GaugeValue, boolean(s.DockerConnected))
	emit("luna_exporter_containers", prometheus.GaugeValue, float64(s.RunningContainers))
	emit("luna_exporter_map_insert_failures_total", prometheus.CounterValue, float64(s.Diagnostics.MapInsertFailures))
	emit("luna_exporter_lookup_failures_total", prometheus.CounterValue, float64(s.Diagnostics.LookupFailures))
	emit("luna_exporter_poll_errors_total", prometheus.CounterValue, float64(s.PollErrors))
	emit("luna_exporter_discovery_errors_total", prometheus.CounterValue, float64(s.DiscoveryErrors))
	emit("luna_exporter_event_reconnects_total", prometheus.CounterValue, float64(s.EventReconnects))
	last := float64(0)
	if !s.LastSuccess.IsZero() {
		last = float64(s.LastSuccess.UnixNano()) / 1e9
	}
	emit("luna_exporter_last_success_timestamp_seconds", prometheus.GaugeValue, last)
	emit("luna_exporter_unattributed_bytes_total", prometheus.CounterValue, float64(s.Diagnostics.ZeroRXBytes), "rx")
	emit("luna_exporter_unattributed_bytes_total", prometheus.CounterValue, float64(s.Diagnostics.ZeroTXBytes), "tx")
	emit("luna_exporter_unattributed_packets_total", prometheus.CounterValue, float64(s.Diagnostics.ZeroRXPackets), "rx")
	emit("luna_exporter_unattributed_packets_total", prometheus.CounterValue, float64(s.Diagnostics.ZeroTXPackets), "tx")
}
