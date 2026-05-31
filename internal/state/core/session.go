package core

import (
	"context"
	"time"

	handmsg "github.com/wandxy/hand/pkg/agent/message"
	"github.com/wandxy/hand/pkg/nanoid"
)

// DefaultSessionID is the package-level default session id constant.
const DefaultSessionID = "default"

// SessionIDPrefix is the package-level session id prefix constant.
const SessionIDPrefix = "ses_"

// SessionTitleSourceGenerated is the package-level session title source generated constant.
const SessionTitleSourceGenerated = "generated"

// SessionTitleSourceManual is the package-level session title source manual constant.
const SessionTitleSourceManual = "manual"

// NewSessionID returns a newly generated session ID.
func NewSessionID() (string, error) {
	return nanoid.Generate(SessionIDPrefix)
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

// SessionArchiveRequest describes a session archive state transition.
type SessionArchiveRequest struct {
	ArchivedAt time.Time
	ExpiresAt  time.Time
}

// SessionGetOptions filters session lookup operations.
type SessionGetOptions struct {
	Archived *bool
}

// SessionListOptions filters session listing operations.
type SessionListOptions struct {
	Archived *bool
}

// SessionRenameRequest describes a session title change.
type SessionRenameRequest struct {
	SessionID   string
	Title       string
	TitleSource string
	RenamedAt   time.Time
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

// SessionMetadataStore defines persisted session metadata operations.
type SessionMetadataStore interface {
	Save(ctx context.Context, session Session) error
	Get(ctx context.Context, id string, opts SessionGetOptions) (Session, bool, error)
	List(ctx context.Context, opts SessionListOptions) ([]Session, error)
	Rename(ctx context.Context, req SessionRenameRequest) (Session, error)
	Delete(ctx context.Context, id string) error
	UpdateCheckpoints(ctx context.Context, id string, patch CheckpointPatch) error
	Archive(ctx context.Context, id string, req SessionArchiveRequest) (Session, error)
	Unarchive(ctx context.Context, id string) (Session, error)
	DeleteExpiredArchives(ctx context.Context, now time.Time) error
}

// CurrentSessionStore defines current session tracking operations.
type CurrentSessionStore interface {
	SetCurrent(ctx context.Context, id string) error
	Current(ctx context.Context) (string, bool, error)
	ClearCurrent(ctx context.Context) error
}

// SessionMessageStore defines persisted session message operations.
type SessionMessageStore interface {
	AppendMessages(ctx context.Context, id string, messages []handmsg.Message) error
	CountMessages(ctx context.Context, id string, opts MessageQueryOptions) (int, error)
	GetMessage(ctx context.Context, id string, index int) (handmsg.Message, bool, error)
	GetMessages(ctx context.Context, id string, opts MessageQueryOptions) ([]handmsg.Message, error)
	GetMessagesByIDs(ctx context.Context, id string, messageIDs []uint) ([]MessageRecord, error)
	GetMessageWindow(ctx context.Context, id string, anchorMessageID uint, before int, after int) ([]MessageRecord, error)
	SearchMessages(ctx context.Context, id string, opts SearchMessageOptions) ([]SearchMessageResult, error)
	ClearMessages(ctx context.Context, id string) error
}

// SessionSummaryStore defines persisted session summary operations.
type SessionSummaryStore interface {
	SaveSummary(ctx context.Context, summary SessionSummary) error
	GetSummary(ctx context.Context, sessionID string) (SessionSummary, bool, error)
	DeleteSummary(ctx context.Context, sessionID string) error
}

// SessionStore defines the persistence operations for conversation sessions.
type SessionStore interface {
	SessionMetadataStore
	CurrentSessionStore
	SessionMessageStore
	SessionSummaryStore
}
