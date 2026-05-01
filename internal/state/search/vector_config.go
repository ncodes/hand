package search

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
