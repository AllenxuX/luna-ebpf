//go:build linux

package bpf

import (
	"github.com/AllenxuX/luna-ebpf/internal/collector"
	"os"
	"path/filepath"
	"testing"
)

func TestSocketInventoryChecksMembershipAndAmbiguity(t *testing.T) {
	root := t.TempDir()
	proc := filepath.Join(root, "proc")
	write := func(path, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	add := func(name, pid string, id uint64) collector.ContainerInfo {
		t.Helper()
		cg := filepath.Join(root, "cgroup", name)
		write(filepath.Join(cg, "cgroup.procs"), pid+"\n")
		write(filepath.Join(proc, pid, "cgroup"), "4:memory:/"+name+"\n")
		if err := os.MkdirAll(filepath.Join(proc, pid, "fd"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("socket:[101]", filepath.Join(proc, pid, "fd", "3")); err != nil {
			t.Fatal(err)
		}
		return collector.ContainerInfo{ID: name, PID: 123, CgroupPath: cg, CgroupID: id}
	}
	one := add("one", "123", 11)
	got, err := socketInventory(proc, "memory", []collector.ContainerInfo{one}, 8)
	if err != nil || got[101] != 11 {
		t.Fatalf("%v %v", got, err)
	}
	// PID was listed in cgroup.procs, but migrated before its FD scan.
	write(filepath.Join(proc, "123", "cgroup"), "4:memory:/other\n")
	if _, err = socketInventory(proc, "memory", []collector.ContainerInfo{one}, 8); err == nil {
		t.Fatal("main PID migration accepted")
	}
	write(filepath.Join(proc, "123", "cgroup"), "4:memory:/one\n")
	two := add("two", "124", 22)
	two.PID = 124
	if _, err = socketInventory(proc, "memory", []collector.ContainerInfo{one, two}, 8); err == nil {
		t.Fatal("shared inode ambiguously seeded")
	}
	if _, err = socketInventory(proc, "memory", []collector.ContainerInfo{one}, 0); err == nil {
		t.Fatal("unbounded inventory accepted")
	}
	child := add("one/child", "125", 33)
	child.PID = 125
	childFD := filepath.Join(proc, "125", "fd", "3")
	if err = os.Remove(childFD); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink("socket:[102]", childFD); err != nil {
		t.Fatal(err)
	}
	got, err = socketInventory(proc, "memory", []collector.ContainerInfo{one, child}, 16)
	if err != nil || got[101] != 11 || got[102] != 33 {
		t.Fatalf("nearest Docker subtree ownership: %v %v", got, err)
	}
}
