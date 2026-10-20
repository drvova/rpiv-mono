# Go port of rpiv-mono for PiG. Every directory under extensions/ is its own Go module; PiG builds each
# extension with GOWORK=off, so the per-module targets below prove what PiG will see. go.work exists for
# editors and for workspace-wide commands.

MODULES := $(sort $(dir $(wildcard extensions/*/go.mod)))
EXTENSIONS := $(sort $(dir $(wildcard extensions/*/extension.go)))
PIG ?= pig

.PHONY: all fmt fmt-check vet build test test-workspace validate tidy list

all: fmt-check vet build test

list:
	@echo "modules:    $(MODULES)"
	@echo "extensions: $(EXTENSIONS)"

fmt:
	@for m in $(MODULES); do (cd $$m && gofmt -w .); done

fmt-check:
	@out=$$(for m in $(MODULES); do (cd $$m && gofmt -l .); done); \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

vet:
	@for m in $(MODULES); do echo "== vet $$m"; (cd $$m && GOWORK=off go vet ./...) || exit 1; done

build:
	@for m in $(MODULES); do echo "== build $$m"; (cd $$m && GOWORK=off go build ./...) || exit 1; done

test:
	@for m in $(MODULES); do echo "== test $$m"; (cd $$m && GOWORK=off go test -race ./...) || exit 1; done

test-workspace:
	go test ./...

tidy:
	@for m in $(MODULES); do echo "== tidy $$m"; (cd $$m && GOWORK=off go mod tidy) || exit 1; done

# Validate every extension against a real pig binary without installing it (PIG=/path/to/pig make validate).
validate:
	@for e in $(EXTENSIONS); do echo "== validate $$e"; $(PIG) install $$e --validate-only --json || exit 1; done
