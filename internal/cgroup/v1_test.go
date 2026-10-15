package cgroup

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestV1ResolverAndPIDMove(t *testing.T) {
	root, proc := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docker", "child"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proc, "42"), 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(proc, "42", "cgroup")
	write := func(contents string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("5:memory:/docker\n6:cpu:/different\n")
	r := Resolver{Root: root, Proc: proc, Controller: "memory", MaxNodes: 4, Handle: func(string) ([]byte, error) {
		b := make([]byte, 8)
		binary.NativeEndian.PutUint64(b, 42)
		return b, nil
	}}
	p, ids, err := r.Resolve(42)
	if err != nil || p != filepath.Join(root, "docker") || len(ids) != 2 {
		t.Fatalf("%q %v %v", p, ids, err)
	}
	r.Handle = func(string) ([]byte, error) {
		write("5:memory:/moved\n")
		b := make([]byte, 8)
		binary.NativeEndian.PutUint64(b, 42)
		return b, nil
	}
	if _, _, err = r.Resolve(42); err == nil {
		t.Fatal("accepted cgroup migration during scan")
	}
	write("5:memory:/\n")
	if _, _, err = r.Resolve(42); err == nil {
		t.Fatal("accepted host root")
	}
	write("5:memory:/../../escape\n")
	if _, _, err = r.Resolve(42); err == nil {
		t.Fatal("accepted path escape")
	}
}

func TestFindV1Root(t *testing.T) {
	mounts := "1 0 0:1 / /cg/cpu rw - cgroup cgroup rw,cpu,cpuacct\n2 0 0:2 / /cg/memory rw - cgroup cgroup rw,memory\n"
	check := func(string) error { return nil }
	root, controller, err := findV1Root("/cg", mounts, check)
	if err != nil || root != filepath.Clean("/cg/memory") || controller != "memory" {
		t.Fatalf("%q %q %v", root, controller, err)
	}
	root, controller, err = findV1Root("/cg/cpu", mounts, check)
	if err != nil || root != filepath.Clean("/cg/cpu") || controller != "cpu" {
		t.Fatalf("%q %q %v", root, controller, err)
	}
	if _, _, err = findV1Root("/cg", "1 0 0:1 /docker /cg/memory rw - cgroup cgroup rw,memory\n", check); err == nil {
		t.Fatal("accepted subtree mount")
	}
	if _, _, err = findV1Root("/cg", "1 0 0:1 / /cg-other/memory rw - cgroup cgroup rw,memory\n", check); err == nil {
		t.Fatal("accepted mount outside root")
	}
}

func TestParseV1(t *testing.T) {
	for _, tt := range []struct {
		in, controller, want string
		ok                   bool
	}{
		{"4:cpu,cpuacct:/docker/a\n7:memory:/docker/b\n", "memory", "/docker/b", true},
		{"4:cpu,cpuacct:/docker/a\n", "cpu", "/docker/a", true},
		{"4:cpuacct:/docker/a\n", "cpu", "", false},
		{"0::/docker/a\n", "memory", "", false},
		{"7:memory:/../../escape\n", "memory", "", false},
		{"7:memory:relative\n", "memory", "", false},
		{"7:memory:/a\n8:memory:/b\n", "memory", "", false},
		{"7:memory:/\n", "memory", "/", true},
		{"invalid:memory:/a\n", "memory", "", false},
	} {
		got, err := ParseV1(tt.in, tt.controller)
		if got != tt.want || (err == nil) != tt.ok {
			t.Fatalf("%q: %q %v", tt.in, got, err)
		}
	}
}
