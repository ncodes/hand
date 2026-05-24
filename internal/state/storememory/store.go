package storememory

import (
	"sync"

	base "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/state/search"
	handmsg "github.com/wandxy/hand/pkg/agent/message"
)

type Store struct {
	vectors         *search.VectorConfig
	memoryReranker  search.Reranker
	mu              sync.RWMutex
	sessions        map[string]Session
	messages        map[string][]handmsg.Message
	summaries       map[string]SessionSummary
	memoryItems     map[string]base.MemoryItem
	traceEvents     map[string][]base.TraceEvent
	traceSequences  map[string]int
	archives        map[string]ArchivedSession
	archiveMessages map[string][]handmsg.Message
	currentSession  string
	nextMessageID   uint
	nextTraceID     uint
}

func NewStore() *Store {
	return &Store{
		sessions:        make(map[string]Session),
		messages:        make(map[string][]handmsg.Message),
		summaries:       make(map[string]SessionSummary),
		memoryItems:     make(map[string]base.MemoryItem),
		traceEvents:     make(map[string][]base.TraceEvent),
		traceSequences:  make(map[string]int),
		archives:        make(map[string]ArchivedSession),
		archiveMessages: make(map[string][]handmsg.Message),
	}
}
