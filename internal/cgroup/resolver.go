package cgroup

import (
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

type Resolver struct {
	Root, Proc string
	Controller string
	MaxNodes   int
	Handle     func(string) ([]byte, error)
}

func ParseFor(contents, controller string) (string, error) {
	if controller == "" {
		return Parse(contents)
	}
	return ParseV1(contents, controller)
}

func ParseV1(contents, controller string) (string, error) {
	if controller == "" || strings.ContainsAny(controller, ",:\n/") {
		return "", fmt.Errorf("invalid cgroup v1 controller %q", controller)
	}
	var result string
	for _, line := range strings.Split(contents, "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		match := false
		for _, name := range strings.Split(parts[1], ",") {
			if name == controller {
				match = true
			}
		}
		if !match {
			continue
		}
		hierarchy, err := strconv.Atoi(parts[0])
		if err != nil || hierarchy <= 0 {
			return "", fmt.Errorf("invalid cgroup v1 hierarchy %q", parts[0])
		}
		if result != "" {
			return "", fmt.Errorf("multiple cgroup entries for %s", controller)
		}
		result, err = Parse("0::" + parts[2])
		if err != nil {
			return "", err
		}
	}
	if result == "" {
		return "", fmt.Errorf("missing cgroup v1 controller %s", controller)
	}
	return result, nil
}

func Parse(contents string) (string, error) {
	var result string
	for _, line := range strings.Split(contents, "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 || parts[0] != "0" || parts[1] != "" {
			continue
		}
		if result != "" {
			return "", fmt.Errorf("multiple unified cgroup entries")
		}
		if !strings.HasPrefix(parts[2], "/") || strings.ContainsRune(parts[2], 0) {
			return "", fmt.Errorf("invalid unified cgroup path %q", parts[2])
		}
		for _, p := range strings.Split(parts[2], "/") {
			if p == ".." {
				return "", fmt.Errorf("unsafe unified cgroup path %q", parts[2])
			}
		}
		result = path.Clean(parts[2])
	}
	if result == "" {
		return "", fmt.Errorf("missing 0::/path; cgroup v2 required")
	}
	return result, nil
}
func SafePath(root, p string) (string, error) {
	if _, err := Parse("0::" + p); err != nil {
		return "", err
	}
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	base, err = filepath.Abs(base)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(base, filepath.FromSlash(strings.TrimPrefix(p, "/"))))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(base, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cgroup path escapes root: %s", p)
	}
	return resolved, nil
}
func (r *Resolver) Resolve(pid int) (string, []uint64, error) {
	if pid <= 0 {
		return "", nil, fmt.Errorf("resolve unavailable PID %d", pid)
	}
	procFile := filepath.Join(r.Proc, strconv.Itoa(pid), "cgroup")
	b, err := os.ReadFile(procFile)
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", procFile, err)
	}
	p, err := ParseFor(string(b), r.Controller)
	if err != nil {
		return "", nil, err
	}
	if p == "/" {
		return "", nil, fmt.Errorf("PID %d shares the host root cgroup; a dedicated container cgroup is required", pid)
	}
	resolved, err := SafePath(r.Root, p)
	if err != nil {
		return "", nil, fmt.Errorf("resolve %s: %w", p, err)
	}
	handle := r.Handle
	if handle == nil {
		handle = NativeHandle
	}
	limit := r.MaxNodes
	if limit <= 0 {
		limit = 1024
	}
	var ids []uint64
	err = filepath.WalkDir(resolved, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if len(ids) >= limit {
			return fmt.Errorf("cgroup subtree exceeds max_nodes=%d at %s", limit, current)
		}
		bytes, err := handle(current)
		if err != nil {
			return fmt.Errorf("name_to_handle_at %s: %w", current, err)
		}
		if len(bytes) != 8 {
			return fmt.Errorf("cgroup handle %s has length %d; expected 8", current, len(bytes))
		}
		id := binary.NativeEndian.Uint64(bytes)
		if id == 0 {
			return fmt.Errorf("zero cgroup ID at %s", current)
		}
		ids = append(ids, id)
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	// Recheck proc after the filesystem scan, rejecting exit/PID moves during discovery.
	after, err := os.ReadFile(procFile)
	if err != nil {
		return "", nil, fmt.Errorf("recheck PID %d: %w", pid, err)
	}
	if string(after) != string(b) {
		return "", nil, fmt.Errorf("PID %d cgroup changed during discovery", pid)
	}
	return resolved, ids, nil
}
