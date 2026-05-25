package agent

import (
	agentsession "github.com/wandxy/hand/pkg/agent/session"

	storage "github.com/wandxy/hand/internal/state/core"
)

func agentSessionFromStorageSession(value storage.Session) agentsession.Session {
	return agentsession.Session{
		CreatedAt:                  value.CreatedAt,
		Compaction:                 agentCompactionFromStorageCompaction(value.Compaction),
		ID:                         value.ID,
		EpisodicCheckpointOffset:   value.EpisodicCheckpointOffset,
		LastPromptTokens:           value.LastPromptTokens,
		ReflectionCheckpointOffset: value.ReflectionCheckpointOffset,
		Title:                      value.Title,
		TitleSource:                value.TitleSource,
		UpdatedAt:                  value.UpdatedAt,
	}
}

func agentCompactionFromStorageCompaction(value storage.SessionCompaction) agentsession.Compaction {
	return agentsession.Compaction{
		CompletedAt:        value.CompletedAt,
		FailedAt:           value.FailedAt,
		LastError:          value.LastError,
		RequestedAt:        value.RequestedAt,
		StartedAt:          value.StartedAt,
		Status:             agentsession.CompactionStatus(value.Status),
		TargetMessageCount: value.TargetMessageCount,
		TargetOffset:       value.TargetOffset,
	}
}
