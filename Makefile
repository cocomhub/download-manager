VERSION ?= $(shell git describe --tags --always --dirty)
BUILD_DIR ?= build
BUILD_AT ?= $(shell date +"%Y-%m-%dT%H:%M:%SZ")
BIN_DIR ?= $(BUILD_DIR)/bin
BIN_NAME ?= $(BIN_DIR)/download-manager
CONFIG_FILE ?= $(BUILD_DIR)/config.yaml

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GO ?= GOOS=$(GOOS) GOARCH=$(GOARCH) go
GOFLAGS ?= -v
GO_LDFLAGS ?= -w -s
GO_LDFLAGS += -X main.Version=$(VERSION) -X main.BuildAt=$(BUILD_AT)

GOFMT=gofmt

# all .go files that are not auto-generated and should be auto-formatted and linted.
ALL_SRC = $(shell find . -name '*.go' \
				   -not -name 'doc.go' \
				   -not -name '_*' \
				   -not -name '.*' \
				   -not -name 'mocks*' \
				   -not -name 'model.pb.go' \
				   -not -name 'model_test.pb.go' \
				   -not -name 'storage_test.pb.go' \
				   -not -path './examples/*' \
				   -not -path './vendor/*' \
				   -not -path '*/mocks/*' \
				   -not -path '*/*-gen/*' \
				   -type f | \
				sort)

.PHONY: build

build: fmt
	mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(GO_LDFLAGS)" -o $(BIN_NAME) main.go

# 格式化目标
.PHONY: fmt
fmt: addlicense fix
	@echo Running gofmt on ALL_SRC ...
	@$(GOFMT) -e -s -l -w $(ALL_SRC)
	@echo Running gofumpt on ALL_SRC ...
# 	@$(GOFUMPT) -e -l -w $(ALL_SRC)

# 添加许可证
.PHONY: addlicense
addlicense:
	addlicense -c "The Cocomhub Authors. All rights reserved." -s=only -ignore ".claude/**" -ignore ".trae/**" -ignore ".cursor/**" .

# 修复目标
.PHONY: fix
fix:
	@echo Running go fix ./...
	@$(GO) fix ./...

.PHONY: clean

clean:
	rm -rf $(BIN_DIR)

.PHONY: run

run: build
	$(BIN_NAME) --config $(CONFIG_FILE)

.PHONY: show-version

show-version:
	$(BIN_NAME) --version

# Playwright E2E 测试
# Windows 下编译产出 playwright-server.exe，Unix 下产出 playwright-server
EXT =
ifneq ($(findstring Windows,$(OS)),)
    EXT = .exe
endif
PW_SERVER_BIN = cmd/playwright-server/playwright-server$(EXT)
PW_DIR = test/playwright

.PHONY: playwright-server
playwright-server: ## 编译 Playwright 测试用 Go server
	cd cmd/playwright-server && $(GO) build -o playwright-server$(EXT) .

.PHONY: playwright-test
playwright-test: playwright-server ## 运行 Playwright E2E 测试
	cd $(PW_DIR) && npx playwright test

.PHONY: playwright-ui
playwright-ui: playwright-server ## 运行 Playwright UI 交互模式（AI 辅助调试）
	cd $(PW_DIR) && npx playwright test --ui

.PHONY: playwright-report
playwright-report: ## 查看 Playwright 测试报告
	cd $(PW_DIR) && npx playwright show-report

.PHONY: playwright-codegen
playwright-codegen: playwright-server ## 启动 Playwright 代码生成器
	cd $(PW_DIR) && SERVER_BINARY=../../$(PW_SERVER_BIN) TEST_PORT=19199 npx playwright codegen http://localhost:19199
