//go:build linux

package bpf

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/AllenxuX/luna-ebpf/internal/collector"
)

type Loader struct {
	objects trafficObjects
	links   []link.Link
}

func Load(entries uint32) (*Loader, error) {
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("remove memlock limit (check CAP_SYS_RESOURCE on older kernels): %w", err)
	}
	spec, err := loadTraffic()
	if err != nil {
		return nil, fmt.Errorf("decode embedded BPF: %w", err)
	}
	spec.Maps["traffic"].MaxEntries = entries
	spec.Maps["monitored_cgroups"].MaxEntries = entries
	l := &Loader{}
	if err = spec.LoadAndAssign(&l.objects, nil); err != nil {
		var verifier *ebpf.VerifierError
		if errors.As(err, &verifier) {
			slog.Error("BPF verifier rejected object", "op", "bpf_load", "verifier_log", fmt.Sprintf("%+v", verifier))
		}
		reason := "kernel/helper support or verifier rejection"
		if errors.Is(err, os.ErrPermission) {
			reason = "permission denied (capabilities, seccomp or kernel lockdown)"
		} else if errors.Is(err, ebpf.ErrNotSupported) {
			reason = "required BPF kernel capability unsupported"
		}
		return nil, fmt.Errorf("load cgroup_skb programs/helper bpf_skb_cgroup_id: %s; check CAP_BPF+CAP_NET_ADMIN (or CAP_SYS_ADMIN), bpftool feature probe and verifier log: %w", reason, err)
	}
	slog.Info("BPF maps loaded", "traffic_max_entries", entries, "allowlist_max_entries", entries)
	return l, nil
}

// Attach is called only after the initial inventory has populated the allowlist.
func (l *Loader) Attach(root string) error {
	for _, p := range []struct {
		program *ebpf.Program
		kind    ebpf.AttachType
	}{{l.objects.Ingress, ebpf.AttachCGroupInetIngress}, {l.objects.Egress, ebpf.AttachCGroupInetEgress}} {
		attached, err := link.AttachCgroup(link.CgroupOptions{Path: root, Attach: p.kind, Program: p.program})
		if err != nil {
			for _, v := range l.links {
				_ = v.Close()
			}
			l.links = nil
			return fmt.Errorf("attach %s at %s non-destructively: %w; inspect bpftool cgroup tree, existing exclusive attachments, host cgroup namespace and capabilities", p.kind, root, err)
		}
		l.links = append(l.links, attached)
	}
	return nil
}
func (l *Loader) Add(id uint64) error {
	if id == 0 {
		return errors.New("refuse allowlist ID zero")
	}
	return l.objects.MonitoredCgroups.Update(id, uint32(1), ebpf.UpdateAny)
}
func (l *Loader) Remove(id uint64) error {
	// Stop new accounting before deleting both direction keys. ENOENT is idempotent.
	if err := l.objects.MonitoredCgroups.Delete(id); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
		return err
	}
	for d := uint32(0); d < 2; d++ {
		err := l.objects.Traffic.Delete(collector.TrafficKey{CgroupID: id, Direction: d})
		if err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
			return err
		}
	}
	return nil
}
func (l *Loader) Iterate(fn func(collector.TrafficKey, []collector.TrafficValue)) error {
	n, err := ebpf.PossibleCPU()
	if err != nil {
		return err
	}
	values := make([]collector.TrafficValue, n)
	it := l.objects.Traffic.Iterate()
	var k collector.TrafficKey
	for it.Next(&k, &values) {
		fn(k, values)
	}
	return it.Err()
}
func (l *Loader) ReadDiagnostics() (collector.Diagnostics, error) {
	n, err := ebpf.PossibleCPU()
	if err != nil {
		return collector.Diagnostics{}, err
	}
	values := make([]trafficDiagnostics, n)
	if err = l.objects.DiagnosticCounters.Lookup(uint32(0), &values); err != nil {
		return collector.Diagnostics{}, err
	}
	var d collector.Diagnostics
	for _, v := range values {
		d.MapInsertFailures += v.MapInsertFailures
		d.LookupFailures += v.LookupFailures
		d.ZeroRXBytes += v.ZeroRxBytes
		d.ZeroRXPackets += v.ZeroRxPackets
		d.ZeroTXBytes += v.ZeroTxBytes
		d.ZeroTXPackets += v.ZeroTxPackets
	}
	return d, nil
}
func (l *Loader) Close() error {
	var errs []error
	for _, v := range l.links {
		errs = append(errs, v.Close())
	}
	l.links = nil
	errs = append(errs, l.objects.Close())
	return errors.Join(errs...)
}
