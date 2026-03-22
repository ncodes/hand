APP := hand
BUILD_DIR := build

.PHONY: build test lint install

build:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP) ./cmd/hand

test:
	go test ./...

lint:
	golangci-lint run ./...

install:
	go install ./cmd/hand
