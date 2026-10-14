FROM golang:1.25.6-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends clang llvm libbpf-dev make ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETARCH
RUN make generate && CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} go build -trimpath -ldflags="-s -w" -o /out/luna-ebpf ./cmd/luna-ebpf
COPY LICENSE THIRD_PARTY_NOTICES.md /out/licenses/
COPY internal/bpf/include/LICENSE.BSD-2-Clause /out/licenses/libbpf-BSD-2-Clause
RUN go list -m -f '{{if not .Main}}{{.Dir}}{{end}}' all > /tmp/module-dirs && \
    while IFS= read -r module_dir; do \
      [ -n "$module_dir" ] || continue; \
      license_dir="/out/licenses/dependencies/${module_dir#/go/pkg/mod/}"; \
      mkdir -p "$license_dir"; \
      find "$module_dir" -maxdepth 1 -type f \( -iname 'LICENSE*' -o -iname 'COPYING*' -o -iname 'NOTICE*' \) -exec cp -t "$license_dir" {} +; \
    done < /tmp/module-dirs

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/luna-ebpf /luna-ebpf
COPY configs/config.example.yaml /etc/luna-ebpf/config.yaml
COPY --from=build /out/licenses /licenses/
EXPOSE 9808
ENTRYPOINT ["/luna-ebpf"]
CMD ["--config", "/etc/luna-ebpf/config.yaml"]
