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

.PHONY: build

build:
	mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(GO_LDFLAGS)" -o $(BIN_NAME) main.go

.PHONY: clean

clean:
	rm -rf $(BIN_DIR)

.PHONY: run

run: build
	$(BIN_NAME) --config $(CONFIG_FILE)

.PHONY: show-version

show-version:
	$(BIN_NAME) --version
