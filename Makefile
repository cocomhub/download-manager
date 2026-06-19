# Copyright 2026 The Cocomhub Authors. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

PROJECT_NAME := download-manager

# ═══════════════════════════════════════════════
# STANDARD VARIABLES
# ═══════════════════════════════════════════════
BUILD_DIR       ?= build
BIN_DIR         ?= $(BUILD_DIR)/bin
RAW_GO          ?= go
GOOS            ?= $(shell $(RAW_GO) env GOOS)
GOARCH          ?= $(shell $(RAW_GO) env GOARCH)
HOST_GOARCH     ?= $(shell $(RAW_GO) env GOHOSTARCH)
EXE             :=
GO              := GOOS=$(GOOS) GOARCH=$(GOARCH) $(RAW_GO)
GORACE          := -race
GOTEST_COUNT    ?= -count=1
GOTEST_TIMEOUT  ?= -timeout=5m
NOTEST_IGNORE   := .notestignore
SUB_MODULE_DIRS := $(shell find . -name 'go.mod' \
  -not -path './$(BUILD_DIR)/*' \
  -not -path './.claude/*' \
  -not -path './vendor/*' \
  -exec dirname {} \; | sort -u | grep -v '^\.$$')

# ═══════════════════════════════════════════════
# CUSTOM VARIABLES
# ═══════════════════════════════════════════════
COVER_THRESHOLD ?= 40
SONAR_PROJECT_KEY ?= cocomhub_download-manager
SKIP_VERSION    ?= true
CONFIG_FILE     ?= $(BUILD_DIR)/config.yaml
GOTAGS          ?=
GOBUILD_EXTRA   ?= -v
VERSION         ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_AT        ?= $(shell date +"%Y-%m-%dT%H:%M:%SZ")
GO_LDFLAGS      := -X main.Version=$(VERSION) -X main.BuildAt=$(BUILD_AT)

# ═══════════════════════════════════════════════
# OTHER VARIABLES
# ═══════════════════════════════════════════════
GOFMT := gofmt
ALL_SRC := $(shell find . -name '*.go' \
  -not -name 'doc.go' \
  -not -name '_*' \
  -not -name '.*' \
  -not -name 'mocks*' \
  -not -name 'model.pb.go' \
  -not -name 'model_test.pb.go' \
  -not -name 'storage_test.pb.go' \
  -not -path './examples/*' \
  -not -path './vendor/*' \
  -not -path './.claude/*' \
  -not -path './.trae/*' \
  -not -path './.cursor/*' \
  -not -path '*/mocks/*' \
  -not -path '*/*-gen/*' \
  -type f | sort)
ALL_PKGS := $(sort $(dir $(ALL_SRC)))

PW_DIR := test/playwright
PW_SERVER_DIR := cmd/playwright-server
EXT =
ifneq ($(findstring Windows,$(OS)),)
    EXT = .exe
endif
PW_SERVER_BIN := $(PW_SERVER_DIR)/playwright-server$(EXT)

# ═══════════════════════════════════════════════
# STANDARD TARGETS
# ═══════════════════════════════════════════════

.DEFAULT_GOAL := help

.PHONY: prepare
prepare:
	@mkdir -p $(BUILD_DIR) $(BIN_DIR)

.PHONY: build
build: fmt
	$(GO) build $(GOBUILD_EXTRA) -ldflags "$(GO_LDFLAGS)" -o $(BIN_DIR)/$(PROJECT_NAME)$(EXE) .

.PHONY: build-ci
build-ci: prepare
	@echo "Build (skip fmt, for CI)"
	$(GO) build $(GOBUILD_EXTRA) -ldflags "$(GO_LDFLAGS)" -o $(BIN_DIR)/$(PROJECT_NAME)$(EXE) .

.PHONY: test
test: prepare
	$(GO) test $(GORACE) $(GOTEST_COUNT) $(GOTEST_TIMEOUT) $(GOTAGS) ./...

.PHONY: test-ci test-cover
test-ci test-cover: prepare
	$(GO) test $(GORACE) $(GOTEST_COUNT) $(GOTEST_TIMEOUT) -coverprofile=$(BUILD_DIR)/cover.out -covermode=atomic $(GOTAGS) ./...

.PHONY: notest
notest:
	@scripts/check-test-files.sh $(ALL_PKGS)

.PHONY: cover-check
cover-check: test-cover
	@total=$$(go tool cover -func=$(BUILD_DIR)/cover.out | tail -1 | awk '{print $$NF}') && \
	total_int=$${total%\%} && \
	if [ "$$(echo "$$total_int < $(COVER_THRESHOLD)" | bc -l 2>/dev/null)" = "1" ]; then \
		echo "FAIL: coverage $$total is below threshold $(COVER_THRESHOLD)%"; \
		exit 1; \
	else \
		echo "OK: coverage $$total meets threshold $(COVER_THRESHOLD)%"; \
	fi

.PHONY: sonar-analyze
sonar-analyze:
	@if [ ! -f sonar-project.properties ]; then \
		echo "missing sonar-project.properties"; exit 1; \
	fi
	sonar-scanner

.PHONY: sonar-remediate
sonar-remediate:
	@if [ ! -f sonar-project.properties ]; then \
		echo "missing sonar-project.properties"; exit 1; \
	fi
	sonar-scanner -Dsonar.remediation.projectKey=$(SONAR_PROJECT_KEY)

.PHONY: vet
vet:
	$(RAW_GO) vet ./...

.PHONY: lint
lint:
	golangci-lint run

.PHONY: bench
bench: prepare
	@mkdir -p $(BUILD_DIR)/bench
	$(GO) test -bench=. -benchmem -count=5 -run=^$$ $(GOTAGS) ./... > $(BUILD_DIR)/bench/bench.txt

.PHONY: bench-compare
bench-compare:
	@which benchstat > /dev/null 2>&1 || go install golang.org/x/perf/cmd/benchstat@latest
	@if [ -f $(BUILD_DIR)/bench/bench.txt ] && [ -f $(BUILD_DIR)/bench/baseline.txt ]; then \
		benchstat $(BUILD_DIR)/bench/baseline.txt $(BUILD_DIR)/bench/bench.txt; \
	else \
		echo "Need both bench.txt and baseline.txt to compare"; \
		exit 1; \
	fi

.PHONY: check-loopback
check-loopback:
	@if grep -rn '0\.0\.0\.0' --include='*.go' . \
		| grep -v '_test.go' \
		| grep -v 'vendor/' \
		| grep -v 'testdata/' \
		| grep -v 'fixtures/' \
		| grep '.' > /dev/null 2>&1; then \
		echo "FAIL: found potential unsafe listen addresses (0.0.0.0)"; \
		grep -rn '0\.0\.0\.0' --include='*.go' . \
			| grep -v '_test.go' \
			| grep -v 'vendor/' \
			| grep -v 'testdata/' \
			| grep -v 'fixtures/'; \
		exit 1; \
	else \
		echo "OK: no unsafe loopback addresses found"; \
	fi

.PHONY: gofix
gofix:
	$(RAW_GO) fix ./...

.PHONY: addlicense
addlicense:
	addlicense -c "The Cocomhub Authors. All rights reserved." -s=only -ignore ".claude/**" -ignore ".trae/**" -ignore ".cursor/**" .

.PHONY: fmt
fmt: addlicense gofix
	@echo "Running gofmt on ALL_SRC ..."
	@$(GOFMT) -e -s -l -w $(ALL_SRC)

.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)

.PHONY: test-all
test-all:
	@for dir in $(SUB_MODULE_DIRS); do \
		echo "=== Testing $$dir ==="; \
		cd "$$dir" && $(RAW_GO) test $(GORACE) $(GOTEST_COUNT) $(GOTEST_TIMEOUT) $(GOTAGS) ./...; \
		cd "$(CURDIR)"; \
	done

.PHONY: build-all
build-all:
	@for dir in $(SUB_MODULE_DIRS); do \
		echo "=== Building $$dir ==="; \
		cd "$$dir" && $(RAW_GO) build $(GOBUILD_EXTRA) ./...; \
		cd "$(CURDIR)"; \
	done

.PHONY: check-ci
check-ci: vet lint check-loopback notest build-ci test-cover cover-check test-all build-all
	@echo "CI pipeline passed"

.PHONY: help
help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Standard targets:"
	@echo "  build           Build project (fmt first)"
	@echo "  build-ci        Build project (skip fmt, for CI)"
	@echo "  test            Run tests"
	@echo "  test-ci         Run tests with coverage (for CI)"
	@echo "  test-cover      Alias for test-ci"
	@echo "  notest          Check test files exist"
	@echo "  cover-check     Check coverage threshold"
	@echo "  vet             Run go vet"
	@echo "  lint            Run golangci-lint"
	@echo "  bench           Run benchmarks"
	@echo "  bench-compare   Compare benchmark results with baseline"
	@echo "  check-loopback  Check for unsafe listen addresses"
	@echo "  gofix           Run go fix"
	@echo "  addlicense      Add license headers"
	@echo "  fmt             Format code (addlicense + gofix + gofmt)"
	@echo "  clean           Clean build artifacts"
	@echo "  test-all        Test all sub-modules"
	@echo "  build-all       Build all sub-modules"
	@echo "  check-ci        Full CI pipeline"
	@echo "  sonar-analyze    Run SonarQube Cloud analysis"
	@echo "  sonar-remediate  Run SonarQube Cloud remediation"
	@echo ""
	@echo "Custom targets:"
	@echo "  all             vet test bench (quick check)"
	@echo "  run             Build and run"
	@echo "  show-version    Show binary version"
	@echo "  test-no-mongo   Run tests with no_mongo tag"
	@echo "  test-cover-html Generate coverage HTML report"
	@echo "  playwright-server  Build Playwright test server"
	@echo "  playwright-test    Run Playwright tests"
	@echo "  playwright-ui      Run Playwright tests in UI mode"
	@echo "  playwright-report  Generate Playwright report"
	@echo "  playwright-codegen Run Playwright codegen"
	@echo "  playwright-report-gen Generate Playwright summary report"
	@echo "  install-hooks    Install git hooks"

# ═══════════════════════════════════════════════
# CUSTOM TARGETS
# ═══════════════════════════════════════════════

.PHONY: all
all: vet test bench
	@echo "All checks passed"

.PHONY: run
run: build
	$(BIN_DIR)/$(PROJECT_NAME)$(EXE) --config $(CONFIG_FILE)

.PHONY: show-version
show-version:
	$(BIN_DIR)/$(PROJECT_NAME)$(EXE) --version

.PHONY: test-no-mongo
test-no-mongo:
	go test -tags no_mongo -race -count=1 -timeout=180s ./...

.PHONY: test-cover-html
test-cover-html: test-cover
	go tool cover -html=$(BUILD_DIR)/cover.out -o $(BUILD_DIR)/cover.html
	@echo "Coverage report: $(BUILD_DIR)/cover.html"

.PHONY: playwright-server
playwright-server:
	cd $(PW_SERVER_DIR) && $(GO) build -o playwright-server$(EXT) .

.PHONY: playwright-test
playwright-test: playwright-server
	cd $(PW_DIR) && npx playwright test

.PHONY: playwright-ui
playwright-ui: playwright-server
	cd $(PW_DIR) && npx playwright test --ui

.PHONY: playwright-report
playwright-report:
	@echo "Playwright report target"

.PHONY: playwright-codegen
playwright-codegen:
	cd $(PW_DIR) && SERVER_BINARY=../../$(PW_SERVER_BIN) TEST_PORT=19199 npx playwright codegen http://localhost:19199

.PHONY: playwright-report-gen
playwright-report-gen:
	@if command -v bash >/dev/null 2>&1; then \
		bash scripts/playwright-report-gen.sh; \
	else \
		echo "Skipping report generation (requires bash)"; \
	fi

.PHONY: install-hooks
install-hooks:
	@echo "Installing git hooks..."
	git config core.hooksPath .githooks
	@echo "Git hooks installed at .githooks/"
