package storage

import (
	"context"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/pkg/nanoid"
)

const DefaultSessionID = "default"
const SessionIDPrefix = "ses_"

func NewSessionID() (string, error) {
	return nanoid.Generate(SessionIDPrefix)
}

type Session struct {
	CreatedAt        time.Time
	ID               string
	LastPromptTokens int
	UpdatedAt        time.Time
}

type ArchivedSession struct {
	ID              string
	SourceSessionID string
	ArchivedAt      time.Time
	ExpiresAt       time.Time
}

type MessageQueryOptions struct {
	Archived bool
}

type SessionStore interface {
	Save(ctx context.Context, session Session) error
	Get(ctx context.Context, id string) (Session, bool, error)
	List(ctx context.Context) ([]Session, error)
	Delete(ctx context.Context, id string) error
	SetCurrent(ctx context.Context, id string) error
	Current(ctx context.Context) (string, bool, error)
	AppendMessages(ctx context.Context, id string, messages []handmsg.Message) error
	GetMessage(ctx context.Context, id string, index int, opts MessageQueryOptions) (handmsg.Message, bool, error)
	GetMessages(ctx context.Context, id string, opts MessageQueryOptions) ([]handmsg.Message, error)
	ClearMessages(ctx context.Context, id string, opts MessageQueryOptions) error
	CreateArchive(ctx context.Context, archive ArchivedSession) error
	GetArchive(ctx context.Context, id string) (ArchivedSession, bool, error)
	ListArchives(ctx context.Context, sourceSessionID string) ([]ArchivedSession, error)
	DeleteArchive(ctx context.Context, archiveID string) error
	DeleteExpiredArchives(ctx context.Context, now time.Time) error
}
