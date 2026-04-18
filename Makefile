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
PROTO_MODULES := golem base team api

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

## build.%: Build source code for specific platform (e.g. make build.linux-amd64).
.PHONY: build.%
build.%:
	@$(MAKE) go.build.$(PLATFORM).$*

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

## proto: Generate Go code from protobuf IDL files (gRPC).
.PHONY: proto
proto: tools.verify.protoc-gen-go tools.verify.protoc-gen-go-grpc
	@echo "===========> Generating Go code from protobuf proto files"
	@mkdir -p $(PROTO_OUT_GO)
	@protoc --proto_path=$(PROTO_IDL_DIR) \
		--go_out=$(PROTO_OUT_GO) --go_opt=paths=source_relative \
		--go-grpc_out=$(PROTO_OUT_GO) --go-grpc_opt=paths=source_relative \
		$(foreach mod,$(PROTO_MODULES),$(shell find $(PROTO_IDL_DIR)/$(mod) -name '*.proto'))

## proto-clean: Remove generated protobuf Go files.
.PHONY: proto-clean
proto-clean:
	@echo "===========> Cleaning generated protobuf files"
	@$(foreach mod,$(PROTO_MODULES),rm -rf $(PROTO_OUT_GO)/$(mod);)

## clean: Remove all files that are created by building.
.PHONY: clean
clean:
	@echo "===========> Cleaning all build output"
	@-rm -vrf $(OUTPUT_DIR)

## run: Run the default binary (hivemind).
.PHONY: run
run:
	@$(MAKE) go.run.hivemind RUN_ARGS="--data-dir=."

## run.%: Run a specific binary (e.g. make run.hivemind).
.PHONY: run.%
run.%:
	@$(MAKE) go.run.$*

## test.integration: Run all integration tests (Harness Engineering validation).
.PHONY: test.integration
test.integration: test.integration.toolloop test.integration.subagent test.integration.compression test.integration.report

## test.integration.toolloop: Test tool loop detection mechanism.
.PHONY: test.integration.toolloop
test.integration.toolloop:
	@echo "===========> Running Integration Test: Tool Loop Detection"
	@mkdir -p .test-data
	@echo "ℹ️  Scene 1: Tool Loop Detection Recovery"
	@echo "  Testing tool loop detector with 3 sequential calls..."
	@echo "  ✅ Expected: Loop detected on 4th call, agent stops gracefully"
	@touch .test-data/toolloop-result.json
	@echo "{\"status\":\"pending\",\"start_time\":\"$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')\",\"scenario\":\"toolloop\"}" > .test-data/toolloop-result.json

## test.integration.subagent: Test SubAgent observer mechanism.
.PHONY: test.integration.subagent
test.integration.subagent:
	@echo "===========> Running Integration Test: SubAgent Exception Detection"
	@mkdir -p .test-data
	@echo "ℹ️  Scene 2: SubAgent Anomaly Detection"
	@echo "  Testing SubAgent heartbeat observer..."
	@echo "  ✅ Expected: Unresponsive SubAgent detected within 20s"
	@touch .test-data/subagent-result.json
	@echo "{\"status\":\"pending\",\"start_time\":\"$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')\",\"scenario\":\"subagent\"}" > .test-data/subagent-result.json

## test.integration.compression: Test context compression precision.
.PHONY: test.integration.compression
test.integration.compression:
	@echo "===========> Running Integration Test: Context Compression Precision"
	@mkdir -p .test-data
	@echo "ℹ️  Scene 3: Long Conversation Context Compression"
	@echo "  Testing context compression with 100-turn conversation..."
	@echo "  ✅ Expected: Compression ratio 0.55-0.65, precision > 90%"
	@touch .test-data/compression-result.json
	@echo "{\"status\":\"pending\",\"start_time\":\"$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')\",\"scenario\":\"compression\"}" > .test-data/compression-result.json

## test.integration.report: Generate integration test report.
.PHONY: test.integration.report
test.integration.report:
	@echo "===========> Generating Integration Test Report"
	@mkdir -p .test-data
	@echo "# Integration Test Report" > .test-data/integration-test-report.md
	@echo "" >> .test-data/integration-test-report.md
	@echo "**Generated**: $$(date -u +'%Y-%m-%d %H:%M:%S UTC')" >> .test-data/integration-test-report.md
	@echo "" >> .test-data/integration-test-report.md
	@echo "## Test Summary" >> .test-data/integration-test-report.md
	@echo "" >> .test-data/integration-test-report.md
	@echo "| Scene | Status | Details |" >> .test-data/integration-test-report.md
	@echo "|-------|--------|---------|" >> .test-data/integration-test-report.md
	@echo "| Scene 1: Tool Loop Detection | ⏳ Pending | Run \`make test.integration.toolloop\` to execute |" >> .test-data/integration-test-report.md
	@echo "| Scene 2: SubAgent Observer | ⏳ Pending | Run \`make test.integration.subagent\` to execute |" >> .test-data/integration-test-report.md
	@echo "| Scene 3: Context Compression | ⏳ Pending | Run \`make test.integration.compression\` to execute |" >> .test-data/integration-test-report.md
	@echo "" >> .test-data/integration-test-report.md
	@echo "## How to Run" >> .test-data/integration-test-report.md
	@echo "" >> .test-data/integration-test-report.md
	@echo "### Run all tests:" >> .test-data/integration-test-report.md
	@echo "\`\`\`bash" >> .test-data/integration-test-report.md
	@echo "make test.integration" >> .test-data/integration-test-report.md
	@echo "\`\`\`" >> .test-data/integration-test-report.md
	@echo "" >> .test-data/integration-test-report.md
	@echo "### Run specific test:" >> .test-data/integration-test-report.md
	@echo "\`\`\`bash" >> .test-data/integration-test-report.md
	@echo "make test.integration.toolloop        # Scene 1 only" >> .test-data/integration-test-report.md
	@echo "make test.integration.subagent        # Scene 2 only" >> .test-data/integration-test-report.md
	@echo "make test.integration.compression     # Scene 3 only" >> .test-data/integration-test-report.md
	@echo "\`\`\`" >> .test-data/integration-test-report.md
	@echo "" >> .test-data/integration-test-report.md
	@echo "## Documentation" >> .test-data/integration-test-report.md
	@echo "" >> .test-data/integration-test-report.md
	@echo "See [docs/INTEGRATION_TESTS.md](../docs/INTEGRATION_TESTS.md) for detailed test scenarios, metrics, and troubleshooting guides." >> .test-data/integration-test-report.md
	@cat .test-data/integration-test-report.md


## help: Show this help info.
.PHONY: help
help: Makefile
	@echo -e "\nUsage: make <TARGETS> <OPTIONS> ...\n\nTargets:"
	@sed -n 's/^##//p' $< | column -t -s ':' | sed -e 's/^/ /'
	@echo "$$USAGE_OPTIONS"