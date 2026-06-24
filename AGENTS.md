# Repository Guidelines

## Testing

- Run the full project test suite with `make test`.
- Prefer the Makefile targets over raw `go test ./...` because they run `build-proto` first and pass the sqlite FTS5 build settings:
  - `CGO_ENABLED=1`
  - `-tags sqlite_fts5`
- If you need a focused package test, mirror the Makefile flags, for example:
  - `CGO_ENABLED=1 go test -tags sqlite_fts5 ./cmd/morph`
- Missing those flags can cause sqlite-backed tests to fail with `no such module: fts5`.

## Naming

- Prefer action-oriented prefixes for functions and methods that retrieve or prepare data:
  - Use `get` for cheap in-memory retrieval or derived values, such as `getSourceLinks`.
  - Use `load` for reading from storage, cache, or persisted state, such as `loadSessionSummary`.
  - Use `fetch` for remote, provider, network, or externally backed retrieval, such as `fetchModelResponse`.
  - Use `set` for mutating or assigning state, such as `setDefaultConfig`.
- Prefer state-oriented prefixes for predicates and validation-style helpers:
  - Use `is` for direct state checks, such as `isSessionIdle`.
  - Use `has` for possession, presence, or evidence checks, such as `hasReflectionEvidence`.
  - Use `check` for validation or multi-step decision logic that may return a reason or error, such as `checkPromotionEligibility`.
- Name adapters and conversion helpers with `XToY` or `XFromY` so the source and destination are obvious, such as `memoryModelToMemoryItem` or `memoryItemFromCandidate`.
- Avoid vague noun-only helper names when the function performs an action or check. For example, prefer `getSourceLinks` over `reflectionSourceLinks`, and prefer `hasMatchingReflectionCandidate` over `reflectionMatchesExistingCandidate`.
- Keep renames behavior-preserving. Naming cleanup should not change control flow, data shape, persistence, logging semantics, or public behavior unless the task explicitly asks for that.
