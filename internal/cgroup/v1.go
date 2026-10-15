package cgroup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FindV1Root selects a mounted controller hierarchy. The memory hierarchy is
// preferred because it is normally enabled for every Docker container.
func FindV1Root(root string) (string, string, error) {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return "", "", fmt.Errorf("read cgroup mounts: %w", err)
	}
	return findV1Root(root, string(data), func(p string) error { return CheckRootVersion(p, "v1") })
}

func findV1Root(root, mountinfo string, check func(string) error) (string, string, error) {
	root = filepath.Clean(root)
	type candidate struct{ root, controller string }
	var candidates []candidate
	for _, controller := range []string{"memory", "pids", "cpuset", "cpu", "cpuacct", "blkio", "devices", "freezer", "net_cls", "net_prio", "hugetlb", "perf_event"} {
		for _, line := range strings.Split(mountinfo, "\n") {
			halves := strings.SplitN(line, " - ", 2)
			if len(halves) != 2 {
				continue
			}
			before, after := strings.Fields(halves[0]), strings.Fields(halves[1])
			if len(before) < 5 || len(after) < 3 || after[0] != "cgroup" || before[3] != "/" {
				continue
			}
			mount := filepath.Clean(unescapeMount(before[4]))
			rel, err := filepath.Rel(root, mount)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			for _, option := range strings.Split(after[2], ",") {
				if option == controller {
					candidates = append(candidates, candidate{mount, controller})
				}
			}
		}
	}
	for _, candidate := range candidates {
		if check(candidate.root) == nil {
			return candidate.root, candidate.controller, nil
		}
	}
	return "", "", fmt.Errorf("no full-root mounted cgroup v1 controller under %s", root)
}

func unescapeMount(p string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(p)
}
