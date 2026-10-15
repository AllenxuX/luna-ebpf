# Releasing

Release versions use annotated `vMAJOR.MINOR.PATCH` tags. Keep the Makefile,
Compose files, changelog and version-specific notes synchronized. For the
pre-1.0 MVP, document any compatibility or metric changes explicitly.

1. Run Go tests/vet, Linux race tests, BPF generation and cross-builds. Record
   privileged kernel/runtime results separately; do not infer them from CI.
2. Audit tracked source, release notes and attachments for private host data,
   credentials and internal registry names. Never upload internal image archives
   or raw private integration evidence.
3. Merge the reviewed changes into main after CI passes. Add notes at
   `docs/releases/vVERSION.md`, including installation, changes and limitations.
4. Create an annotated tag on the tested commit and push it. The release
   workflow validates the tag/version, runs tests, builds Linux binaries and
   an amd64 Docker archive, and publishes a draft only after all assets upload.
5. Confirm the workflow succeeds, download and check SHA256SUMS, and verify
   the published image tag and Compose agree. Never move a published tag;
   fix release problems with a new patch version.

Public images use `luna-ebpf:VERSION`. Release assets include dependency license
notices, Compose, Grafana JSON, alert rules and SHA256 checksums. SHA256 checks
integrity; these releases do not currently have cryptographic signing or an SBOM.
amd64 runtime validation does not establish arm64 runtime support.
