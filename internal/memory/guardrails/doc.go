// Package guardrails provides the default validation and redaction checks used
// by memory providers.
//
// Guardrails are intentionally small and synchronous. They protect provider
// boundaries from malformed queries, unsafe writes, and prompt-injection content
// before memory is searched, stored, or injected back into model context.
package guardrails
