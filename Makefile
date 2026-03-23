APP := hand
BUILD_DIR := build

.PHONY: install-tools build-proto build test lint install

install-tools:
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.76.0

build-proto:
	@PATH="$(PATH):$(HOME)/go/bin" protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative \
		internal/rpc/proto/hand.proto

build: build-proto
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(APP) ./cmd/hand

test: build-proto
	@go test ./...

lint:
	@golangci-lint run ./...

install: build-proto
	@go install ./cmd/hand
