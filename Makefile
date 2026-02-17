# Build all by default, even if it's not first
.DEFAULT_GOAL := all

.PHONY: all
all: tidy format lint build

# ==============================================================================
# Build options

ROOT_PACKAGE=github.com/kiosk404/echoryn
VERSION_PACKAGE=github.com/kiosk404/echoryn/pkg/version

# Protobuf IDL options
PROTO_IDL_DIR := ./idl
PROTO_OUT_GO := ./pkg/proto

# ==============================================================================
# Includes

include scripts/make-rules/common.mk # make sure include common.mk at the first include line
include scripts/make-rules/golang.mk
include scripts/make-rules/tools.mk

# ==============================================================================
# Usage

define USAGE_OPTIONS

Options:
  DEBUG            Whether to generate debug symbols. Default is 0.
  DLV              Set to 1 to enable dlv debug symbols. Default is empty.
  BINS             The binaries to build. Default is all of cmd.
                   This option is available when using: make build/build.multiarch
                   Example: make build BINS="hivemind echoctl"
  VERSION          The version information compiled into binaries.
                   The default is obtained from gsemver or git.
  V                Set to 1 enable verbose build. Default is 0.
endef
export USAGE_OPTIONS

# ==============================================================================
# Targets

## build: Build source code for host platform.
.PHONY: build
build:
	@$(MAKE) go.build

## tidy: Run go mod tidy.
.PHONY: tidy
tidy:
	@$(MAKE) go.tidy

## test: Run unit tests.
.PHONY: test
test:
	@$(MAKE) go.test

## cover: Run unit tests with coverage.
.PHONY: cover
cover:
	@$(MAKE) go.test.cover

## lint: Run golangci-lint.
.PHONY: lint
lint:
	@$(MAKE) go.lint

## format: Gofmt (reformat) package sources (exclude vendor dir if existed).
.PHONY: format
format: tools.verify.golines tools.verify.goimports
	@echo "===========> Formating codes"
	@$(FIND) -type f -name '*.go' | $(XARGS) gofmt -s -w
	@$(FIND) -type f -name '*.go' | $(XARGS) goimports -w -local $(ROOT_PACKAGE)
	@$(FIND) -type f -name '*.go' | $(XARGS) golines -w --max-len=240 --reformat-tags --shorten-comments --ignore-generated .
	@$(GO) mod edit -fmt

## proto: Generate Go code from protobuf IDL files.
.PHONY: proto
proto:
	@echo "===========> Generating Go code from protobuf IDL files"
	@mkdir -p $(PROTO_OUT_GO)
	@protoc --proto_path=$(PROTO_IDL_DIR) \
		--go_out=$(PROTO_OUT_GO) --go_opt=paths=source_relative \
		--go-grpc_out=$(PROTO_OUT_GO) --go-grpc_opt=paths=source_relative \
		$(shell find $(PROTO_IDL_DIR) -name '*.proto')

## clean: Remove all files that are created by building.
.PHONY: clean
clean:
	@echo "===========> Cleaning all build output"
	@-rm -vrf $(OUTPUT_DIR)

## run: Run the default binary (hivemind).
.PHONY: run
run:
	@$(MAKE) go.run.hivemind

## run.%: Run a specific binary (e.g. make run.hivemind).
.PHONY: run.%
run.%:
	@$(MAKE) go.run.$*

## help: Show this help info.
.PHONY: help
help: Makefile
	@echo -e "\nUsage: make <TARGETS> <OPTIONS> ...\n\nTargets:"
	@sed -n 's/^##//p' $< | column -t -s ':' | sed -e 's/^/ /'
	@echo "$$USAGE_OPTIONS"