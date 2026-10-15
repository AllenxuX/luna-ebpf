//go:build ignore

/* Socket ownership is captured in process context, then used in RX softirq
 * context. No RX ownership is ever inferred from current PID/task.
 * Offsets are resolved from the actual running kernel's BTF by Go.
 */
#include "include/compat.h"
#include "include/bpf_helpers.h"

struct traffic_key { __u64 cgroup_id; __u32 direction; __u32 reserved; };
struct traffic_value { __u64 bytes; __u64 packets; };
struct diagnostics { __u64 map_insert_failures,lookup_failures,zero_rx_bytes,zero_rx_packets,zero_tx_bytes,zero_tx_packets; };
struct offsets {
 __u32 sk_rcu;
 __u32 task_cgroups,css_dfl,css_subsys,css_cgroup,cgroup_kn,kernfs_id;
 __u32 sk_socket,sk_family,socket_file,file_inode,inode_number;
 __u32 skb_len,skb_head,skb_data,skb_network_header;
 __u32 arg1,arg2,arg3,controller_index,unified;
};
const volatile struct offsets settings={};
struct { __uint(type,BPF_MAP_TYPE_PERCPU_HASH); __uint(max_entries,8192); __type(key,struct traffic_key); __type(value,struct traffic_value); } traffic SEC(".maps");
struct { __uint(type,BPF_MAP_TYPE_HASH); __uint(max_entries,8192); __type(key,__u64); __type(value,__u32); } monitored_cgroups SEC(".maps");
struct { __uint(type,BPF_MAP_TYPE_PERCPU_ARRAY); __uint(max_entries,1); __type(key,__u32); __type(value,struct diagnostics); } diagnostic_counters SEC(".maps");
struct { __uint(type,BPF_MAP_TYPE_HASH); __uint(max_entries,65536); __type(key,__u64); __type(value,__u64); } socket_owners SEC(".maps");
struct { __uint(type,BPF_MAP_TYPE_HASH); __uint(max_entries,65536); __type(key,__u64); __type(value,__u64); } inode_owners SEC(".maps");

static __always_inline __u64 read64(__u64 ptr,__u32 off) {
 __u64 v=0;if(ptr)bpf_probe_read_kernel(&v,sizeof(v),(void *)(ptr+off));return v;
}
static __always_inline __u64 arg(void *ctx,__u32 off) {return read64((__u64)ctx,off);}
static __always_inline struct diagnostics *diag(void) {__u32 z=0;return bpf_map_lookup_elem(&diagnostic_counters,&z);}
static __always_inline void remember(__u64 sk,__u64 id) {
 if(!sk || !id)return;
 if(bpf_map_update_elem(&socket_owners,&sk,&id,0)) {struct diagnostics *d=diag();if(d)d->map_insert_failures++;}
}
static __always_inline __u64 owner(__u64 sk) {
 if(!sk)return 0;
 __u64 *known=bpf_map_lookup_elem(&socket_owners,&sk);if(known)return *known;
 __u64 socket=read64(sk,settings.sk_socket);
 __u64 file=read64(socket,settings.socket_file);
 __u64 inode=read64(file,settings.file_inode);
 __u64 number=read64(inode,settings.inode_number);
 if(!number)return 0;
 known=bpf_map_lookup_elem(&inode_owners,&number);if(!known)return 0;
 __u64 id=*known;remember(sk,id);return id;
}
static __always_inline __u64 task_cgroup(void) {
 __u64 task=(__u64)bpf_get_current_task();
 __u64 css=read64(task,settings.task_cgroups),cg=0;
 if(settings.unified)cg=read64(css,settings.css_dfl);
 else {__u64 sub=read64(css,settings.css_subsys+settings.controller_index*8);cg=read64(sub,settings.css_cgroup);}
 return read64(read64(cg,settings.cgroup_kn),settings.kernfs_id);
}
static __always_inline void capture(__u64 sk) {
 __u16 family=0;
 if(!sk)return;
 bpf_probe_read_kernel(&family,sizeof(family),(void *)(sk+settings.sk_family));
 if(family!=2 && family!=10)return;
 // Tracking unmonitored sockets too prevents stale inode snapshots from
 // attributing a newly-created host socket to a previously retired inode.
 remember(sk,task_cgroup());
}
static __always_inline int account(__u64 sk,__u64 skb,__u32 direction,__u8 version) {
 if(!sk || !skb)return 0;
 __u32 len=0;__u16 network=0;
 bpf_probe_read_kernel(&len,sizeof(len),(void *)(skb+settings.skb_len));
 bpf_probe_read_kernel(&network,sizeof(network),(void *)(skb+settings.skb_network_header));
 __u64 head=read64(skb,settings.skb_head),data=read64(skb,settings.skb_data);
 // The v6 TCP backlog path delegates mapped IPv4 to tcp_v4_do_rcv.
 // Count only at the probe matching the packet's actual IP version.
 if(version) {__u8 first=0;bpf_probe_read_kernel(&first,1,(void *)(head+network));if((first>>4)!=version)return 0;}
 // TCP/UDP input has pulled headers; restore network-layer skb length.
 if(data>=head+network && data-head-network<65536)len+=(__u32)(data-head-network);
 struct diagnostics *d=diag();__u64 id=owner(sk);
 if(!id) {if(d) {if(direction){d->zero_tx_bytes+=len;d->zero_tx_packets++;}else{d->zero_rx_bytes+=len;d->zero_rx_packets++;}}return 0;}
 if(!bpf_map_lookup_elem(&monitored_cgroups,&id))return 0;
 struct traffic_key key={.cgroup_id=id,.direction=direction};
 struct traffic_value zero={},*v=bpf_map_lookup_elem(&traffic,&key);
 if(!v) {long err=bpf_map_update_elem(&traffic,&key,&zero,BPF_NOEXIST);if(err && err!=-17 && d)d->map_insert_failures++;v=bpf_map_lookup_elem(&traffic,&key);}
 if(!v){if(d)d->lookup_failures++;return 0;}v->bytes+=len;v->packets++;return 0;
}
SEC("kprobe") int trace_create(void *ctx) {capture(arg(ctx,settings.arg2));return 0;}
SEC("kprobe") int trace_clone(void *ctx) {__u64 id=owner(arg(ctx,settings.arg1));remember(arg(ctx,settings.arg2),id);return 0;}
SEC("kprobe") int trace_destroy(void *ctx) {__u64 sk=arg(ctx,settings.arg1);bpf_map_delete_elem(&socket_owners,&sk);return 0;}
SEC("kprobe") int trace_destroy_rcu(void *ctx) {__u64 sk=arg(ctx,settings.arg1)-settings.sk_rcu;bpf_map_delete_elem(&socket_owners,&sk);return 0;}
SEC("kprobe") int trace_sendmsg(void *ctx) {__u64 sk=arg(ctx,settings.arg1);if(!owner(sk))capture(sk);return 0;}
SEC("kprobe") int trace_ip4(void *ctx) {return account(arg(ctx,settings.arg2),arg(ctx,settings.arg3),1,4);}
SEC("kprobe") int trace_ip6(void *ctx) {return account(arg(ctx,settings.arg2),arg(ctx,settings.arg3),1,6);}
SEC("kprobe") int trace_tcp4(void *ctx) {return account(arg(ctx,settings.arg1),arg(ctx,settings.arg2),0,4);}
SEC("kprobe") int trace_tcp6(void *ctx) {return account(arg(ctx,settings.arg1),arg(ctx,settings.arg2),0,6);}
SEC("kprobe") int trace_udp(void *ctx) {return account(arg(ctx,settings.arg1),arg(ctx,settings.arg2),0,0);}
char LICENSE[] SEC("license")="GPL";
