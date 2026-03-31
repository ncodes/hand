package session

import (
	"context"
	"errors"
	"strings"
	"time"

	handctx "github.com/wandxy/hand/internal/context"
	"github.com/wandxy/hand/pkg/nanoid"
)

const DefaultSessionID = "default"
const SessionIDPrefix = "ses_"

type Session struct {
	CreatedAt time.Time
	ID        string
	UpdatedAt time.Time
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

type Store interface {
	// Save persists session metadata for a session id.
	// Callers should treat this as a metadata write for session existence and timestamps,
	// not as a message write. Implementations may insert or update an existing session row.
	Save(ctx context.Context, session Session) error

	// Get returns session metadata for a session id.
	// It does not return session messages. Callers must use GetMessage or GetMessages for payloads.
	Get(ctx context.Context, id string) (Session, bool, error)

	// List returns session metadata ordered by most recent update first.
	// It does not include session messages.
	List(ctx context.Context) ([]Session, error)

	// Delete removes session metadata and live session messages for a session id.
	// Implementations must reject deletion of the default session.
	Delete(ctx context.Context, id string) error

	// SetCurrent stores the selected live session id.
	// Implementations should reject unknown session ids.
	SetCurrent(ctx context.Context, id string) error

	// Current returns the selected live session id when one has been stored.
	Current(ctx context.Context) (string, bool, error)

	// AppendMessages appends messages to a live session in order.
	// Callers must save the session first. Implementations should preserve append order.
	AppendMessages(ctx context.Context, id string, messages []handctx.Message) error

	// GetMessage returns a single message by zero-based index from either the live session
	// or an archive, depending on MessageQueryOptions.
	GetMessage(ctx context.Context, id string, index int, opts MessageQueryOptions) (handctx.Message, bool, error)

	// GetMessages returns all messages for either the live session or an archive,
	// depending on MessageQueryOptions.
	GetMessages(ctx context.Context, id string, opts MessageQueryOptions) ([]handctx.Message, error)

	// ClearMessages removes all messages from either the live session or an archive,
	// depending on MessageQueryOptions. It does not delete the session or archive metadata.
	ClearMessages(ctx context.Context, id string, opts MessageQueryOptions) error

	// CreateArchive persists archive metadata and snapshots messages from the source session.
	// Callers provide archive metadata only; implementations are responsible for materializing
	// archive message contents from the referenced session. Archive creation always clears the
	// source session's live messages after the snapshot is created successfully. When the source
	// session is not the default session, it also removes the live session metadata.
	CreateArchive(ctx context.Context, archive ArchivedSession) error

	// GetArchive returns archive metadata for a single archive id.
	// It does not return archived messages.
	GetArchive(ctx context.Context, id string) (ArchivedSession, bool, error)

	// ListArchives returns archive metadata, optionally filtered by source session id.
	// It does not return archived messages.
	ListArchives(ctx context.Context, sourceSessionID string) ([]ArchivedSession, error)

	// DeleteArchives removes archive metadata and archived messages for a single archive id.
	DeleteArchives(ctx context.Context, archiveID string) error

	// DeleteExpiredArchives removes archive metadata and archived messages whose expiry is
	// at or before the provided time.
	DeleteExpiredArchives(ctx context.Context, now time.Time) error
}

func NewSessionID() (string, error) {
	return nanoid.Generate(SessionIDPrefix)
}

func validateSessionID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("session id is required")
	}
	if id == DefaultSessionID {
		return nil
	}
	if !strings.HasPrefix(id, SessionIDPrefix) || nanoid.ValidateID(id) != nil {
		return errors.New("session id must be a valid ses_ nanoid")
	}
	return nil
}

func normalizeSession(session Session) (Session, error) {
	session.ID = strings.TrimSpace(session.ID)
	if err := validateSessionID(session.ID); err != nil {
		return Session{}, err
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now().UTC()
	} else {
		session.CreatedAt = session.CreatedAt.UTC()
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = time.Now().UTC()
	} else {
		session.UpdatedAt = session.UpdatedAt.UTC()
	}
	return session, nil
}

func normalizeCreateArchive(archive ArchivedSession) (ArchivedSession, error) {
	archive.ID = strings.TrimSpace(archive.ID)
	if archive.ID == "" {
		return ArchivedSession{}, errors.New("archive id is required")
	}
	archive.SourceSessionID = strings.TrimSpace(archive.SourceSessionID)
	if err := validateSessionID(archive.SourceSessionID); err != nil {
		if err.Error() == "session id is required" {
			return ArchivedSession{}, errors.New("source session id is required")
		}
		return ArchivedSession{}, err
	}
	if archive.ArchivedAt.IsZero() {
		archive.ArchivedAt = time.Now().UTC()
	} else {
		archive.ArchivedAt = archive.ArchivedAt.UTC()
	}
	if archive.ExpiresAt.IsZero() {
		return ArchivedSession{}, errors.New("archive expiry is required")
	}
	archive.ExpiresAt = archive.ExpiresAt.UTC()
	return archive, nil
}

func cloneSession(session Session) Session {
	return session
}

func cloneCreateArchive(archive ArchivedSession) ArchivedSession {
	return archive
}

func cloneMessages(messages []handctx.Message) []handctx.Message {
	if len(messages) == 0 {
		return nil
	}
	out := make([]handctx.Message, len(messages))
	copy(out, messages)
	return out
}
