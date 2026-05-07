// Package memory is the provider layer for Hand's durable memory system.
//
// The package sits between the agent/tools and the state stores. It owns the
// policy decisions that should not leak into storage: guardrail validation,
// pinned-memory loading, candidate recording, reflection, promotion, lifecycle
// metadata, and background workers. The backing store is deliberately treated as
// a persistence/search primitive; higher-level memory meaning lives here.
//
// Memory moves through three broad phases:
//   - candidate creation: episodic extraction or explicit semantic/procedural
//     recording creates candidate records with source provenance.
//   - consolidation: reflection can turn episodic evidence into more durable
//     pinned, semantic, procedural, or episodic candidates.
//   - governance: promotion evaluates candidates against admission rules,
//     guardrails, confidence, provenance, and duplicate/conflict checks before a
//     memory becomes active.
//
// Most public interfaces in this package are intentionally capability-oriented.
// Callers should ask a provider what it supports and use the narrow interface
// they need instead of assuming every backend has every feature.
package memory
