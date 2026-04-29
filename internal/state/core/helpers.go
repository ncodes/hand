package core

import (
	"errors"
	"strings"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/pkg/nanoid"
)

func ValidateSessionID(id string) error {
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

func ValidateArchiveID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("archive id is required")
	}

	if !strings.HasPrefix(id, ArchiveIDPrefix) || nanoid.ValidateID(id) != nil {
		return errors.New("archive id must be a valid arc_ nanoid")
	}

	return nil
}

func NormalizeCreateArchive(archive ArchivedSession) (ArchivedSession, error) {
	archive.ID = strings.TrimSpace(archive.ID)
	if err := ValidateArchiveID(archive.ID); err != nil {
		return ArchivedSession{}, err
	}

	archive.SourceSessionID = strings.TrimSpace(archive.SourceSessionID)
	if err := ValidateSessionID(archive.SourceSessionID); err != nil {
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

func CloneMessages(messages []handmsg.Message) []handmsg.Message {
	return handmsg.CloneMessages(messages)
}

func NormalizeSessionSummary(summary SessionSummary) (SessionSummary, error) {
	summary.SessionID = strings.TrimSpace(summary.SessionID)
	if err := ValidateSessionID(summary.SessionID); err != nil {
		if err.Error() == "session id is required" {
			return SessionSummary{}, errors.New("session id is required")
		}

		return SessionSummary{}, err
	}

	summary.SessionSummary = strings.TrimSpace(summary.SessionSummary)
	if summary.SessionSummary == "" {
		return SessionSummary{}, errors.New("session summary is required")
	}

	if summary.SourceEndOffset < 0 {
		return SessionSummary{}, errors.New("summary source end offset must be greater than or equal to zero")
	}

	if summary.SourceMessageCount < 0 {
		return SessionSummary{}, errors.New("summary source message count must be greater than or equal to zero")
	}

	if summary.SourceEndOffset > summary.SourceMessageCount {
		return SessionSummary{}, errors.New("summary source end offset cannot exceed source message count")
	}

	if summary.UpdatedAt.IsZero() {
		summary.UpdatedAt = time.Now().UTC()
	} else {
		summary.UpdatedAt = summary.UpdatedAt.UTC()
	}

	summary.CurrentTask = strings.TrimSpace(summary.CurrentTask)
	summary.Discoveries = cloneStrings(summary.Discoveries)
	summary.OpenQuestions = cloneStrings(summary.OpenQuestions)
	summary.NextActions = cloneStrings(summary.NextActions)

	return summary, nil
}

func CloneSessionSummary(summary SessionSummary) SessionSummary {
	cloned := summary
	cloned.Discoveries = cloneStrings(summary.Discoveries)
	cloned.OpenQuestions = cloneStrings(summary.OpenQuestions)
	cloned.NextActions = cloneStrings(summary.NextActions)
	return cloned
}

// UniqueStrings trims, de-duplicates, and preserves the first occurrence order of strings.
func UniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}

	return unique
}

// NormalizeMatchValue canonicalizes role, tool, and filter values before comparison.
func NormalizeMatchValue(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	cloned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		cloned = append(cloned, value)
	}

	if len(cloned) == 0 {
		return nil
	}

	return cloned
}
