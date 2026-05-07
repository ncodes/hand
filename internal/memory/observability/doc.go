// Package observability adapts regular application logging and trace sessions
// to the memory provider's lightweight logging contract.
//
// Memory code records both human-readable logs and structured trace events. The
// adapter keeps that instrumentation available without coupling the provider to
// a particular logger implementation or trace storage backend.
package observability
