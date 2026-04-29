package core

import (
	"context"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
)

type Store interface {
	// Session management
	Save(ctx context.Context, session Session) error
	Get(ctx context.Context, id string) (Session, bool, error)
	List(ctx context.Context) ([]Session, error)
	Delete(ctx context.Context, id string) error

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

	// Archive management
	CreateArchive(ctx context.Context, archive ArchivedSession) error
	GetArchive(ctx context.Context, id string) (ArchivedSession, bool, error)
	ListArchives(ctx context.Context, sourceSessionID string) ([]ArchivedSession, error)
	DeleteArchive(ctx context.Context, archiveID string) error
	DeleteExpiredArchives(ctx context.Context, now time.Time) error
}
