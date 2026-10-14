package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/AllenxuX/luna-ebpf/internal/collector"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestMetrics(t *testing.T) {
	s := &collector.Store{}
	s.Publish(collector.Snapshot{Up: true, DockerConnected: true, RunningContainers: 1, Containers: []collector.ContainerSample{{Info: collector.ContainerInfo{ID: "full-id", Name: "test"}}}})
	r := prometheus.NewPedanticRegistry()
	if err := r.Register(New(s)); err != nil {
		t.Fatal(err)
	}
	render := func() string {
		w := httptest.NewRecorder()
		promhttp.HandlerFor(r, promhttp.HandlerOpts{}).ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		return w.Body.String()
	}
	body := render()
	for _, want := range []string{"# HELP luna_container_rx_bytes_total", "# TYPE luna_container_rx_bytes_total counter", `luna_container_rx_bytes_total{container_id="full-id",container_name="test"} 0`, "luna_exporter_up 1"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s in %s", want, body)
		}
	}
	s.Publish(collector.Snapshot{})
	if strings.Contains(render(), `container_id="full-id"`) {
		t.Fatal("stale series")
	}
}
func TestConcurrentCollection(t *testing.T) {
	s := &collector.Store{}
	r := prometheus.NewRegistry()
	r.MustRegister(New(s))
	var wg sync.WaitGroup
	for j := 0; j < 5; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				s.Publish(collector.Snapshot{Up: true})
				if _, err := r.Gather(); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestLunaMetricContract(t *testing.T) {
	s := &collector.Store{}
	s.Publish(collector.Snapshot{Containers: []collector.ContainerSample{{Info: collector.ContainerInfo{ID: "full-id", Name: "test"}}}})
	r := prometheus.NewPedanticRegistry()
	r.MustRegister(New(s))
	families, err := r.Gather()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for _, name := range []string{
		"luna_container_rx_bytes_total", "luna_container_tx_bytes_total", "luna_container_rx_packets_total", "luna_container_tx_packets_total",
		"luna_exporter_up", "luna_exporter_docker_connected", "luna_exporter_containers", "luna_exporter_last_success_timestamp_seconds",
		"luna_exporter_map_insert_failures_total", "luna_exporter_lookup_failures_total", "luna_exporter_poll_errors_total", "luna_exporter_discovery_errors_total", "luna_exporter_event_reconnects_total",
		"luna_exporter_unattributed_bytes_total", "luna_exporter_unattributed_packets_total",
	} {
		want[name] = true
	}
	for _, family := range families {
		name := family.GetName()
		if !want[name] {
			t.Fatalf("unexpected metric family %q; only luna_* is supported", name)
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Fatalf("missing metrics: %v", want)
	}
}
