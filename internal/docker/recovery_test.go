package docker

import (
	"context"
	"errors"
	"github.com/moby/moby/api/types/container"
	"testing"
)

type transientResolver struct{ fail bool }

func (r *transientResolver) Resolve(pid int) (string, []uint64, error) {
	if r.fail {
		return "", nil, errors.New("child disappeared during scan")
	}
	return "/actual", []uint64{uint64(pid)}, nil
}
func TestUnresolvedRunningContainerKeepsVerifiedMapping(t *testing.T) {
	api := &fakeAPI{items: []container.InspectResponse{{ID: "known", Name: "/before", HostConfig: &container.HostConfig{NetworkMode: "host"}, State: &container.State{Running: true, Pid: 42}}}}
	resolver := &transientResolver{}
	d := Discoverer{API: api, Resolver: resolver}
	if got, _, err := d.List(context.Background()); err != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, err)
	}
	resolver.fail = true
	api.items[0].Name = "/renamed"
	got, skipped, err := d.List(context.Background())
	if err != nil || skipped != 1 || len(got) != 1 || got[0].CgroupID != 42 || got[0].Name != "renamed" {
		t.Fatalf("lost verified running mapping: %+v %d %v", got, skipped, err)
	}
	api.items[0].State = &container.State{Running: true, Pid: 43}
	got, _, _ = d.List(context.Background())
	if len(got) != 1 || got[0].CgroupID != 42 {
		t.Fatal("restart scan failure prematurely expires counter state")
	}
	resolver.fail = false
	got, _, _ = d.List(context.Background())
	if len(got) != 1 || got[0].CgroupID != 43 {
		t.Fatal("mapping did not recover")
	}
	api.items = nil
	got, _, _ = d.List(context.Background())
	if len(got) != 0 {
		t.Fatal("confirmed exit retained")
	}
	// No guessed identity for a newly seen container with no resolvable cgroup.
	resolver.fail = true
	api.items = []container.InspectResponse{{ID: "new", HostConfig: &container.HostConfig{NetworkMode: "host"}, State: &container.State{Running: true, Pid: 99}}}
	got, skipped, err = d.List(context.Background())
	if err != nil || len(got) != 0 || skipped != 1 {
		t.Fatal("unknown container attributed")
	}
}
