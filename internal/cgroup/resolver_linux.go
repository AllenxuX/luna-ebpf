//go:build linux

package cgroup

import (
	"fmt"
	"golang.org/x/sys/unix"
)

func CheckRoot(root string) error {
	return CheckRootVersion(root, "v2")
}
func CheckRootVersion(root, version string) error {
	magic := uint64(unix.CGROUP2_SUPER_MAGIC)
	if version == "v1" {
		magic = uint64(unix.CGROUP_SUPER_MAGIC)
	} else if version != "v2" {
		return fmt.Errorf("unsupported cgroup version %q", version)
	}
	var st unix.Statfs_t
	if err := unix.Statfs(root, &st); err != nil {
		return fmt.Errorf("statfs cgroup root: %w", err)
	}
	if uint64(st.Type) != magic {
		return fmt.Errorf("%s is not cgroup %s filesystem (type=%x)", root, version, st.Type)
	}
	return nil
}
func NativeHandle(p string) ([]byte, error) {
	if err := CheckRoot(p); err != nil {
		if err := CheckRootVersion(p, "v1"); err != nil {
			return nil, err
		}
	}
	handle, _, err := unix.NameToHandleAt(unix.AT_FDCWD, p, 0)
	if err != nil {
		return nil, fmt.Errorf("kernel cgroup handle (check permissions and host cgroup mount): %w", err)
	}
	return handle.Bytes(), nil
}
