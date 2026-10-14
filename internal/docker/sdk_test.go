package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moby/moby/client"
)

// Exercise the pinned SDK against real HTTP responses, not just an interface fake.
func TestSDKDiscoveryHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1.52/containers/json":
			fmt.Fprint(w, `[{"Id":"host-id"},{"Id":"bridge-id"},{"Id":"none-id"}]`)
		case strings.HasPrefix(r.URL.Path, "/v1.52/containers/"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1.52/containers/"), "/json")
			mode := strings.TrimSuffix(id, "-id")
			json.NewEncoder(w).Encode(map[string]any{"Id": id, "Name": "/" + mode, "HostConfig": map[string]any{"NetworkMode": mode}, "State": map[string]any{"Running": true, "Pid": 42, "StartedAt": "2026-10-13T00:00:00Z"}})
		default:
			http.Error(w, "unexpected path "+r.URL.Path, 404)
		}
	}))
	defer server.Close()
	api, err := client.New(client.WithHost(server.URL), client.WithVersion("1.52"))
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	d := Discoverer{API: api, Resolver: fakeResolver{}}
	got, skipped, err := d.List(context.Background())
	if err != nil || skipped != 0 || len(got) != 3 || got[0].ID != "host-id" || got[0].PID != 42 || got[0].StartedAt.IsZero() {
		t.Fatalf("%+v %d %v", got, skipped, err)
	}
	server.Close()
	if _, _, err = d.List(context.Background()); err == nil {
		t.Fatal("transport failure ignored")
	}
}
