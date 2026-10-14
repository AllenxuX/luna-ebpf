package agent

import (
	"errors"
	"github.com/AllenxuX/luna-ebpf/internal/collector"
	"testing"
	"time"
)

type fakeMaps struct {
	allowed                       map[uint64]bool
	rows                          map[collector.TrafficKey][]collector.TrafficValue
	failRead, failAdd, failRemove bool
	store                         *collector.Store
	requireHidden                 bool
}

func (f *fakeMaps) Add(id uint64) error {
	if f.failAdd {
		return errors.New("map full")
	}
	f.allowed[id] = true
	return nil
}
func (f *fakeMaps) Remove(id uint64) error {
	if f.requireHidden && f.store.Load().RunningContainers != 0 {
		return errors.New("metadata still live")
	}
	if f.failRemove {
		return errors.New("delete failed")
	}
	delete(f.allowed, id)
	for k := range f.rows {
		if k.CgroupID == id {
			delete(f.rows, k)
		}
	}
	return nil
}
func (f *fakeMaps) Iterate(fn func(collector.TrafficKey, []collector.TrafficValue)) error {
	if f.failRead {
		return errors.New("read failed")
	}
	for k, v := range f.rows {
		fn(k, v)
	}
	return nil
}
func (f *fakeMaps) ReadDiagnostics() (collector.Diagnostics, error) {
	return collector.Diagnostics{}, nil
}
func fixture() (*Engine, *fakeMaps, *collector.Store) {
	s := &collector.Store{}
	m := &fakeMaps{allowed: map[uint64]bool{}, rows: map[collector.TrafficKey][]collector.TrafficValue{}, store: s}
	e := NewEngine(m, s, time.Minute)
	e.Attached = true
	e.DockerConnected = true
	return e, m, s
}
func info(id, name string, cg uint64) collector.ContainerInfo {
	return collector.ContainerInfo{ID: id, Name: name, CgroupID: cg, CgroupIDs: []uint64{cg}, CgroupPath: "/docker/" + id}
}
func TestLifecycle(t *testing.T) {
	e, m, s := fixture()
	now := time.Now()
	if err := e.Reconcile([]collector.ContainerInfo{info("a", "first", 10)}, now); err != nil {
		t.Fatal(err)
	}
	m.rows[collector.TrafficKey{CgroupID: 10}] = []collector.TrafficValue{{Bytes: 10, Packets: 2}, {Bytes: 20, Packets: 3}}
	if err := e.Poll(now); err != nil {
		t.Fatal(err)
	}
	if got := s.Load().Containers[0].Totals.RXBytes; got != 30 {
		t.Fatal(got)
	}
	e.Poll(now)
	if s.Load().Containers[0].Totals.RXBytes != 30 {
		t.Fatal("double count")
	}
	e.Reconcile([]collector.ContainerInfo{info("a", "renamed", 11)}, now)
	m.rows[collector.TrafficKey{CgroupID: 11, Direction: 1}] = []collector.TrafficValue{{Bytes: 7, Packets: 1}}
	e.Poll(now)
	if len(m.allowed) != 1 || m.allowed[10] || s.Load().Containers[0].Totals.RXBytes != 30 || s.Load().Containers[0].Totals.TXBytes != 7 || s.Load().Containers[0].Info.Name != "renamed" {
		t.Fatalf("%+v %v", s.Load(), m.allowed)
	}
	m.requireHidden = true
	if err := e.Reconcile(nil, now); err != nil {
		t.Fatal(err)
	}
	if len(m.allowed) != 0 || len(m.rows) != 0 || len(s.Load().Containers) != 1 || s.Load().RunningContainers != 0 {
		t.Fatal("stop cleanup/grace")
	}
	e.Poll(now.Add(61 * time.Second))
	if len(s.Load().Containers) != 0 {
		t.Fatal("unbounded stale metrics")
	}
}
func TestFailuresAndIDReuse(t *testing.T) {
	e, m, s := fixture()
	now := time.Now()
	e.Reconcile([]collector.ContainerInfo{info("a", "a", 10)}, now)
	m.failRead = true
	if err := e.Poll(now); err == nil || s.Load().Up || s.Load().PollErrors != 1 {
		t.Fatal("poll health")
	}
	m.failRead = false
	m.failRemove = true
	if err := e.Reconcile([]collector.ContainerInfo{info("b", "b", 10)}, now); err == nil {
		t.Fatal("cleanup failure suppressed")
	}
	if s.Load().RunningContainers != 0 {
		t.Fatal("reused ID assigned before cleanup")
	}
	m.failRemove = false
	if err := e.Reconcile([]collector.ContainerInfo{info("b", "b", 10)}, now); err != nil {
		t.Fatal(err)
	}
	m.rows[collector.TrafficKey{CgroupID: 10}] = []collector.TrafficValue{{Bytes: 9}}
	e.Poll(now)
	for _, v := range s.Load().Containers {
		if v.Info.ID == "b" && v.Totals.RXBytes != 9 {
			t.Fatal(v)
		}
	}
	m.failAdd = true
	if err := e.Reconcile([]collector.ContainerInfo{info("b", "b", 10), info("c", "c", 12)}, now); err == nil {
		t.Fatal("allowlist error")
	}
	if m.allowed[12] {
		t.Fatal("partial add")
	}
}
func TestNearestAncestor(t *testing.T) {
	outer := info("outer", "outer", 1)
	outer.CgroupPath = "/a"
	outer.CgroupIDs = []uint64{1, 2, 3}
	inner := info("inner", "inner", 2)
	inner.CgroupPath = "/a/b"
	inner.CgroupIDs = []uint64{2, 3}
	owners, err := Ownership([]collector.ContainerInfo{outer, inner})
	if err != nil || owners[3] != "inner" || owners[1] != "outer" {
		t.Fatalf("%v %v", owners, err)
	}
	other := info("other", "other", 3)
	other.CgroupPath = "/unrelated"
	if _, err = Ownership([]collector.ContainerInfo{outer, other}); err == nil {
		t.Fatal("ambiguous ID accepted")
	}
}

func TestLateUnregisteredMapEntryIsCleaned(t *testing.T) {
	e, m, _ := fixture()
	now := time.Now()
	e.Reconcile([]collector.ContainerInfo{info("a", "a", 10)}, now)
	e.Reconcile(nil, now)
	// An in-flight hook may reinsert a traffic key after userspace deletes it.
	m.rows[collector.TrafficKey{CgroupID: 10}] = []collector.TrafficValue{{Bytes: 99}}
	if err := e.Poll(now); err != nil {
		t.Fatal(err)
	}
	if len(m.rows) != 0 {
		t.Fatal("orphan traffic entry leaked")
	}
}
