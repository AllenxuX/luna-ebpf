package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/AllenxuX/luna-ebpf/internal/bpf"
	"github.com/AllenxuX/luna-ebpf/internal/cgroup"
	"github.com/AllenxuX/luna-ebpf/internal/collector"
	"github.com/AllenxuX/luna-ebpf/internal/config"
	discovery "github.com/AllenxuX/luna-ebpf/internal/docker"
	"github.com/AllenxuX/luna-ebpf/internal/metrics"
	"github.com/moby/moby/client"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func Handler(s *collector.Store) http.Handler {
	registry := prometheus.NewRegistry()
	registry.MustRegister(metrics.New(s))
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		v := s.Load()
		if !v.Up || !v.DockerConnected || v.LastSuccess.IsZero() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ready\n"))
	})
	return mux
}
func Run(ctx context.Context, c config.Config) error {
	if runtime.GOOS != "linux" {
		return errors.New("Linux with host cgroup v1 or v2 is required; Windows supports portable unit tests only")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return fmt.Errorf("supported architectures: amd64/arm64; got %s", runtime.GOARCH)
	}
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cli, err := discovery.NewClient(callCtx, "unix://"+strings.TrimPrefix(c.DockerSocket, "unix://"))
	if err != nil {
		return fmt.Errorf("Docker client: %w", err)
	}
	defer cli.Close()
	_, err = cli.Ping(callCtx, client.PingOptions{})
	if err != nil {
		cancel()
		return fmt.Errorf("Docker ping: %w", err)
	}
	version, err := cli.ServerVersion(callCtx, client.ServerVersionOptions{})
	if err != nil {
		cancel()
		return fmt.Errorf("Docker version: %w", err)
	}
	system, err := cli.Info(callCtx, client.InfoOptions{})
	cancel()
	if err != nil {
		return fmt.Errorf("Docker info: %w", err)
	}
	mode := system.Info.CgroupVersion
	backend, err := selectBackend(mode, c.Backend)
	if err != nil {
		return err
	}
	root, controller := c.CgroupRoot, ""
	if mode == "1" {
		root, controller, err = cgroup.FindV1Root(root)
		if err != nil {
			return fmt.Errorf("cgroup v1 root: %w", err)
		}
	}
	if err = cgroup.CheckRootVersion(root, "v"+mode); err != nil {
		return err
	}
	if _, err = cgroup.NativeHandle(root); err != nil {
		return err
	}
	if err = CheckHostViewFor(c.ProcRoot, controller); err != nil {
		return err
	}
	mountinfo, err := os.ReadFile(filepath.Join(c.ProcRoot, "self", "mountinfo"))
	if err != nil {
		return err
	}
	if err = ValidateMountVersion(mountinfo, root, "v"+mode); err != nil {
		return err
	}
	slog.Info("preflight", "go", runtime.Version(), "arch", runtime.GOARCH, "docker_version", version.Version, "docker_api_version", cli.ClientVersion(), "cgroup_driver", system.Info.CgroupDriver, "cgroup_version", mode, "collector_backend", backend, "btf_required", backend == "socket-trace", "cgroup_controller", controller)
	d := &discovery.Discoverer{API: cli, Resolver: &cgroup.Resolver{Root: root, Proc: c.ProcRoot, MaxNodes: c.MaxNodes, Controller: controller}}
	callCtx, cancel = context.WithTimeout(ctx, 10*time.Second)
	containers, skipped, err := d.List(callCtx)
	cancel()
	if err != nil {
		return err
	}
	var maps *bpf.Loader
	if backend == "socket-trace" {
		maps, err = bpf.LoadSocketTrace(c.MapEntries, c.ProcRoot, controller)
	} else {
		maps, err = bpf.Load(c.MapEntries)
	}
	if err != nil {
		return err
	}
	defer maps.Close()
	store := &collector.Store{}
	engine := NewEngine(maps, store, c.Grace)
	engine.DiscoveryErrors = skipped
	engine.DockerConnected = true
	if err = engine.Reconcile(containers, time.Now()); err != nil {
		return err
	}
	if err = maps.SyncContainers(containers); err != nil {
		return fmt.Errorf("initialize socket ownership: %w", err)
	}
	if err = maps.Attach(root); err != nil {
		return err
	}
	engine.Attached = true
	if err = engine.Poll(time.Now()); err != nil {
		return err
	}
	if c.Check {
		slog.Info("preflight successful; selected backend loaded and attached, maps sampled; detaching")
		return nil
	}
	listener, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: Handler(store), ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	httpErr := make(chan error, 1)
	go func() { httpErr <- server.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	events := discovery.Watch(watchCtx, cli)
	poll := time.NewTicker(c.Poll)
	defer poll.Stop()
	reconcile := time.NewTicker(c.Reconcile)
	defer reconcile.Stop()
	eventsOK := true
	refresh := func() error {
		// Capture the last available counters before retiring old cgroups.
		if err := engine.Poll(time.Now()); err != nil {
			slog.Error("map poll failed", "op", "poll", "error", err)
		}
		callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		inventory, skipped, err := d.List(callCtx)
		engine.DiscoveryErrors += skipped
		if err != nil {
			engine.DiscoveryErrors++
			engine.DockerConnected = false
			engine.Publish()
			slog.Error("Docker reconciliation failed; retaining inventory", "op", "reconcile", "error", err)
			return nil
		}
		engine.DockerConnected = eventsOK
		if err = engine.Reconcile(inventory, time.Now()); err != nil {
			slog.Error("map reconciliation failed", "op", "reconcile_maps", "error", err)
			return nil
		}
		if err = maps.SyncContainers(inventory); err != nil {
			return fmt.Errorf("refresh socket ownership: %w", err)
		}
		return nil
	}
	slog.Info("exporter listening", "address", c.Listen)
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-httpErr:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return fmt.Errorf("HTTP serve: %w", err)
		case n, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if n.Disconnected {
				eventsOK = false
				engine.DockerConnected = false
				engine.Publish()
				continue
			}
			if n.Reconnected {
				eventsOK = true
				engine.EventReconnects++
			}
			if err := refresh(); err != nil {
				return err
			}
		case <-reconcile.C:
			if err := refresh(); err != nil {
				return err
			}
		case now := <-poll.C:
			if err := engine.Poll(now); err != nil {
				slog.Error("map poll failed", "op", "poll", "error", err)
			}
			if c.Raw {
				for k, v := range engine.previous {
					slog.Info("raw cgroup counter", "cgroup_id", k.CgroupID, "direction", k.Direction, "bytes", v.Bytes, "packets", v.Packets)
				}
			}
		}
	}
}
