package search

import "github.com/wandxy/hand/internal/state/search/vectorstore"

type SourceKind = vectorstore.SourceKind

const (
	SourceKindSessionMessage = vectorstore.SourceKindSessionMessage
	SourceKindMemoryItem     = vectorstore.SourceKindMemoryItem
)

type VectorStore = vectorstore.Store
type VectorRecordLister = vectorstore.RecordLister
type VectorRecord = vectorstore.Record
type VectorDeleteRequest = vectorstore.DeleteRequest
type VectorSearchRequest = vectorstore.SearchRequest
type VectorFilter = vectorstore.Filter
type VectorSearchResult = vectorstore.SearchResult
type VectorListRequest = vectorstore.ListRequest
type VectorListResult = vectorstore.ListResult
type VectorSearchMatch = vectorstore.SearchMatch
type VectorStoreMetadata = vectorstore.StoreMetadata
type VectorModelMetadata = vectorstore.ModelMetadata

var ValidateVectorRecord = vectorstore.ValidateRecord
var ValidateVectorSearchRequest = vectorstore.ValidateSearchRequest
var ValidateVectorListRequest = vectorstore.ValidateListRequest
var ValidateVectorDeleteRequest = vectorstore.ValidateDeleteRequest

func VectorContentHash(text string) string {
	return vectorstore.ContentHash(text)
}

func IsVectorRecordStale(record VectorRecord, text string) bool {
	return vectorstore.IsRecordStale(record, text)
}
