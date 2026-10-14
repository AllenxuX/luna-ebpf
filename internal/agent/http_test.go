package agent

import (
	"github.com/AllenxuX/luna-ebpf/internal/collector"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPHealth(t *testing.T) {
	s := &collector.Store{}
	h := Handler(s)
	check := func(path string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != want {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	check("/healthz", 200)
	check("/readyz", 503)
	check("/metrics", 200)
	s.Publish(collector.Snapshot{Up: true, DockerConnected: true, LastSuccess: time.Now()})
	check("/readyz", 200)
	s.Publish(collector.Snapshot{Up: true, LastSuccess: time.Now()})
	check("/readyz", 503)
}
