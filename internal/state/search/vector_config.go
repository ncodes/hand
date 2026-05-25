package search

// VectorStoreOptions wires embedding, reranking, and vector storage for retrieval.
type VectorStoreOptions struct {
	Embedder            Embedder
	Reranker            Reranker
	VectorStore         VectorStore
	EnableRerank        *bool
	EmbeddingModel      string
	RebuildBatchSize    int
	RerankMaxCandidates int
	Diagnostics         bool
	Required            bool
}

// VectorConfig is the resolved vector retrieval configuration used by stores.
type VectorConfig struct {
	Provider    Embedder
	Reranker    Reranker
	Store       VectorStore
	Model       string
	RerankMax   int
	Diagnostics bool
	Rerank      bool
	Required    bool
}
