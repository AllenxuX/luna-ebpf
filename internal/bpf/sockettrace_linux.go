//go:build linux

package bpf

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AllenxuX/luna-ebpf/internal/cgroup"
	"github.com/AllenxuX/luna-ebpf/internal/collector"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
)

func LoadSocketTrace(entries uint32, proc, controller string) (*Loader, error) {
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, err
	}
	kernel, err := btf.LoadKernelSpec()
	if err != nil {
		return nil, fmt.Errorf("socket-trace requires readable kernel BTF (/sys/kernel/btf/vmlinux): %w", err)
	}
	offsets, err := traceOffsets(kernel, controller)
	if err != nil {
		return nil, fmt.Errorf("socket-trace kernel layout unsupported: %w", err)
	}
	if err = validateProbeSignatures(kernel); err != nil {
		return nil, err
	}
	var destructor *btf.Func
	if err = kernel.TypeByName("__sk_destruct", &destructor); err != nil {
		return nil, err
	}
	if probeParamIs(destructor.Type, 0, "sock") {
		offsets.SkRcu = 0
	}
	spec, err := loadSockettrace()
	if err != nil {
		return nil, err
	}
	if err = spec.RewriteConstants(map[string]interface{}{"settings": offsets}); err != nil {
		return nil, err
	}
	capacity := entries * 8
	if capacity > 1048576 {
		capacity = 1048576
	}
	for _, name := range []string{"traffic", "monitored_cgroups"} {
		spec.Maps[name].MaxEntries = entries
	}
	for _, name := range []string{"socket_owners", "inode_owners"} {
		spec.Maps[name].MaxEntries = capacity
	}
	l := &Loader{trace: &sockettraceObjects{}, procRoot: proc, controller: controller, socketCapacity: capacity}
	var initFn *btf.Func
	if err := kernel.TypeByName("sock_init_data_uid", &initFn); err == nil {
		if !probeParamIs(initFn.Type, 0, "socket") || !probeParamIs(initFn.Type, 1, "sock") {
			return nil, fmt.Errorf("unsupported sock_init_data_uid signature")
		}
		l.initUID = true
	}
	if err = spec.LoadAndAssign(l.trace, nil); err != nil {
		var verifier *ebpf.VerifierError
		if errors.As(err, &verifier) {
			slog.Error("socket tracing verifier rejected program", "verifier_log", fmt.Sprintf("%+v", verifier))
		}
		return nil, fmt.Errorf("load socket-trace programs (requires BTF, kprobes, BPF permissions and supported TCP/UDP symbols): %w", err)
	}
	l.objects.trafficMaps = trafficMaps{Traffic: l.trace.Traffic, MonitoredCgroups: l.trace.MonitoredCgroups, DiagnosticCounters: l.trace.DiagnosticCounters}
	slog.Info("socket tracing loaded", "controller", controller, "controller_index", offsets.ControllerIndex, "socket_capacity", capacity)
	return l, nil
}

func (l *Loader) attachSocketTrace() error {
	probes := []struct {
		name    string
		program *ebpf.Program
	}{
		{"sock_init_data", l.trace.TraceCreate}, {"security_sk_clone", l.trace.TraceClone},
		{"sk_destruct", l.trace.TraceDestroy}, {"tcp_sendmsg", l.trace.TraceSendmsg},
		{"__sk_destruct", l.trace.TraceDestroyRcu},
		{"udp_sendmsg", l.trace.TraceSendmsg}, {"udpv6_sendmsg", l.trace.TraceSendmsg},
		{"ip_output", l.trace.TraceIp4}, {"ip6_output", l.trace.TraceIp6},
		{"tcp_v4_do_rcv", l.trace.TraceTcp4}, {"tcp_v6_do_rcv", l.trace.TraceTcp6},
		{"__udp_enqueue_schedule_skb", l.trace.TraceUdp},
	}
	if l.initUID {
		probes = append(probes, struct {
			name    string
			program *ebpf.Program
		}{"sock_init_data_uid", l.trace.TraceCreate})
	}
	for _, p := range probes {
		attached, err := link.Kprobe(p.name, p.program, nil)
		if err != nil {
			for _, v := range l.links {
				_ = v.Close()
			}
			l.links = nil
			return fmt.Errorf("attach socket-trace kprobe %s (kernel symbol/probe permission required): %w", p.name, err)
		}
		l.links = append(l.links, attached)
	}
	return nil
}

func removeOwned(m *ebpf.Map, id uint64) error {
	var key, value uint64
	var keys []uint64
	it := m.Iterate()
	for it.Next(&key, &value) {
		if value == id {
			keys = append(keys, key)
		}
	}
	if err := it.Err(); err != nil {
		return err
	}
	for _, key := range keys {
		if err := m.Delete(key); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
			return err
		}
	}
	return nil
}

// Seed pre-existing sockets from validated cgroup process membership, never
// from current RX context. New sockets are tracked by creation/clone probes.
func (l *Loader) SyncContainers(containers []collector.ContainerInfo) error {
	if l.trace == nil {
		return nil
	}
	desired, err := socketInventory(l.procRoot, l.controller, containers, int(l.socketCapacity))
	if err != nil {
		return err
	}
	var inode, id uint64
	existing := make(map[uint64]uint64)
	it := l.trace.InodeOwners.Iterate()
	for it.Next(&inode, &id) {
		existing[inode] = id
	}
	if err = it.Err(); err != nil {
		return err
	}
	return syncInodeOwners(existing, desired, func(inode uint64) error {
		err := l.trace.InodeOwners.Delete(inode)
		if errors.Is(err, ebpf.ErrKeyNotExist) {
			return nil
		}
		return err
	}, func(inode, id uint64) error {
		if err := l.trace.InodeOwners.Update(inode, id, ebpf.UpdateAny); err != nil {
			return fmt.Errorf("seed socket inode allowlist: %w", err)
		}
		return nil
	})
}

func socketInventory(proc, controller string, containers []collector.ContainerInfo, limit int) (map[uint64]uint64, error) {
	out := map[uint64]uint64{}
	scanned := 0
	for _, info := range containers {
		main, err := os.ReadFile(filepath.Join(proc, strconv.Itoa(info.PID), "cgroup"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		mainPath, err := cgroup.ParseFor(string(main), controller)
		if err != nil {
			return nil, err
		}
		physical := filepath.ToSlash(info.CgroupPath)
		if mainPath == "/" || !strings.HasSuffix(physical, mainPath) {
			return nil, fmt.Errorf("container %s moved cgroup before socket scan", info.ID)
		}
		root := strings.TrimSuffix(physical, mainPath)
		err = filepath.WalkDir(info.CgroupPath, func(dir string, entry fs.DirEntry, walkErr error) error {
			scanned++
			if scanned > limit*8 {
				return fmt.Errorf("socket cgroup/FD scan exceeds bounded limit")
			}
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if !entry.IsDir() {
				return nil
			}
			// Match Engine.Ownership: an actual nested Docker container
			// owns its subtree instead of being seeded as its parent's socket.
			if dir != info.CgroupPath {
				for _, other := range containers {
					if filepath.Clean(dir) == filepath.Clean(other.CgroupPath) {
						return fs.SkipDir
					}
				}
			}
			procs, err := os.ReadFile(filepath.Join(dir, "cgroup.procs"))
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			expected := strings.TrimPrefix(filepath.ToSlash(dir), root)
			for _, pid := range strings.Fields(string(procs)) {
				scanned++
				if scanned > limit*8 {
					return fmt.Errorf("socket process scan exceeds bounded limit")
				}
				if _, err := strconv.ParseUint(pid, 10, 32); err != nil {
					return fmt.Errorf("invalid cgroup PID %q", pid)
				}
				base := filepath.Join(proc, pid)
				before, err := os.ReadFile(filepath.Join(base, "cgroup"))
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				if err != nil {
					return err
				}
				current, err := cgroup.ParseFor(string(before), controller)
				if err != nil {
					return err
				}
				if current != expected {
					continue
				}
				fds, err := os.ReadDir(filepath.Join(base, "fd"))
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				if err != nil {
					return err
				}
				var inodes []uint64
				for _, fd := range fds {
					scanned++
					if scanned > limit*8 {
						return fmt.Errorf("socket FD scan exceeds bounded limit")
					}
					target, err := os.Readlink(filepath.Join(base, "fd", fd.Name()))
					if errors.Is(err, os.ErrNotExist) {
						continue
					}
					if err != nil {
						return err
					}
					if !strings.HasPrefix(target, "socket:[") || !strings.HasSuffix(target, "]") {
						continue
					}
					n, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]"), 10, 64)
					if err != nil || n == 0 {
						continue
					}
					inodes = append(inodes, n)
				}
				after, err := os.ReadFile(filepath.Join(base, "cgroup"))
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				if err != nil {
					return err
				}
				if string(before) != string(after) {
					continue
				}
				for _, inode := range inodes {
					if previous, ok := out[inode]; ok && previous != info.CgroupID {
						return fmt.Errorf("socket inode %d shared by different container cgroups; refusing ambiguous seed", inode)
					}
					out[inode] = info.CgroupID
					if len(out) > limit {
						return fmt.Errorf("socket inventory exceeds capacity %d", limit)
					}
				}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan existing sockets for container %s: %w", info.ID, err)
		}
	}
	return out, nil
}
