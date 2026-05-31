package session

import (
	"context"
	"time"

	"github.com/wandxy/hand/pkg/agent/message"
)

const (
	DefaultID = "default"

	MessageOrderAsc  = "asc"
	MessageOrderDesc = "desc"
)

type CompactionStatus string

const (
	CompactionStatusPending   CompactionStatus = "pending"
	CompactionStatusRunning   CompactionStatus = "running"
	CompactionStatusSucceeded CompactionStatus = "succeeded"
	CompactionStatusFailed    CompactionStatus = "failed"
)

type Session struct {
	Compaction                 Compaction
	ID                         string
	EpisodicCheckpointOffset   int
	LastPromptTokens           int
	ReflectionCheckpointOffset int
	Title                      string
	TitleSource                string
	Archived                   bool
	ArchivedAt                 time.Time
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
	ExpiresAt                  time.Time
}

type Compaction struct {
	CompletedAt        time.Time
	FailedAt           time.Time
	LastError          string
	RequestedAt        time.Time
	StartedAt          time.Time
	Status             CompactionStatus
	TargetMessageCount int
	TargetOffset       int
}

type MessageQuery struct {
	Limit  int
	Name   string
	Order  string
	Offset int
	Role   message.Role
}

type Store interface {
	Resolve(context.Context, string) (Session, error)
	GetMessages(context.Context, string, MessageQuery) ([]message.Message, error)
	AppendMessages(context.Context, string, []message.Message) error
	UpdateLastPromptTokens(context.Context, string, int) error
}

type TraceEvent struct {
	ID        uint
	SessionID string
	Sequence  int
	Type      string
	Timestamp time.Time
	Payload   any
}

type TraceRecorder interface {
	AppendTraceEvent(context.Context, TraceEvent) (TraceEvent, error)
}
