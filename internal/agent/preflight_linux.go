//go:build linux

package agent

import (
	"fmt"
	"os"
	"path/filepath"
)

func CheckCgroupNamespace(proc string) error {
	a, err := os.Readlink(filepath.Join(proc, "self", "ns", "cgroup"))
	if err != nil {
		return err
	}
	b, err := os.Readlink(filepath.Join(proc, "1", "ns", "cgroup"))
	if err != nil {
		return fmt.Errorf("inspect host init cgroup namespace (requires ptrace read permission, usually CAP_SYS_PTRACE in a restricted service): %w", err)
	}
	if a != b {
		return fmt.Errorf("private cgroup namespace %s differs from host init %s; use --cgroupns=host", a, b)
	}
	return nil
}
