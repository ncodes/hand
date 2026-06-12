---
title: Installation
description: Install or build Hand for local use.
---

# Installation

This page covers how to get the `hand` binary onto the machine that will run Hand. After `hand version` works,
continue with the [Quickstart](./quickstart) to configure credentials and send your first message.

## Install Script

The quickest installation path is the install script:

```bash
curl -fsSL https://handagent.ai/install.sh | bash
```

Verify the CLI is available:

```bash
hand version
```

If your shell cannot find `hand`, restart the shell or make sure the install directory printed by the installer is on
your `PATH`.

## Supported Platforms

Hand can be installed on macOS, Linux, and Windows.

- macOS and Linux use the standard home-directory layout under `~/.hand`.
- Windows uses the same profile structure under `%USERPROFILE%\.hand`.

Install Hand on the machine where you want the daemon to run. The TUI, CLI commands, and gateways connect to that local
daemon.

## Build From Source

Use this path when you are contributing to Hand, testing local changes, or prefer to build your tools from source.

Before running the Makefile targets, install the build basics:

- Go `1.26.1`.
- `make`.
- A C compiler toolchain with CGO support for the SQLite-backed runtime.
- `protoc`, the Protocol Buffers compiler.

Then install the Go protobuf generators used by the Makefile:

```bash
make install-tools
```

`make install-tools` installs `protoc-gen-go` and `protoc-gen-go-grpc` into your Go binary directory.

From the repository root:

```bash
make build
```

The compiled binary is written to `build/hand`.

You can also install the local source build into your Go binary directory:

```bash
make install
```

If you did not run `make install`, use the built binary directly:

```bash
./build/hand version
```

## Verify The Runtime Build

For development and validation, prefer the Makefile targets. They regenerate protobuf stubs and set the SQLite FTS5
build tag used by the runtime:

```bash
make test
```

For a focused Go package test, mirror the same flags:

```bash
CGO_ENABLED=1 go test -tags sqlite_fts5 ./cmd/hand
```

If a SQLite-backed test fails with `no such module: fts5`, the command was likely run without the FTS5 build tag or
without CGO support.

## Update Or Replace Hand

To update an installer-managed binary, rerun the install script:

```bash
curl -fsSL https://handagent.ai/install.sh | bash
```

To replace a source build, pull the repository changes and rebuild:

```bash
make build
```

Then use `./build/hand` or run `make install` again.

## Next Step

Continue with the [Quickstart](./quickstart) to create or select a profile, store model credentials, and start your
first chat.
