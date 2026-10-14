package bpf

import (
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"testing"
)

// Parsing ELF on Windows checks shipped artifacts, not kernel verifier acceptance.
func TestObjectABI(t *testing.T) {
	s, err := ebpf.LoadCollectionSpec("traffic_bpfel.o")
	if err != nil {
		t.Fatal(err)
	}
	for name, kind := range map[string]ebpf.MapType{"traffic": ebpf.PerCPUHash, "monitored_cgroups": ebpf.Hash, "diagnostic_counters": ebpf.PerCPUArray} {
		m := s.Maps[name]
		if m == nil || m.Type != kind {
			t.Fatalf("map %s", name)
		}
	}
	if s.Maps["traffic"].KeySize != 16 || s.Maps["traffic"].ValueSize != 16 || s.Maps["traffic"].MaxEntries != 8192 || s.Maps["monitored_cgroups"].KeySize != 8 {
		t.Fatal("map ABI")
	}
	for name, attach := range map[string]ebpf.AttachType{"ingress": ebpf.AttachCGroupInetIngress, "egress": ebpf.AttachCGroupInetEgress} {
		p := s.Programs[name]
		if p == nil || p.Type != ebpf.CGroupSKB || p.AttachType != attach {
			t.Fatalf("program %s", name)
		}
		helper := false
		for i, v := range p.Instructions {
			if v.IsBuiltinCall() {
				if asm.BuiltinFunc(v.Constant) == asm.FnGetCurrentCgroupId {
					t.Fatal("process-based attribution")
				}
				if asm.BuiltinFunc(v.Constant) == asm.FnSkbCgroupId {
					helper = true
				}
			}
			if v.OpCode.JumpOp() == asm.Exit {
				if i == 0 || p.Instructions[i-1].OpCode != asm.Mov.Imm(asm.R0, 1).OpCode || p.Instructions[i-1].Constant != 1 {
					t.Fatalf("non-accept exit in %s", name)
				}
			}
		}
		if !helper {
			t.Fatal("missing socket helper")
		}
	}
}
