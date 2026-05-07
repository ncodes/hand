// Package pinned loads and prepares operator-controlled pinned memories.
//
// Pinned memory is treated differently from model-generated memory: it can come
// from a local file, it is always intended for direct context injection, and it
// is subject to strict promotion rules when created through reflection. This
// package handles file loading, limit enforcement, safety scanning, and
// redaction before pinned items reach the agent prompt.
package pinned
