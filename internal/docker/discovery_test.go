package docker

import (
	"context"
	"errors"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"testing"
)

type fakeAPI struct {
	items []container.InspectResponse
	err   error
}

func (f *fakeAPI) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	var r client.ContainerListResult
	for _, v := range f.items {
		r.Items = append(r.Items, container.Summary{ID: v.ID})
	}
	return r, f.err
}
func (f *fakeAPI) ContainerInspect(_ context.Context, id string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	for _, v := range f.items {
		if v.ID == id {
			return client.ContainerInspectResult{Container: v}, nil
		}
	}
	return client.ContainerInspectResult{}, errors.New("gone")
}

type fakeResolver struct{}

func (fakeResolver) Resolve(pid int) (string, []uint64, error) {
	if pid <= 0 {
		return "", nil, errors.New("unavailable PID")
	}
	return "/actual", []uint64{uint64(pid)}, nil
}
func TestAllNetworkModes(t *testing.T) {
	api := &fakeAPI{}
	for i, m := range []string{"host", "bridge", "none", "custom-net", "container:peer", "macvlan-net", "ipvlan-net", "host"} {
		pid := i + 1
		if i == 7 {
			pid = 0
		}
		api.items = append(api.items, container.InspectResponse{ID: m + string(rune('a'+i)), Name: "/name", HostConfig: &container.HostConfig{NetworkMode: container.NetworkMode(m)}, State: &container.State{Running: true, Pid: pid}})
	}
	d := Discoverer{API: api, Resolver: fakeResolver{}}
	got, skipped, err := d.List(context.Background())
	if err != nil || len(got) != 7 || got[0].Name != "name" || skipped != 1 {
		t.Fatalf("%v %d %v", got, skipped, err)
	}
	for i, info := range got {
		if info.NetworkMode != string(api.items[i].HostConfig.NetworkMode) {
			t.Fatalf("lost network mode: %+v", info)
		}
	}
	api.err = errors.New("daemon unavailable")
	if _, _, err = d.List(context.Background()); err == nil {
		t.Fatal("lost API error")
	}
}
