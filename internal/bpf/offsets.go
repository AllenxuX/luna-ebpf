package bpf

import (
	"fmt"
	"github.com/cilium/ebpf/btf"
	"runtime"
	"strings"
)

// Every private kernel layout is obtained from the running kernel's BTF.
// A missing field fails startup; there are no version-based guessed offsets.
type socketTraceOffsets struct {
	SkRcu                                                         uint32
	TaskCgroups, CssDfl, CssSubsys, CssCgroup, CgroupKn, KernfsID uint32
	SkSocket, SkFamily, SocketFile, FileInode, InodeNumber        uint32
	SkbLen, SkbHead, SkbData, SkbNetworkHeader                    uint32
	Arg1, Arg2, Arg3, ControllerIndex, Unified                    uint32
}

func memberOffset(typ btf.Type, name string) (uint32, error) {
	var members []btf.Member
	switch t := btf.UnderlyingType(typ).(type) {
	case *btf.Struct:
		members = t.Members
	case *btf.Union:
		members = t.Members
	default:
		return 0, fmt.Errorf("BTF field %s is not in a struct/union", name)
	}
	for _, m := range members {
		if m.Name == name {
			if m.BitfieldSize != 0 || uint32(m.Offset)%8 != 0 {
				return 0, fmt.Errorf("BTF field %s is a bitfield", name)
			}
			return uint32(m.Offset) / 8, nil
		}
	}
	for _, m := range members {
		if m.Name != "" {
			continue
		}
		if offset, err := memberOffset(m.Type, name); err == nil {
			return uint32(m.Offset)/8 + offset, nil
		}
	}
	return 0, fmt.Errorf("BTF field %s missing or unsupported", name)
}

func probeParamIs(typ btf.Type, index int, want string) bool {
	proto, ok := btf.UnderlyingType(typ).(*btf.FuncProto)
	if !ok || index < 0 || index >= len(proto.Params) {
		return false
	}
	pointer, ok := btf.UnderlyingType(proto.Params[index].Type).(*btf.Pointer)
	if !ok {
		return false
	}
	structure, ok := btf.UnderlyingType(pointer.Target).(*btf.Struct)
	return ok && structure.Name == want
}

func validateProbeSignatures(spec *btf.Spec) error {
	probes := []struct {
		name string
		args []string
	}{
		{"sock_init_data", []string{"socket", "sock"}},
		{"security_sk_clone", []string{"sock", "sock"}},
		{"sk_destruct", []string{"sock"}},
		{"__sk_destruct", []string{"sock|rcu_head|callback_head"}},
		{"tcp_sendmsg", []string{"sock"}}, {"udp_sendmsg", []string{"sock"}}, {"udpv6_sendmsg", []string{"sock"}},
		{"ip_output", []string{"net", "sock", "sk_buff"}}, {"ip6_output", []string{"net", "sock", "sk_buff"}},
		{"tcp_v4_do_rcv", []string{"sock", "sk_buff"}}, {"tcp_v6_do_rcv", []string{"sock", "sk_buff"}},
		{"__udp_enqueue_schedule_skb", []string{"sock", "sk_buff"}},
	}
	for _, probe := range probes {
		var fn *btf.Func
		if err := spec.TypeByName(probe.name, &fn); err != nil {
			return fmt.Errorf("required kernel probe signature %s: %w", probe.name, err)
		}
		for i, want := range probe.args {
			matched := false
			for _, alternative := range strings.Split(want, "|") {
				if probeParamIs(fn.Type, i, alternative) {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("unsupported kernel signature %s argument %d (expected %s pointer)", probe.name, i+1, want)
			}
		}
	}
	return nil
}

func traceOffsets(spec *btf.Spec, controller string) (socketTraceOffsets, error) {
	var out socketTraceOffsets
	fields := []struct {
		target    *uint32
		typ, name string
	}{
		{&out.TaskCgroups, "task_struct", "cgroups"},
		{&out.CssCgroup, "cgroup_subsys_state", "cgroup"},
		{&out.CgroupKn, "cgroup", "kn"}, {&out.KernfsID, "kernfs_node", "id"},
		{&out.SkSocket, "sock", "sk_socket"}, {&out.SocketFile, "socket", "file"},
		{&out.SkRcu, "sock", "sk_rcu"},
		{&out.FileInode, "file", "f_inode"}, {&out.InodeNumber, "inode", "i_ino"},
		{&out.SkbLen, "sk_buff", "len"}, {&out.SkbHead, "sk_buff", "head"},
		{&out.SkbData, "sk_buff", "data"}, {&out.SkbNetworkHeader, "sk_buff", "network_header"},
	}
	if controller == "" {
		out.Unified = 1
		fields = append(fields, struct {
			target    *uint32
			typ, name string
		}{&out.CssDfl, "css_set", "dfl_cgrp"})
	} else {
		fields = append(fields, struct {
			target    *uint32
			typ, name string
		}{&out.CssSubsys, "css_set", "subsys"})
		var ids *btf.Enum
		if err := spec.TypeByName("cgroup_subsys_id", &ids); err != nil {
			return out, fmt.Errorf("kernel BTF cgroup_subsys_id: %w", err)
		}
		found := false
		for _, v := range ids.Values {
			if v.Name == controller+"_cgrp_id" {
				out.ControllerIndex = uint32(v.Value)
				found = true
				break
			}
		}
		if !found || out.ControllerIndex > 63 {
			return out, fmt.Errorf("BTF controller %q unavailable", controller)
		}
	}
	for _, f := range fields {
		var t *btf.Struct
		if err := spec.TypeByName(f.typ, &t); err != nil {
			return out, fmt.Errorf("kernel BTF %s: %w", f.typ, err)
		}
		offset, err := memberOffset(t, f.name)
		if err != nil {
			return out, fmt.Errorf("%s: %w", f.typ, err)
		}
		*f.target = offset
	}
	var sk, common, regs *btf.Struct
	if err := spec.TypeByName("sock", &sk); err != nil {
		return out, err
	}
	base, err := memberOffset(sk, "__sk_common")
	if err != nil {
		return out, err
	}
	if err = spec.TypeByName("sock_common", &common); err != nil {
		return out, err
	}
	family, err := memberOffset(common, "skc_family")
	if err != nil {
		return out, err
	}
	out.SkFamily = base + family
	if err = spec.TypeByName("pt_regs", &regs); err != nil {
		return out, err
	}
	if runtime.GOARCH == "amd64" {
		out.Arg1, err = memberOffset(regs, "di")
		if err != nil {
			return out, err
		}
		out.Arg2, err = memberOffset(regs, "si")
		if err != nil {
			return out, err
		}
		out.Arg3, err = memberOffset(regs, "dx")
		if err != nil {
			return out, err
		}
	} else if runtime.GOARCH == "arm64" {
		out.Arg1, err = memberOffset(regs, "regs")
		if err != nil {
			return out, err
		}
		out.Arg2 = out.Arg1 + 8
		out.Arg3 = out.Arg1 + 16
	} else {
		return out, fmt.Errorf("socket tracing architecture %s unsupported", runtime.GOARCH)
	}
	return out, nil
}
