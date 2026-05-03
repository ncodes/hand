package core

import (
	"time"

	"github.com/wandxy/hand/pkg/nanoid"
)

const DefaultSessionID = "default"
const SessionIDPrefix = "ses_"
const ArchiveIDPrefix = "arc_"

func NewSessionID() (string, error) {
	return nanoid.Generate(SessionIDPrefix)
}

func NewArchiveID() (string, error) {
	return nanoid.Generate(ArchiveIDPrefix)
}

type Session struct {
	CreatedAt                time.Time
	Compaction               SessionCompaction
	ID                       string
	EpisodicCheckpointOffset int
	LastPromptTokens         int
	UpdatedAt                time.Time
}

type SessionCompactionStatus string

const (
	CompactionStatusPending   SessionCompactionStatus = "pending"
	CompactionStatusRunning   SessionCompactionStatus = "running"
	CompactionStatusSucceeded SessionCompactionStatus = "succeeded"
	CompactionStatusFailed    SessionCompactionStatus = "failed"
)

type SessionCompaction struct {
	CompletedAt        time.Time
	FailedAt           time.Time
	LastError          string
	RequestedAt        time.Time
	StartedAt          time.Time
	Status             SessionCompactionStatus
	TargetMessageCount int
	TargetOffset       int
}

type ArchivedSession struct {
	ID              string
	SourceSessionID string
	ArchivedAt      time.Time
	ExpiresAt       time.Time
}

type SessionSummary struct {
	SessionID          string
	SourceEndOffset    int
	SourceMessageCount int
	UpdatedAt          time.Time
	SessionSummary     string
	CurrentTask        string
	Discoveries        []string
	OpenQuestions      []string
	NextActions        []string
}
