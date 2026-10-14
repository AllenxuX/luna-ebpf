//go:build linux

package cgroup

import (
	"fmt"
	"golang.org/x/sys/unix"
)

func CheckRoot(root string) error {
	var st unix.Statfs_t
	if err := unix.Statfs(root, &st); err != nil {
		return fmt.Errorf("statfs cgroup root: %w", err)
	}
	if uint64(st.Type) != uint64(unix.CGROUP2_SUPER_MAGIC) {
		return fmt.Errorf("%s is not cgroup2fs (type=%x)", root, st.Type)
	}
	return nil
}
func NativeHandle(p string) ([]byte, error) {
	if err := CheckRoot(p); err != nil {
		return nil, err
	}
	handle, _, err := unix.NameToHandleAt(unix.AT_FDCWD, p, 0)
	if err != nil {
		return nil, fmt.Errorf("kernel cgroup handle (check permissions and host cgroup mount): %w", err)
	}
	return handle.Bytes(), nil
}
