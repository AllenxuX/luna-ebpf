package agent

import (
	"fmt"
	"github.com/AllenxuX/luna-ebpf/internal/collector"
	"sort"
	"strings"
	"time"
)

type Maps interface {
	collector.Iterator
	Add(uint64) error
	Remove(uint64) error
	ReadDiagnostics() (collector.Diagnostics, error)
}
type state struct {
	info    collector.ContainerInfo
	totals  collector.TrafficTotals
	stopped time.Time
}

// Engine is owned by one goroutine; only the Store is shared with HTTP handlers.
type Engine struct {
	maps                                         Maps
	store                                        *collector.Store
	grace                                        time.Duration
	states                                       map[string]*state
	owners                                       map[uint64]string
	previous                                     map[collector.TrafficKey]collector.TrafficValue
	pending                                      map[uint64]bool
	Attached, DockerConnected                    bool
	pollOK                                       bool
	PollErrors, DiscoveryErrors, EventReconnects uint64
	last                                         time.Time
	diagnostics                                  collector.Diagnostics
}

func NewEngine(m Maps, s *collector.Store, grace time.Duration) *Engine {
	return &Engine{maps: m, store: s, grace: grace, states: map[string]*state{}, owners: map[uint64]string{}, previous: map[collector.TrafficKey]collector.TrafficValue{}, pending: map[uint64]bool{}}
}
func Ownership(in []collector.ContainerInfo) (map[uint64]string, error) {
	sorted := append([]collector.ContainerInfo(nil), in...)
	sort.Slice(sorted, func(i, j int) bool { return len(sorted[i].CgroupPath) > len(sorted[j].CgroupPath) })
	out := map[uint64]string{}
	paths := map[string]string{}
	seen := map[string]bool{}
	for _, v := range sorted {
		if v.ID == "" || v.CgroupID == 0 || len(v.CgroupIDs) == 0 || seen[v.ID] {
			return nil, fmt.Errorf("invalid or duplicate container identity %q", v.ID)
		}
		seen[v.ID] = true
		paths[v.ID] = v.CgroupPath
		for _, id := range v.CgroupIDs {
			if id == 0 {
				return nil, fmt.Errorf("zero child cgroup for %s", v.ID)
			}
			if old, ok := out[id]; ok && old != v.ID {
				// Deepest actual Docker subtree wins; unrelated overlapping IDs are unsafe.
				if !strings.HasPrefix(paths[old], strings.TrimRight(v.CgroupPath, "/")+"/") {
					return nil, fmt.Errorf("ambiguous cgroup ID %d for %s and %s", id, old, v.ID)
				}
				continue
			}
			out[id] = v.ID
		}
	}
	return out, nil
}
func (e *Engine) cleanup() error {
	for id := range e.pending {
		if err := e.maps.Remove(id); err != nil {
			return fmt.Errorf("clean cgroup %d allowlist and traffic: %w", id, err)
		}
		delete(e.pending, id)
		delete(e.previous, collector.TrafficKey{CgroupID: id})
		delete(e.previous, collector.TrafficKey{CgroupID: id, Direction: 1})
	}
	return nil
}
func (e *Engine) fail(err error) error { e.PollErrors++; e.pollOK = false; e.Publish(); return err }
func (e *Engine) Reconcile(in []collector.ContainerInfo, now time.Time) error {
	desired, err := Ownership(in)
	if err != nil {
		return e.fail(err)
	}
	incoming := map[string]bool{}
	for _, v := range in {
		incoming[v.ID] = true
	}
	// Freeze old counters and publish retirement BEFORE touching kernel maps.
	for id, s := range e.states {
		if !incoming[id] && s.stopped.IsZero() {
			s.stopped = now
		}
	}
	for id, owner := range e.owners {
		if desired[id] != owner {
			delete(e.owners, id)
			e.pending[id] = true
		}
	}
	e.expire(now)
	e.Publish()
	if err = e.cleanup(); err != nil {
		return e.fail(err)
	}
	var added []uint64
	for id := range desired {
		if _, exists := e.owners[id]; exists {
			continue
		}
		if err = e.maps.Add(id); err != nil {
			for _, a := range added {
				e.pending[a] = true
			}
			_ = e.cleanup()
			return e.fail(fmt.Errorf("populate monitored_cgroups ID %d: %w", id, err))
		}
		added = append(added, id)
	}
	// All new IDs are registered; now expose metadata.
	e.owners = desired
	for _, v := range in {
		s := e.states[v.ID]
		if s == nil {
			s = &state{}
			e.states[v.ID] = s
		}
		s.info = v
		s.stopped = time.Time{}
	}
	e.Publish()
	return nil
}
func (e *Engine) expire(now time.Time) {
	for id, s := range e.states {
		if !s.stopped.IsZero() && !now.Before(s.stopped.Add(e.grace)) {
			delete(e.states, id)
		}
	}
}
func (e *Engine) Poll(now time.Time) error {
	if err := e.cleanup(); err != nil {
		return e.fail(err)
	}
	allowed := map[uint64]bool{}
	for id := range e.owners {
		allowed[id] = true
	}
	// Retry late orphan keys from hooks in flight during retirement.
	raw, err := collector.ReadTotals(orphanIterator{engine: e, allowed: allowed}, allowed)
	if err != nil {
		return e.fail(err)
	}
	if err = e.cleanup(); err != nil {
		return e.fail(err)
	}
	d, err := e.maps.ReadDiagnostics()
	if err != nil {
		return e.fail(fmt.Errorf("read diagnostic per-CPU map: %w", err))
	}
	for k, v := range raw {
		s := e.states[e.owners[k.CgroupID]]
		if s == nil || !s.stopped.IsZero() {
			continue
		}
		old := e.previous[k]
		delta := collector.TrafficValue{Bytes: v.Bytes - old.Bytes, Packets: v.Packets - old.Packets}
		// Unexpected map resets are a new raw epoch, not uint64 underflow.
		if v.Bytes < old.Bytes {
			delta.Bytes = v.Bytes
		}
		if v.Packets < old.Packets {
			delta.Packets = v.Packets
		}
		if k.Direction == 0 {
			s.totals.RXBytes += delta.Bytes
			s.totals.RXPackets += delta.Packets
		} else {
			s.totals.TXBytes += delta.Bytes
			s.totals.TXPackets += delta.Packets
		}
		e.previous[k] = v
	}
	e.diagnostics = d
	e.last = now
	e.pollOK = true
	e.expire(now)
	e.Publish()
	return nil
}

type orphanIterator struct {
	engine  *Engine
	allowed map[uint64]bool
}

func (o orphanIterator) Iterate(fn func(collector.TrafficKey, []collector.TrafficValue)) error {
	return o.engine.maps.Iterate(func(k collector.TrafficKey, v []collector.TrafficValue) {
		if !o.allowed[k.CgroupID] {
			o.engine.pending[k.CgroupID] = true
		}
		fn(k, v)
	})
}
func (e *Engine) Publish() {
	s := collector.Snapshot{Up: e.Attached && e.pollOK, DockerConnected: e.DockerConnected, PollErrors: e.PollErrors, DiscoveryErrors: e.DiscoveryErrors, EventReconnects: e.EventReconnects, LastSuccess: e.last, Diagnostics: e.diagnostics}
	for _, v := range e.states {
		if v.stopped.IsZero() {
			s.RunningContainers++
		}
		s.Containers = append(s.Containers, collector.ContainerSample{Info: v.info, Totals: v.totals})
	}
	sort.Slice(s.Containers, func(i, j int) bool { return s.Containers[i].Info.ID < s.Containers[j].Info.ID })
	e.store.Publish(s)
}
