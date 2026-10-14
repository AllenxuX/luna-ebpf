# Third-party notices

Original project code is licensed under Apache-2.0. Go module dependencies retain their own licenses; the pinned dependency graph is in go.mod and go.sum.

Source-built Docker images include these notices, the project license, the vendored BSD notice and available top-level license/notice files from the pinned Go modules under /licenses/.

The files `internal/bpf/include/bpf_helpers.h` and `bpf_helper_defs.h` are vendored, unmodified libbpf v1.6.2 headers. Their SPDX expression is `LGPL-2.1 OR BSD-2-Clause`; this distribution uses the BSD-2-Clause option. The copyright and license text are retained in `internal/bpf/include/LICENSE.BSD-2-Clause`.

Source and provenance: https://github.com/libbpf/libbpf/tree/v1.6.2/src

The `GPL` license string in the BPF ELF is the kernel program license declaration. It does not replace the repository's source-file license notices or third-party notices.
