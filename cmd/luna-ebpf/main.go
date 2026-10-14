package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/AllenxuX/luna-ebpf/internal/agent"
	"github.com/AllenxuX/luna-ebpf/internal/config"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	c, err := config.Parse(os.Args[1:])
	if err != nil {
		if err == flag.ErrHelp {
			fmt.Println("luna-ebpf: Linux Docker host-network socket traffic exporter\nUse --config configs/config.example.yaml; see README for all flags.\n--check runs discovery, BPF load/attach/sample then exits.\n--raw-counters logs raw cgroup counters.")
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var level slog.Level
	_ = level.UnmarshalText([]byte(c.LogLevel))
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err = agent.Run(ctx, c); err != nil {
		slog.Error("agent failed", "op", "run", "error", err)
		os.Exit(1)
	}
}
