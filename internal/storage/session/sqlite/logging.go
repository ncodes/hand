package sqlite

import (
	"strings"

	"github.com/rs/zerolog"

	base "github.com/wandxy/hand/internal/storage/session"
	"github.com/wandxy/hand/pkg/logutils"
)

var sessionSearchLog = logutils.InitLogger("storage.session.sqlite")

func (s *SessionStore) logSearchEvent(eventName string, id string, opts base.SearchMessageOptions) *zerolog.Event {
	event := sessionSearchLog.Debug().
		Str("event", strings.TrimSpace(eventName)).
		Int("query_chars", len([]rune(strings.TrimSpace(opts.Query))))
	if id = strings.TrimSpace(id); id != "" {
		event = event.Str("session_id", id)
	}
	if opts.IgnoreSessionID != "" {
		event = event.Str("ignore_session_id", opts.IgnoreSessionID)
	}
	if opts.Role != "" {
		event = event.Str("role", strings.TrimSpace(string(opts.Role)))
	}
	if toolName := normalizeSearchValue(opts.ToolName); toolName != "" {
		event = event.Str("tool_name", toolName)
	}
	if opts.MaxSessions > 0 {
		event = event.Int("max_sessions", opts.MaxSessions)
	}
	if opts.MaxMessagesPerSession > 0 {
		event = event.Int("max_messages_per_session", opts.MaxMessagesPerSession)
	}

	return event
}

func (s *SessionStore) logVectorEvent(eventName string) *zerolog.Event {
	return sessionSearchLog.Debug().Str("event", strings.TrimSpace(eventName))
}

func (s *SessionStore) logCandidateDiagnostics(stage string, candidates []*searchCandidate) {
	if !s.diagnosticsEnabled() {
		return
	}

	for rank, candidate := range candidates {
		event := sessionSearchLog.Debug().
			Str("event", "ranking diagnostic").
			Str("stage", strings.TrimSpace(stage)).
			Str("session_id", candidate.SessionID).
			Uint("message_id", candidate.ID).
			Float64("lexical_score", candidate.LexicalScore).
			Float64("vector_score", candidate.VectorScore).
			Float64("fused_score", candidate.FusedScore).
			Int("lexical_rank", candidate.LexicalRank).
			Int("vector_rank", candidate.VectorRank).
			Int("rank", rank+1)
		if candidate.HasRerank {
			event = event.Float64("rerank_score", candidate.RerankScore).Int("rerank_rank", rank+1)
		}
		if candidate.MatchedToolName != "" {
			event = event.Str("matched_tool_name", candidate.MatchedToolName)
		}
		event.Msg("session search ranking diagnostic")
	}
}
