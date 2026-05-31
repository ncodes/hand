package core

import (
	"context"
	"time"

	handmsg "github.com/wandxy/hand/pkg/agent/message"
)

// SessionStore defines the persistence operations for conversation sessions.
type SessionStore interface {
	// Session management
	Save(ctx context.Context, session Session) error
	Get(ctx context.Context, id string) (Session, bool, error)
	List(ctx context.Context) ([]Session, error)
	Delete(ctx context.Context, id string) error
	UpdateCheckpoints(ctx context.Context, id string, patch CheckpointPatch) error
	Archive(ctx context.Context, req SessionArchiveRequest) (Session, error)
	Unarchive(ctx context.Context, id string) (Session, error)
	DeleteExpiredArchives(ctx context.Context, now time.Time) error

	// Current session tracking
	SetCurrent(ctx context.Context, id string) error
	Current(ctx context.Context) (string, bool, error)

	// Message operations
	AppendMessages(ctx context.Context, id string, messages []handmsg.Message) error
	CountMessages(ctx context.Context, id string, opts MessageQueryOptions) (int, error)
	GetMessage(ctx context.Context, id string, index int, opts MessageQueryOptions) (handmsg.Message, bool, error)
	GetMessages(ctx context.Context, id string, opts MessageQueryOptions) ([]handmsg.Message, error)
	GetMessagesByIDs(ctx context.Context, id string, messageIDs []uint) ([]MessageRecord, error)
	GetMessageWindow(ctx context.Context, id string, anchorMessageID uint, before int, after int) ([]MessageRecord, error)
	SearchMessages(ctx context.Context, id string, opts SearchMessageOptions) ([]SearchMessageResult, error)
	ClearMessages(ctx context.Context, id string, opts MessageQueryOptions) error

	// Session summaries
	SaveSummary(ctx context.Context, summary SessionSummary) error
	GetSummary(ctx context.Context, sessionID string) (SessionSummary, bool, error)
	DeleteSummary(ctx context.Context, sessionID string) error
}

// Store defines the aggregate durable state store.
type Store interface {
	Session() SessionStore
}
