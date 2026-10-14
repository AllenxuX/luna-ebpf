package docker

import (
	"context"
	"github.com/moby/moby/api/types/container"
	"testing"
)

type racingResolver struct{ api *fakeAPI }

func (r racingResolver) Resolve(int) (string, []uint64, error) {
	// PID was inspected while A ran, but is now associated with somebody else.
	r.api.items[0].State = &container.State{Running: false, Pid: 0}
	return "/docker/somebody-else", []uint64{123}, nil
}
func TestPIDReuseDuringDiscoveryIsRejected(t *testing.T) {
	api := &fakeAPI{items: []container.InspectResponse{{ID: "a", Name: "/a", HostConfig: &container.HostConfig{NetworkMode: "host"}, State: &container.State{Running: true, Pid: 42}}}}
	d := Discoverer{API: api, Resolver: racingResolver{api}}
	got, skipped, err := d.List(context.Background())
	if err != nil || len(got) != 0 || skipped != 1 {
		t.Fatalf("PID race wrongly attributed: %+v %d %v", got, skipped, err)
	}
}
