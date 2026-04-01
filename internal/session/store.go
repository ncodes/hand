package session

import (
	"errors"
	"strings"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/storage"
	"github.com/wandxy/hand/pkg/nanoid"
)

const DefaultSessionID = "default"
const SessionIDPrefix = "ses_"

type Session = storage.Session
type ArchivedSession = storage.ArchivedSession
type MessageQueryOptions = storage.MessageQueryOptions

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

func cloneMessages(messages []handmsg.Message) []handmsg.Message {
	return handmsg.CloneMessages(messages)
}
