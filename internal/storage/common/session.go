package common

import (
	"errors"
	"strings"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/storage"
	"github.com/wandxy/hand/pkg/nanoid"
)

func ValidateSessionID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("session id is required")
	}

	if id == storage.DefaultSessionID {
		return nil
	}

	if !strings.HasPrefix(id, storage.SessionIDPrefix) || nanoid.ValidateID(id) != nil {
		return errors.New("session id must be a valid ses_ nanoid")
	}

	return nil
}

func NormalizeCreateArchive(archive storage.ArchivedSession) (storage.ArchivedSession, error) {
	archive.ID = strings.TrimSpace(archive.ID)
	if archive.ID == "" {
		return storage.ArchivedSession{}, errors.New("archive id is required")
	}

	archive.SourceSessionID = strings.TrimSpace(archive.SourceSessionID)
	if err := ValidateSessionID(archive.SourceSessionID); err != nil {
		if err.Error() == "session id is required" {
			return storage.ArchivedSession{}, errors.New("source session id is required")
		}

		return storage.ArchivedSession{}, err
	}

	if archive.ArchivedAt.IsZero() {
		archive.ArchivedAt = time.Now().UTC()
	} else {
		archive.ArchivedAt = archive.ArchivedAt.UTC()
	}

	if archive.ExpiresAt.IsZero() {
		return storage.ArchivedSession{}, errors.New("archive expiry is required")
	}
	archive.ExpiresAt = archive.ExpiresAt.UTC()

	return archive, nil
}

func CloneMessages(messages []handmsg.Message) []handmsg.Message {
	return handmsg.CloneMessages(messages)
}
