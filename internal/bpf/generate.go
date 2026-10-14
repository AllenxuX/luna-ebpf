package bpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -tags linux -target bpfel -type traffic_key -type traffic_value -type diagnostics traffic traffic.bpf.c -- -O2 -g -Wall -Werror -Iinclude
