//go:build ignore

/* Native-endian map ABI: key 16 bytes (u64 id,u32 direction,u32 zero),
 * value 16 bytes (u64 bytes,u64 packets). RX=0, TX=1. All paths accept.
 * Only the stable __sk_buff.len UAPI prefix is accessed; no kernel BTF needed.
 */
#include "include/compat.h"
#include "include/bpf_helpers.h"

struct traffic_key { __u64 cgroup_id; __u32 direction; __u32 reserved; };
struct traffic_value { __u64 bytes; __u64 packets; };
struct diagnostics {
    __u64 map_insert_failures;
    __u64 lookup_failures;
    __u64 zero_rx_bytes;
    __u64 zero_rx_packets;
    __u64 zero_tx_bytes;
    __u64 zero_tx_packets;
};
struct { __uint(type, BPF_MAP_TYPE_PERCPU_HASH); __uint(max_entries,8192);
    __type(key,struct traffic_key); __type(value,struct traffic_value);
} traffic SEC(".maps");
struct { __uint(type,BPF_MAP_TYPE_HASH); __uint(max_entries,8192);
    __type(key,__u64); __type(value,__u32);
} monitored_cgroups SEC(".maps");
struct { __uint(type,BPF_MAP_TYPE_PERCPU_ARRAY); __uint(max_entries,1);
    __type(key,__u32); __type(value,struct diagnostics);
} diagnostic_counters SEC(".maps");

static __always_inline int account(struct __sk_buff *skb, __u32 direction) {
    __u32 index=0;
    struct diagnostics *d=bpf_map_lookup_elem(&diagnostic_counters,&index);
    __u64 id=bpf_skb_cgroup_id(skb);
    if (!id) {
        if (d) {
            if (!direction) {d->zero_rx_bytes+=skb->len; d->zero_rx_packets++;}
            else {d->zero_tx_bytes+=skb->len; d->zero_tx_packets++;}
        }
        return 1;
    }
    if (!bpf_map_lookup_elem(&monitored_cgroups,&id)) return 1;
    struct traffic_key key={.cgroup_id=id,.direction=direction,.reserved=0};
    struct traffic_value zero={};
    struct traffic_value *v=bpf_map_lookup_elem(&traffic,&key);
    if (!v) {
        long err=bpf_map_update_elem(&traffic,&key,&zero,BPF_NOEXIST);
        // -EEXIST is a normal concurrent first-insert race.
        if (err && err!=-17 && d) d->map_insert_failures++;
        v=bpf_map_lookup_elem(&traffic,&key);
        if (!v) {if(d) d->lookup_failures++; return 1;}
    }
    v->bytes+=skb->len;
    v->packets++;
    return 1;
}
SEC("cgroup_skb/ingress") int ingress(struct __sk_buff *skb) {return account(skb,0);}
SEC("cgroup_skb/egress") int egress(struct __sk_buff *skb) {return account(skb,1);}
char LICENSE[] SEC("license")="GPL";
