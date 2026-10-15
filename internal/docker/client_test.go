package docker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moby/moby/client"
)

func TestClientHonorsCancellationAndPingFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "denied", http.StatusUnauthorized) }))
	defer srv.Close()
	if api, err := NewClient(ctx, srv.URL); api != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("client=%v err=%v", api, err)
	}
	if api, err := NewClient(context.Background(), srv.URL); api != nil || err == nil {
		t.Fatalf("client=%v err=%v", api, err)
	}
}

func TestClientPingGETFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("API-Version", "1.43")
		fmt.Fprint(w, "OK")
	}))
	defer srv.Close()
	api, err := NewClient(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	if api.ClientVersion() != "1.43" {
		t.Fatal(api.ClientVersion())
	}
}

func TestClientAPIVersionCompatibility(t *testing.T) {
	for _, version := range []string{"1.41", "1.42", "1.43", "1.44", "1.52", "1.54"} {
		t.Run(version, func(t *testing.T) {
			selected := version
			if version == "1.54" {
				selected = client.MaxAPIVersion
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/_ping" {
					w.Header().Set("API-Version", version)
					fmt.Fprint(w, "OK")
					return
				}
				if !strings.HasPrefix(r.URL.Path, "/v"+selected+"/") {
					http.Error(w, "unsupported API", 400)
					return
				}
				switch strings.TrimPrefix(r.URL.Path, "/v"+selected) {
				case "/version":
					fmt.Fprintf(w, `{"Version":"test","ApiVersion":%q}`, version)
				case "/info":
					fmt.Fprint(w, `{"CgroupVersion":"2","CgroupDriver":"systemd"}`)
				case "/containers/json":
					fmt.Fprint(w, `[{"Id":"host-id"}]`)
				case "/containers/host-id/json":
					fmt.Fprint(w, `{"Id":"host-id","Name":"/host","HostConfig":{"NetworkMode":"host"},"State":{"Running":true,"Pid":42}}`)
				case "/events":
					fmt.Fprintln(w, `{"Type":"container","Action":"start","Actor":{"ID":"host-id"}}`)
				default:
					http.Error(w, "unexpected path", 404)
				}
			}))
			defer srv.Close()
			ctx := context.Background()
			api, err := NewClient(ctx, srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer api.Close()
			if api.ClientVersion() != selected {
				t.Fatalf("version=%s want=%s", api.ClientVersion(), selected)
			}
			if _, err = api.ServerVersion(ctx, client.ServerVersionOptions{}); err != nil {
				t.Fatal(err)
			}
			info, err := api.Info(ctx, client.InfoOptions{})
			if err != nil || info.Info.CgroupVersion != "2" {
				t.Fatalf("info=%+v err=%v", info, err)
			}
			d := Discoverer{API: api, Resolver: fakeResolver{}}
			got, skipped, err := d.List(ctx)
			if err != nil || skipped != 0 || len(got) != 1 {
				t.Fatalf("got=%v skipped=%d err=%v", got, skipped, err)
			}
			events := api.Events(ctx, client.EventsListOptions{})
			select {
			case event := <-events.Messages:
				if event.Action != "start" {
					t.Fatal(event)
				}
			case err := <-events.Err:
				t.Fatalf("events: %v", err)
			}
		})
	}
}

func TestClientRejectsUnsupportedAPIVersion(t *testing.T) {
	for _, version := range []string{"", "1.40", "garbage", "1.43/evil", "2.0"} {
		t.Run(version, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Header().Set("API-Version", version) }))
			defer srv.Close()
			api, err := NewClient(context.Background(), srv.URL)
			if api != nil {
				api.Close()
			}
			if err == nil {
				t.Fatalf("accepted %q", version)
			}
		})
	}
}
