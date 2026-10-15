package agent

import (
	"bytes"
	"fmt"
	"github.com/AllenxuX/luna-ebpf/internal/cgroup"
	"os"
	"path"
	"path/filepath"
)

func CheckHostView(proc string) error {
	return CheckHostViewFor(proc, "")
}
func CheckHostViewFor(proc, controller string) error {
	// /proc/1 must be the host init, in the host root cgroup or init.scope.
	b, err := os.ReadFile(filepath.Join(proc, "1", "cgroup"))
	if err != nil {
		return fmt.Errorf("read host init cgroup: %w", err)
	}
	p, err := cgroup.ParseFor(string(b), controller)
	if err != nil {
		return err
	}
	if p != "/" && p != "/init.scope" {
		return fmt.Errorf("proc_root PID 1 is in %s; use host PID namespace and host /proc", p)
	}
	return CheckCgroupNamespace(proc)
}

func ValidateMount(mountinfo []byte, root string) error {
	return ValidateMountVersion(mountinfo, root, "v2")
}
func ValidateMountVersion(mountinfo []byte, root, version string) error {
	fsType := "cgroup2"
	if version == "v1" {
		fsType = "cgroup"
	} else if version != "v2" {
		return fmt.Errorf("unsupported cgroup version %q", version)
	}
	root = path.Clean(root)
	// Mountinfo shows the filesystem root before the mountpoint. A private
	// cgroup namespace presents '/' too, so namespace identity is also checked.
	found := false
	for _, line := range bytes.Split(mountinfo, []byte("\n")) {
		if bytes.Contains(line, []byte(" - "+fsType+" ")) {
			parts := bytes.Fields(line)
			if len(parts) > 4 && string(parts[3]) == "/" && string(parts[4]) == root {
				found = true
			}
		}
	}
	if !found {
		return fmt.Errorf("host root %s mount is missing; bind host /sys/fs/cgroup and use cgroupns=host", fsType)
	}
	return nil
}
