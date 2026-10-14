package cgroup

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for _, v := range []struct {
		in, out string
		ok      bool
	}{{"0::/docker/x\n", "/docker/x", true}, {"2:cpu:/x\n", "", false}, {"0::/../../escape\n", "", false}, {"0::relative\n", "", false}, {"0::/\n", "/", true}, {"0::/a\n0::/b\n", "", false}} {
		got, err := Parse(v.in)
		if (err == nil) != v.ok || got != v.out {
			t.Fatalf("%q => %q %v", v.in, got, err)
		}
	}
}
func TestResolver(t *testing.T) {
	root := t.TempDir()
	proc := t.TempDir()
	os.MkdirAll(filepath.Join(root, "docker", "child"), 0755)
	os.MkdirAll(filepath.Join(proc, "42"), 0755)
	os.WriteFile(filepath.Join(proc, "42", "cgroup"), []byte("0::/docker\n"), 0644)
	handle := func(p string) ([]byte, error) {
		b := make([]byte, 8)
		binary.NativeEndian.PutUint64(b, 42)
		if strings.HasSuffix(p, "child") {
			binary.NativeEndian.PutUint64(b, 43)
		}
		return b, nil
	}
	r := Resolver{Root: root, Proc: proc, MaxNodes: 8, Handle: handle}
	path, ids, err := r.Resolve(42)
	if err != nil || path != filepath.Join(root, "docker") || len(ids) != 2 || ids[0] != 42 {
		t.Fatalf("%s %v %v", path, ids, err)
	}
	if _, _, err = r.Resolve(0); err == nil {
		t.Fatal("PID zero")
	}
	if _, _, err = r.Resolve(404); err == nil {
		t.Fatal("missing PID")
	}
	r.Handle = func(string) ([]byte, error) { return []byte{1}, nil }
	if _, _, err = r.Resolve(42); err == nil {
		t.Fatal("bad handle")
	}
	r.Handle = func(string) ([]byte, error) { return nil, os.ErrPermission }
	if _, _, err = r.Resolve(42); !errors.Is(err, os.ErrPermission) {
		t.Fatal(err)
	}
	r.Handle = handle
	r.MaxNodes = 1
	if _, _, err = r.Resolve(42); err == nil {
		t.Fatal("unbounded subtree")
	}
	r.MaxNodes = 8
	os.WriteFile(filepath.Join(proc, "42", "cgroup"), []byte("0::/\n"), 0644)
	if _, _, err = r.Resolve(42); err == nil {
		t.Fatal("shared host root must not be attributed to a container")
	}
}
func TestSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skip(err)
	}
	if _, err := SafePath(root, "/escape"); err == nil {
		t.Fatal("escape allowed")
	}
}
