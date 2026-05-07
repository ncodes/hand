// Package episodic extracts source-linked memories from conversation history.
//
// Episodic memory is the most evidence-heavy memory kind. Every candidate is
// tied back to a bounded window of messages and, when available, trace events.
// The package keeps extraction deterministic around the model call: it controls
// windowing, provenance, candidate IDs, admission checks, duplicate suppression,
// and background checkpoints.
//
// The extractor is expected to propose useful candidates, not decide what is
// finally durable. Candidates remain in "candidate" status until the provider
// lifecycle promotes or rejects them.
package episodic
