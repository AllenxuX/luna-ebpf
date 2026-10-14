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
	// /proc/1 must be the host init, in the host root cgroup or init.scope.
	b, err := os.ReadFile(filepath.Join(proc, "1", "cgroup"))
	if err != nil {
		return fmt.Errorf("read host init cgroup: %w", err)
	}
	p, err := cgroup.Parse(string(b))
	if err != nil {
		return err
	}
	if p != "/" && p != "/init.scope" {
		return fmt.Errorf("proc_root PID 1 is in %s; use host PID namespace and host /proc", p)
	}
	return CheckCgroupNamespace(proc)
}

func ValidateMount(mountinfo []byte, root string) error {
	root = path.Clean(root)
	// Mountinfo shows the filesystem root before the mountpoint. A private
	// cgroup namespace presents '/' too, so namespace identity is also checked.
	found := false
	for _, line := range bytes.Split(mountinfo, []byte("\n")) {
		if bytes.Contains(line, []byte(" - cgroup2 ")) {
			parts := bytes.Fields(line)
			if len(parts) > 4 && string(parts[3]) == "/" && string(parts[4]) == root {
				found = true
			}
		}
	}
	if !found {
		return fmt.Errorf("host root cgroup2 mount is missing; bind host /sys/fs/cgroup and use cgroupns=host")
	}
	return nil
}
