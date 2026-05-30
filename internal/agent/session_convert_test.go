package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storage "github.com/wandxy/hand/internal/state/core"
	agentsession "github.com/wandxy/hand/pkg/agent/session"
)

func TestSessionConvert_ConvertsStorageSessionAndCompaction(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	session := agentSessionFromStorageSession(storage.Session{
		CreatedAt:                  now,
		Compaction:                 storage.SessionCompaction{Status: storage.CompactionStatusFailed, TargetOffset: 8, TargetMessageCount: 11, LastError: "failed"},
		ID:                         "session",
		EpisodicCheckpointOffset:   2,
		LastPromptTokens:           500,
		ReflectionCheckpointOffset: 3,
		Title:                      "Title",
		TitleSource:                storage.SessionTitleSourceGenerated,
		UpdatedAt:                  now.Add(time.Minute),
	})

	require.Equal(t, agentsession.Session{
		CreatedAt:                  now,
		Compaction:                 agentsession.Compaction{Status: agentsession.CompactionStatusFailed, TargetOffset: 8, TargetMessageCount: 11, LastError: "failed"},
		ID:                         "session",
		EpisodicCheckpointOffset:   2,
		LastPromptTokens:           500,
		ReflectionCheckpointOffset: 3,
		Title:                      "Title",
		TitleSource:                storage.SessionTitleSourceGenerated,
		UpdatedAt:                  now.Add(time.Minute),
	}, session)
}
