APP := hand
BUILD_DIR := build
GO ?= /opt/homebrew/Cellar/go/1.26.1/libexec/bin/go
LIVE_CONFIG ?= $(CURDIR)/config.yaml
LIVE_ENV_FILE ?= $(CURDIR)/.env

.PHONY: install-tools build-proto build test test-spec test-live lint install

install-tools:
	@$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
	@$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.76.0

build-proto:
	@PATH="$(PATH):$(HOME)/go/bin" protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative \
		internal/rpc/proto/hand.proto

build: build-proto
	@mkdir -p $(BUILD_DIR)
	@$(GO) build -o $(BUILD_DIR)/$(APP) ./cmd/hand

test: build-proto
	@$(GO) test ./...

test-spec:
	@$(GO) test ./internal/e2e ./cmd/hand ./cmd/session ./cmd/trace -count=1

test-live:
	@HAND_E2E_LIVE_CONFIG="$(LIVE_CONFIG)" \
		HAND_E2E_LIVE_ENV_FILE="$$(if [ -f "$(LIVE_ENV_FILE)" ]; then printf '%s' "$(LIVE_ENV_FILE)"; fi)" \
		$(GO) test ./cmd/hand -run 'Test_E2E_HandLiveHarness_RootChat$$' -count=1

lint:
	@golangci-lint run ./...

install: build-proto
	@$(GO) install ./cmd/hand
