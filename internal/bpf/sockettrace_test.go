package bpf

import (
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"testing"
)

func TestSocketTraceObjectUsesSocketOwnershipForReceive(t *testing.T) {
	spec, err := ebpf.LoadCollectionSpec("sockettrace_bpfel.o")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"traffic", "monitored_cgroups", "diagnostic_counters", "socket_owners", "inode_owners"} {
		if spec.Maps[name] == nil {
			t.Fatalf("missing map %s", name)
		}
	}
	if spec.Maps["traffic"].KeySize != 16 || spec.Maps["traffic"].ValueSize != 16 || spec.Maps["traffic"].Type != ebpf.PerCPUHash {
		t.Fatal("shared traffic ABI changed")
	}
	for _, name := range []string{"trace_tcp4", "trace_tcp6", "trace_udp"} {
		p := spec.Programs[name]
		if p == nil || p.Type != ebpf.Kprobe {
			t.Fatalf("missing receive probe %s", name)
		}
		lookup := false
		for _, ins := range p.Instructions {
			if !ins.IsBuiltinCall() {
				continue
			}
			switch asm.BuiltinFunc(ins.Constant) {
			case asm.FnGetCurrentTask, asm.FnGetCurrentCgroupId, asm.FnGetCurrentPidTgid:
				t.Fatalf("RX attributed via current task in %s", name)
			case asm.FnMapLookupElem:
				lookup = true
			case asm.FnProbeWriteUser, asm.FnSkbStoreBytes:
				t.Fatalf("packet/memory mutation in %s", name)
			}
		}
		if !lookup {
			t.Fatalf("missing socket owner lookup in %s", name)
		}
	}
}
