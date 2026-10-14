package collector

import (
	"sync/atomic"
	"time"
)

type ContainerInfo struct {
	ID, Name, NetworkMode, CgroupPath string
	PID                               int
	CgroupID                          uint64
	CgroupIDs                         []uint64
	CreatedAt, StartedAt              time.Time
}
type ContainerSample struct {
	Info   ContainerInfo
	Totals TrafficTotals
}
type Diagnostics struct{ MapInsertFailures, LookupFailures, ZeroRXBytes, ZeroRXPackets, ZeroTXBytes, ZeroTXPackets uint64 }
type Snapshot struct {
	Containers                                   []ContainerSample
	Up, DockerConnected                          bool
	RunningContainers                            int
	Diagnostics                                  Diagnostics
	PollErrors, DiscoveryErrors, EventReconnects uint64
	LastSuccess                                  time.Time
}

// Published snapshots and all their nested values must be immutable.
type Store struct{ value atomic.Pointer[Snapshot] }

func (s *Store) Load() *Snapshot {
	if v := s.value.Load(); v != nil {
		return v
	}
	return &Snapshot{}
}
func (s *Store) Publish(v Snapshot) {
	v.Containers = append([]ContainerSample(nil), v.Containers...)
	for i := range v.Containers {
		v.Containers[i].Info.CgroupIDs = append([]uint64(nil), v.Containers[i].Info.CgroupIDs...)
	}
	s.value.Store(&v)
}
