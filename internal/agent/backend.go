package agent

import "fmt"

// selectBackend uses the Docker daemon's cgroup hierarchy version.
func selectBackend(version, requested string) (string, error) {
	if version != "1" && version != "2" {
		return "", fmt.Errorf("Docker cgroup version=%q; supported versions are 1 and 2", version)
	}
	switch requested {
	case "auto":
		if version == "1" {
			return "socket-trace", nil
		}
		return "cgroup-skb", nil
	case "socket-trace":
		return requested, nil
	case "cgroup-skb":
		if version == "1" {
			return "", fmt.Errorf("collector backend cgroup-skb requires cgroup v2; use auto or socket-trace for cgroup v1")
		}
		return requested, nil
	default:
		return "", fmt.Errorf("invalid collector backend %q", requested)
	}
}
