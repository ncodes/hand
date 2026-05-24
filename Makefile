APP := hand
BUILD_DIR := build
GO ?= /opt/homebrew/Cellar/go/1.26.1/libexec/bin/go
GO_SQLITE_TAGS ?= sqlite_fts5
LIVE_CONFIG ?= $(CURDIR)/config.yaml
LIVE_ENV_FILE ?= $(CURDIR)/.env
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || printf dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf unknown)
LD_FLAGS := -X github.com/wandxy/hand/internal/constants.AppVersion=$(VERSION) -X github.com/wandxy/hand/internal/constants.CommitHash=$(COMMIT)

.PHONY: install-tools install-hooks build-proto build test test-agent-baseline test-spec test-live test-live-sqlite test-live-memory test-live-all agent-deps check-pkg-agent-deps lint install

install-tools:
	@$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
	@$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.76.0

install-hooks:
	@git config core.hooksPath .githooks
	@chmod +x .githooks/commit-msg
	@echo "Installed git hooks from .githooks"

build-proto:
	@PATH="$(PATH):$(HOME)/go/bin" protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative \
		internal/rpc/proto/hand.proto

build: build-proto
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=1 $(GO) build -tags $(GO_SQLITE_TAGS) -ldflags "$(LD_FLAGS)" -o $(BUILD_DIR)/$(APP) ./cmd/hand

test: build-proto
	@CGO_ENABLED=1 $(GO) test -tags $(GO_SQLITE_TAGS) ./...

test-agent-baseline: build-proto
	@CGO_ENABLED=1 $(GO) test -tags $(GO_SQLITE_TAGS) \
		./internal/agent \
		./internal/agent/context/compaction \
		./internal/agent/context/summary \
		./internal/tui/app \
		./cmd/hand

test-spec:
	@CGO_ENABLED=1 $(GO) test -tags $(GO_SQLITE_TAGS) ./internal/e2e ./cmd/hand ./cmd/session ./cmd/trace ./cmd/up -count=1

test-live:
	@$(MAKE) test-live-sqlite

test-live-sqlite:
	@HAND_E2E_LIVE=1 \
		HAND_E2E_LIVE_CONFIG="$(LIVE_CONFIG)" \
		HAND_E2E_LIVE_ENV_FILE="$$(if [ -f "$(LIVE_ENV_FILE)" ]; then printf '%s' "$(LIVE_ENV_FILE)"; fi)" \
		HAND_STORAGE_BACKEND=sqlite \
		CGO_ENABLED=1 $(GO) test -tags $(GO_SQLITE_TAGS) ./cmd/hand -run '^Test_E2E_HandLiveHarness_' -count=1

test-live-memory:
	@HAND_E2E_LIVE=1 \
		HAND_E2E_LIVE_CONFIG="$(LIVE_CONFIG)" \
		HAND_E2E_LIVE_ENV_FILE="$$(if [ -f "$(LIVE_ENV_FILE)" ]; then printf '%s' "$(LIVE_ENV_FILE)"; fi)" \
		HAND_STORAGE_BACKEND=memory \
		CGO_ENABLED=1 $(GO) test -tags $(GO_SQLITE_TAGS) ./cmd/hand -run '^Test_E2E_HandLiveHarness_' -count=1

test-live-all: test-live-sqlite test-live-memory

agent-deps:
	@$(GO) list -f 'direct imports for {{.ImportPath}}:{{range .Imports}}{{printf "\n  %s" .}}{{end}}' ./internal/agent
	@printf '\ninternal/agent transitive hand imports:\n'
	@$(GO) list -deps ./internal/agent | grep '^github.com/wandxy/hand/' | sed 's/^/  /'

check-pkg-agent-deps:
	@packages="$$(mktemp)"; \
	errors="$$(mktemp)"; \
	if ! $(GO) list ./pkg/agent/... > "$$packages" 2> "$$errors"; then \
		if grep -q 'lstat ./pkg/agent' "$$errors"; then \
			echo 'pkg/agent not present yet; dependency guard skipped'; \
			rm -f "$$packages" "$$errors"; \
			exit 0; \
		fi; \
		cat "$$errors"; \
		rm -f "$$packages" "$$errors"; \
		exit 1; \
	fi; \
	rm -f "$$packages" "$$errors"; \
	internal_imports="$$( $(GO) list -deps ./pkg/agent/... | grep '^github.com/wandxy/hand/internal' || true )"; \
	if [ -n "$$internal_imports" ]; then \
		echo 'pkg/agent must not import Hand internal packages:'; \
		printf '%s\n' "$$internal_imports"; \
		exit 1; \
	fi; \
	echo 'pkg/agent dependency guard passed'

lint:
	@golangci-lint run ./...

install: build-proto
	@CGO_ENABLED=1 $(GO) install -tags $(GO_SQLITE_TAGS) -ldflags "$(LD_FLAGS)" ./cmd/hand
