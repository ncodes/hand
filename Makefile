APP := agent
BUILD_DIR := build

.PHONY: build lint install

build:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP) ./cmd

lint:
	golangci-lint run ./...

install:
	go install ./cmd
