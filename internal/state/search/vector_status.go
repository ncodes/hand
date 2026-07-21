package search

import "time"

type VectorIndexStatus string

const (
	VectorIndexPending VectorIndexStatus = "pending"
	VectorIndexReady   VectorIndexStatus = "ready"
	VectorIndexFailed  VectorIndexStatus = "failed"
	VectorIndexSkipped VectorIndexStatus = "skipped"
)

type VectorIndexState struct {
	SourceID  string
	SessionID string
	MessageID uint
	Status    VectorIndexStatus
	Attempts  int
	ErrorKind string
	UpdatedAt time.Time
}
