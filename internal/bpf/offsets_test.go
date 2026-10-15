package bpf

import (
	"github.com/cilium/ebpf/btf"
	"testing"
)

func TestMemberOffsetResolvesAnonymousAndRejectsBitfields(t *testing.T) {
	u := &btf.Int{Name: "u64", Size: 8}
	nested := &btf.Struct{Size: 16, Members: []btf.Member{{Name: "field", Type: u, Offset: 64}}}
	outer := &btf.Struct{Size: 32, Members: []btf.Member{{Type: nested, Offset: 64}}}
	got, err := memberOffset(outer, "field")
	if err != nil || got != 16 {
		t.Fatalf("offset=%d err=%v", got, err)
	}
	if _, err = memberOffset(outer, "missing"); err == nil {
		t.Fatal("missing field accepted")
	}
	nested.Members[0].BitfieldSize = 1
	if _, err = memberOffset(outer, "field"); err == nil {
		t.Fatal("bitfield accepted")
	}
}

func TestProbePrototypeRejectsWrongSocketArgument(t *testing.T) {
	sk := &btf.Struct{Name: "sock", Size: 8}
	proto := &btf.FuncProto{Params: []btf.FuncParam{{Type: &btf.Pointer{Target: &btf.Const{Type: sk}}}}}
	if !probeParamIs(proto, 0, "sock") {
		t.Fatal("qualified socket pointer rejected")
	}
	if probeParamIs(proto, 1, "sock") {
		t.Fatal("missing argument accepted")
	}
	proto.Params[0].Type = &btf.Pointer{Target: &btf.Struct{Name: "rcu_head", Size: 8}}
	if probeParamIs(proto, 0, "sock") {
		t.Fatal("RCU callback argument accepted as socket")
	}
}
