GO ?= go
BPF_CLANG ?= clang
BPF_STRIP ?= llvm-strip
ARCH ?= $(shell $(GO) env GOARCH)
VERSION ?= 0.1.4
IMAGE_REPOSITORY ?= luna-ebpf
IMAGE = $(IMAGE_REPOSITORY):$(VERSION)

.PHONY: generate build test race lint docker docker-save preflight integration clean
generate:
	BPF2GO_CC=$(BPF_CLANG) BPF2GO_STRIP=$(BPF_STRIP) $(GO) generate ./internal/bpf
build:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) $(GO) build -trimpath -o bin/luna-ebpf ./cmd/luna-ebpf
test:
	$(GO) test ./...
race:
	CGO_ENABLED=1 $(GO) test -race ./...
lint:
	$(GO) vet ./...
docker:
	docker build -t $(IMAGE) .
docker-save: docker
	mkdir -p dist
	docker save -o dist/luna-ebpf-$(VERSION)-linux-$(ARCH).tar $(IMAGE)
	gzip -f dist/luna-ebpf-$(VERSION)-linux-$(ARCH).tar
preflight:
	LUNA_EBPF_BINARY=./bin/luna-ebpf bash scripts/preflight.sh
integration:
	bash scripts/integration-test.sh
clean:
	$(GO) clean ./...
	rm -f bin/luna-ebpf
