package constants

import "time"

const (
	// DefaultHybridRetrievalCandidateLimit is the fallback candidate limit for hybrid retrieval.
	DefaultHybridRetrievalCandidateLimit = 100
	// MaxHybridRetrievalCandidateLimit is the hard maximum candidate limit for hybrid retrieval.
	MaxHybridRetrievalCandidateLimit = 1000
	// DefaultRerankCandidateLimit is the fallback candidate limit for reranking.
	DefaultRerankCandidateLimit = 100
	// ReciprocalRankFusionConstant is the rank offset used by reciprocal rank fusion.
	ReciprocalRankFusionConstant = 60
	// DefaultVectorRepairBatchSize is the fallback batch size for vector repair.
	DefaultVectorRepairBatchSize = 100
	// DefaultVectorStoreRebuildBatchSize is the fallback batch size for vector store rebuilds.
	DefaultVectorStoreRebuildBatchSize = 100
)

const (
	// DefaultRerankLexicalWeight is the fallback lexical score weight for deterministic reranking.
	DefaultRerankLexicalWeight = 0.45
	// DefaultRerankVectorWeight is the fallback vector score weight for deterministic reranking.
	DefaultRerankVectorWeight = 0.45
	// DefaultRerankFusedWeight is the fallback fused score weight for deterministic reranking.
	DefaultRerankFusedWeight = 0.10
)

const (
	// DefaultLLMRerankerMaxCandidates is the fallback candidate limit for LLM reranking.
	DefaultLLMRerankerMaxCandidates = 20
	// DefaultLLMRerankerMaxCandidateTextLen is the fallback text budget per LLM rerank candidate.
	DefaultLLMRerankerMaxCandidateTextLen = 1200
)

const (
	// DefaultEmbeddingMaxInputsPerBatch is the fallback maximum inputs per embedding batch.
	DefaultEmbeddingMaxInputsPerBatch = 96
	// DefaultEmbeddingMaxInputTextBytes is the fallback byte budget for each embedding input.
	DefaultEmbeddingMaxInputTextBytes = 32 * 1024
	// DefaultEmbeddingTimeout is the fallback timeout for embedding requests.
	DefaultEmbeddingTimeout = 30 * time.Second
	// DefaultEmbeddingMaxRetries is the fallback retry count for embedding requests.
	DefaultEmbeddingMaxRetries = 2
)
