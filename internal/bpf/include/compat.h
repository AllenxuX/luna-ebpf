/* Minimal Linux UAPI definitions used with the vendored libbpf helpers.
 * Numbers and __sk_buff.len prefix match include/uapi/linux/bpf.h.
 */
#ifndef LUNA_EBPF_COMPAT_H
#define LUNA_EBPF_COMPAT_H
typedef unsigned char __u8;
typedef signed char __s8;
typedef unsigned short __u16;
typedef short __s16;
typedef unsigned int __u32;
typedef int __s32;
typedef unsigned long long __u64;
typedef long long __s64;
typedef __u16 __be16;
typedef __u32 __be32;
typedef __u32 __wsum;
struct __sk_buff { __u32 len; };
#define BPF_MAP_TYPE_HASH 1
#define BPF_MAP_TYPE_PERCPU_HASH 5
#define BPF_MAP_TYPE_PERCPU_ARRAY 6
#define BPF_NOEXIST 1
#endif
