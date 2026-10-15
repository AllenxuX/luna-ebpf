package config

import (
	"errors"
	"flag"
	"fmt"
	"go.yaml.in/yaml/v3"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

type Config struct {
	Backend      string        `yaml:"collector_backend"`
	Listen       string        `yaml:"listen_address"`
	DockerSocket string        `yaml:"docker_socket"`
	CgroupRoot   string        `yaml:"cgroup_root"`
	ProcRoot     string        `yaml:"proc_root"`
	Poll         time.Duration `yaml:"metrics_poll_interval"`
	Reconcile    time.Duration `yaml:"docker_reconcile_interval"`
	Grace        time.Duration `yaml:"container_stale_grace_period"`
	LogLevel     string        `yaml:"log_level"`
	MapEntries   uint32        `yaml:"map_max_entries"`
	MaxNodes     int           `yaml:"cgroup_subtree_max_nodes"`
	Check        bool          `yaml:"-"`
	Raw          bool          `yaml:"-"`
}

func Default() Config {
	return Config{Backend: "auto", Listen: "127.0.0.1:9808", DockerSocket: "/var/run/docker.sock", CgroupRoot: "/sys/fs/cgroup", ProcRoot: "/proc", Poll: 5 * time.Second, Reconcile: 30 * time.Second, Grace: 60 * time.Second, LogLevel: "info", MapEntries: 8192, MaxNodes: 1024}
}
func Parse(args []string) (Config, error) {
	c := Default()
	configFile := ""
	// Locate config without applying other flags, preserving CLI > YAML > default.
	for i, a := range args {
		if a == "--config" || a == "-config" {
			if i+1 == len(args) {
				return c, errors.New("--config needs a path")
			}
			configFile = args[i+1]
		}
		if strings.HasPrefix(a, "--config=") || strings.HasPrefix(a, "-config=") {
			configFile = strings.SplitN(a, "=", 2)[1]
		}
	}
	if configFile != "" {
		f, err := os.Open(configFile)
		if err != nil {
			return c, err
		}
		defer f.Close()
		decoder := yaml.NewDecoder(f)
		decoder.KnownFields(true)
		if err = decoder.Decode(&c); err != nil {
			return c, fmt.Errorf("config YAML: %w", err)
		}
		var extra any
		if err = decoder.Decode(&extra); err != io.EOF {
			return c, errors.New("config must contain exactly one YAML document")
		}
	}
	flags := flag.NewFlagSet("luna-ebpf", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&configFile, "config", configFile, "YAML file")
	flags.StringVar(&c.Listen, "listen-address", c.Listen, "HTTP listen address")
	flags.StringVar(&c.DockerSocket, "docker-socket", c.DockerSocket, "local Docker Engine Unix socket")
	flags.StringVar(&c.CgroupRoot, "cgroup-root", c.CgroupRoot, "host cgroup hierarchy root")
	flags.StringVar(&c.Backend, "collector-backend", c.Backend, "auto/cgroup-skb/socket-trace")
	flags.StringVar(&c.ProcRoot, "proc-root", c.ProcRoot, "host proc root")
	flags.DurationVar(&c.Poll, "metrics-poll-interval", c.Poll, "map sample interval")
	flags.DurationVar(&c.Reconcile, "docker-reconcile-interval", c.Reconcile, "inventory and child-cgroup scan interval")
	flags.DurationVar(&c.Grace, "container-stale-grace-period", c.Grace, "frozen metric retention after stop")
	flags.StringVar(&c.LogLevel, "log-level", c.LogLevel, "debug/info/warn/error")
	entries := uint(c.MapEntries)
	flags.UintVar(&entries, "map-max-entries", entries, "traffic and allowlist capacity")
	flags.IntVar(&c.MaxNodes, "cgroup-subtree-max-nodes", c.MaxNodes, "max directories per container scan")
	flags.BoolVar(&c.Check, "check", false, "perform discovery, real BPF load/attach, sample, then detach and exit")
	flags.BoolVar(&c.Raw, "raw-counters", false, "log sampled raw cgroup counters")
	if err := flags.Parse(args); err != nil {
		return c, err
	}
	if flags.NArg() != 0 {
		return c, errors.New("unexpected positional arguments")
	}
	if entries < 2 || entries > 1048576 {
		return c, errors.New("map-max-entries must be 2..1048576")
	}
	c.MapEntries = uint32(entries)
	if c.Poll <= 0 || c.Reconcile <= 0 || c.Grace < 0 || c.MaxNodes < 1 {
		return c, errors.New("intervals/max nodes must be positive; grace must be nonnegative")
	}
	if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		return c, fmt.Errorf("listen address: %w", err)
	}
	switch c.Backend {
	case "auto", "cgroup-skb", "socket-trace":
	default:
		return c, fmt.Errorf("invalid collector backend %q", c.Backend)
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return c, fmt.Errorf("invalid log level %q", c.LogLevel)
	}
	if !strings.HasPrefix(c.CgroupRoot, "/") || !strings.HasPrefix(c.ProcRoot, "/") || !strings.HasPrefix(c.DockerSocket, "/") {
		return c, errors.New("cgroup_root, proc_root and docker_socket must be absolute Linux paths")
	}
	return c, nil
}
