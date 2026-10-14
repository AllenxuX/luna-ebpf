package docker

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"github.com/AllenxuX/luna-ebpf/internal/collector"
)

type API interface {
	ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error)
}
type Resolver interface {
	Resolve(int) (string, []uint64, error)
}
type Discoverer struct {
	API      API
	Resolver Resolver
	previous map[string]collector.ContainerInfo
}

func (d *Discoverer) List(ctx context.Context) ([]collector.ContainerInfo, uint64, error) {
	list, err := d.API.ContainerList(ctx, client.ContainerListOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("Docker ContainerList: %w", err)
	}
	out := make([]collector.ContainerInfo, 0)
	var skipped uint64
	for _, summary := range list.Items {
		result, err := d.API.ContainerInspect(ctx, summary.ID, client.ContainerInspectOptions{})
		if err != nil {
			// A daemon/transport failure invalidates the whole scan, preserving the old view.
			if !errdefs.IsNotFound(err) {
				return nil, skipped, fmt.Errorf("Docker ContainerInspect %s: %w", summary.ID, err)
			}
			skipped++
			slog.Warn("container disappeared during discovery", "op", "inspect", "container_id", summary.ID, "error", err)
			continue
		}
		v := result.Container
		if v.HostConfig == nil || v.State == nil || !v.State.Running {
			continue
		}
		p, ids, err := d.Resolver.Resolve(v.State.Pid)
		if err == nil && len(ids) == 0 {
			err = fmt.Errorf("cgroup resolver returned no IDs")
		}
		if err != nil {
			skipped++
			slog.Warn("cgroup discovery failed", "op", "resolve_cgroup", "container_id", v.ID, "error", err)
			// Retain only previously verified IDs for this confirmed-running
			// container. Do not guess a new cgroup during a restart or scan failure.
			if known, ok := d.previous[v.ID]; ok {
				known.Name = strings.TrimPrefix(v.Name, "/")
				out = append(out, known)
				slog.Warn("retaining last verified cgroup mapping pending retry", "op", "resolve_cgroup", "container_id", v.ID, "cgroup_path", known.CgroupPath)
			}
			continue
		}
		// PID exit/reuse before the first /proc read cannot be detected by
		// re-reading /proc alone; revalidate Docker identity after the scan.
		confirmed, err := d.API.ContainerInspect(ctx, v.ID, client.ContainerInspectOptions{})
		if err != nil {
			if !errdefs.IsNotFound(err) {
				return nil, skipped, fmt.Errorf("Docker re-inspect %s: %w", v.ID, err)
			}
			skipped++
			continue
		}
		fresh := confirmed.Container
		if fresh.State == nil || !fresh.State.Running || fresh.HostConfig == nil {
			skipped++
			continue
		}
		if fresh.State.Pid != v.State.Pid || fresh.State.StartedAt != v.State.StartedAt || fresh.ID != v.ID {
			skipped++
			slog.Warn("container changed during cgroup scan", "op", "revalidate_identity", "container_id", v.ID)
			if known, ok := d.previous[v.ID]; ok {
				out = append(out, known)
			}
			continue
		}
		v = fresh
		started, _ := time.Parse(time.RFC3339Nano, v.State.StartedAt)
		created, _ := time.Parse(time.RFC3339Nano, v.Created)
		out = append(out, collector.ContainerInfo{ID: v.ID, Name: strings.TrimPrefix(v.Name, "/"), NetworkMode: string(v.HostConfig.NetworkMode), PID: v.State.Pid, CgroupPath: p, CgroupID: ids[0], CgroupIDs: ids, StartedAt: started, CreatedAt: created})
	}
	d.previous = make(map[string]collector.ContainerInfo, len(out))
	for _, v := range out {
		d.previous[v.ID] = v
	}
	return out, skipped, nil
}
