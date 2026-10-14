//go:build !linux

package bpf

import (
	"errors"
	"github.com/AllenxuX/luna-ebpf/internal/collector"
)

var errLinux = errors.New("luna-ebpf requires Linux with cgroup v2; this build only supports portable tests")

type Loader struct{}

func Load(uint32) (*Loader, error)                                                 { return nil, errLinux }
func (*Loader) Attach(string) error                                                { return errLinux }
func (*Loader) Add(uint64) error                                                   { return errLinux }
func (*Loader) Remove(uint64) error                                                { return errLinux }
func (*Loader) Iterate(func(collector.TrafficKey, []collector.TrafficValue)) error { return errLinux }
func (*Loader) ReadDiagnostics() (collector.Diagnostics, error) {
	return collector.Diagnostics{}, errLinux
}
func (*Loader) Close() error { return nil }
