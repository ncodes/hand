package core

import (
	"time"

	"github.com/wandxy/hand/pkg/nanoid"
)

// DefaultSessionID is the package-level default session id constant.
const DefaultSessionID = "default"

// SessionIDPrefix is the package-level session id prefix constant.
const SessionIDPrefix = "ses_"

// ArchiveIDPrefix is the package-level archive id prefix constant.
const ArchiveIDPrefix = "arc_"

// SessionTitleSourceGenerated is the package-level session title source generated constant.
const SessionTitleSourceGenerated = "generated"

// SessionTitleSourceManual is the package-level session title source manual constant.
const SessionTitleSourceManual = "manual"

// NewSessionID returns a newly generated session ID.
func NewSessionID() (string, error) {
	return nanoid.Generate(SessionIDPrefix)
}

// NewArchiveID returns a newly generated archive ID.
func NewArchiveID() (string, error) {
	return nanoid.Generate(ArchiveIDPrefix)
}

// Session describes an active conversation session.
type Session struct {
	Compaction                 SessionCompaction
	ID                         string
	EpisodicCheckpointOffset   int
	LastPromptTokens           int
	ReflectionCheckpointOffset int
	Title                      string
	TitleSource                string
	Archived                   bool
	ArchivedAt                 time.Time
	ExpiresAt                  time.Time
	UpdatedAt                  time.Time
	CreatedAt                  time.Time
}

// CheckpointPatch describes changes to apply to checkpoint state.
type CheckpointPatch struct {
	EpisodicOffset   *int
	ReflectionOffset *int
}

// SessionCompactionStatus records whether session history has been compacted.
type SessionCompactionStatus string

const (
	CompactionStatusPending   SessionCompactionStatus = "pending"
	CompactionStatusRunning   SessionCompactionStatus = "running"
	CompactionStatusSucceeded SessionCompactionStatus = "succeeded"
	CompactionStatusFailed    SessionCompactionStatus = "failed"
)

// SessionCompaction records compaction metadata for a session.
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

// ArchivedSession describes a session moved to archive storage.
type ArchivedSession struct {
	ID              string
	SourceSessionID string
	Title           string
	TitleSource     string
	ArchivedAt      time.Time
	ExpiresAt       time.Time
}

// SessionSummary summarizes session state.
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
