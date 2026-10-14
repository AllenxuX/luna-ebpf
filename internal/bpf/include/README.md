# Header provenance

`bpf_helpers.h` and `bpf_helper_defs.h` are unmodified libbpf v1.6.2 files from
https://github.com/libbpf/libbpf/tree/v1.6.2/src. They use the BSD-2-Clause OR
LGPL-2.1 license choice; LICENSE.BSD-2-Clause is included here.

`compat.h` declares only the Linux UAPI primitives/map constants and the stable
`__sk_buff.len` prefix actually accessed by this program. The helpers remain the
real libbpf definitions. No guessed kernel-private struct layouts, CO-RE
relocations, vmlinux header or build-host kernel headers are required.
