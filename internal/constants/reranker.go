package constants

const (
	// RerankerNoop identifies the reranker that preserves fused candidate scores.
	RerankerNoop = "noop"
	// RerankerDeterministic identifies the built-in deterministic reranker.
	RerankerDeterministic = "deterministic"
	// RerankerLLM identifies an LLM-backed reranker.
	RerankerLLM = "llm"
)
