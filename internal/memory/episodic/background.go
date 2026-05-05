package episodic

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	storage "github.com/wandxy/hand/internal/state/core"
	"github.com/wandxy/hand/internal/trace"
)

const (
	DefaultBackgroundInterval    = 1 * time.Minute
	DefaultBackgroundIdleAfter   = 10 * time.Minute
	DefaultBackgroundMinMessages = 2
	DefaultBackgroundMaxRetries  = 1
	backgroundTrigger            = "background"
)

type BackgroundOptions struct {
	Enabled         bool
	Interval        time.Duration
	IdleAfter       time.Duration
	MinMessages     int
	WindowSize      int
	MaxWindows      int
	MaxWindowChars  int
	MaxWindowTokens int
	MaxRetries      int
}

type BackgroundRequest struct {
	Options BackgroundOptions
	RunID   string
	Trace   TraceRecorder
}

type BackgroundResult struct {
	RunID        string                    `json:"run_id"`
	Sessions     []BackgroundSessionResult `json:"sessions,omitempty"`
	CheckedCount int                       `json:"checked_count"`
	Eligible     int                       `json:"eligible"`
	WriteCount   int                       `json:"write_count"`
	SkipCount    int                       `json:"skip_count"`
	FailureCount int                       `json:"failure_count"`
	RetryCount   int                       `json:"retry_count"`
}

type BackgroundSessionResult struct {
	SessionID    string `json:"session_id"`
	MessageCount int    `json:"message_count"`
	Eligible     bool   `json:"eligible"`
	Reason       string `json:"reason,omitempty"`
	Extraction   Result `json:"extraction,omitempty"`
	RetryCount   int    `json:"retry_count"`
	Error        string `json:"error,omitempty"`
}

type BackgroundStateManager interface {
	StateManager
	ListSessions(context.Context) ([]storage.Session, error)
}

func NormalizeBackgroundOptions(opts BackgroundOptions) BackgroundOptions {
	if opts.Interval <= 0 {
		opts.Interval = DefaultBackgroundInterval
	}
	if opts.IdleAfter <= 0 {
		opts.IdleAfter = DefaultBackgroundIdleAfter
	}
	if opts.MinMessages <= 0 {
		opts.MinMessages = DefaultBackgroundMinMessages
	}
	if opts.WindowSize <= 0 {
		opts.WindowSize = DefaultWindowSize
	}
	if opts.WindowSize > MaxWindowSize {
		opts.WindowSize = MaxWindowSize
	}
	if opts.MaxWindows <= 0 {
		opts.MaxWindows = DefaultMaxWindows
	}
	if opts.MaxWindows > MaxWindows {
		opts.MaxWindows = MaxWindows
	}
	if opts.MaxWindowChars <= 0 {
		opts.MaxWindowChars = DefaultMaxWindowChars
	}
	if opts.MaxWindowChars > MaxWindowChars {
		opts.MaxWindowChars = MaxWindowChars
	}
	if opts.MaxWindowTokens <= 0 {
		opts.MaxWindowTokens = DefaultMaxWindowTokens
	}
	if opts.MaxWindowTokens > MaxWindowTokens {
		opts.MaxWindowTokens = MaxWindowTokens
	}
	if opts.MaxRetries < 0 {
		opts.MaxRetries = 0
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = DefaultBackgroundMaxRetries
	}
	return opts
}

func (s *Service) RunBackground(ctx context.Context, req BackgroundRequest) (BackgroundResult, error) {
	started := s.now()
	if s == nil || s.manager == nil {
		return BackgroundResult{}, errors.New("state manager is required")
	}
	if s.memory == nil {
		return BackgroundResult{}, errors.New("memory repository is required")
	}
	manager, ok := s.manager.(BackgroundStateManager)
	if !ok {
		return BackgroundResult{}, errors.New("session listing is required")
	}

	opts := NormalizeBackgroundOptions(req.Options)
	runID := strings.TrimSpace(req.RunID)
	if runID == "" {
		runID = backgroundRunID(started)
	}
	result := BackgroundResult{RunID: runID}

	traceFields := map[string]any{"interval_ms": opts.Interval.Milliseconds(),
		"idle_after_ms": opts.IdleAfter.Milliseconds(), "min_messages": opts.MinMessages}
	recordBackgroundTrace(req.Trace, trace.EvtMemoryEpisodicBackgroundScheduled, backgroundPayload(runID, "", 0, "", traceFields))
	logBackground("scheduled", runID, "", 0, "", traceFields)

	sessions, err := manager.ListSessions(ctx)
	if err != nil {
		recordBackgroundFailure(req.Trace, runID, "", 0, "", err)
		return BackgroundResult{}, err
	}

	for _, session := range sessions {
		sessionResult := s.runBackgroundForSession(ctx, req.Trace, runID, opts, session)
		result.Sessions = append(result.Sessions, sessionResult)
		result.CheckedCount++
		if sessionResult.Eligible {
			result.Eligible++
		}
		result.WriteCount += sessionResult.Extraction.WriteCount
		result.SkipCount += sessionResult.Extraction.SkipCount
		result.RetryCount += sessionResult.RetryCount
		if sessionResult.Error != "" {
			result.FailureCount++
		}
	}

	duration := s.now().Sub(started)
	traceFields = map[string]any{
		"checked_count":  result.CheckedCount,
		"eligible_count": result.Eligible,
		"write_count":    result.WriteCount,
		"skip_count":     result.SkipCount,
		"failure_count":  result.FailureCount,
		"retry_count":    result.RetryCount,
		"duration_ms":    duration.Milliseconds(),
	}
	recordBackgroundTrace(req.Trace, trace.EvtMemoryEpisodicBackgroundCompleted, backgroundPayload(runID, "", 0, "", traceFields))
	logBackground("completed", runID, "", 0, "", traceFields)

	return result, nil
}

func (s *Service) runBackgroundForSession(
	ctx context.Context,
	recorder TraceRecorder,
	runID string,
	opts BackgroundOptions,
	session storage.Session,
) BackgroundSessionResult {
	sessionID := strings.TrimSpace(session.ID)
	messageCount, err := s.manager.CountMessages(ctx, sessionID, storage.MessageQueryOptions{})
	if err != nil {
		recordBackgroundFailure(recorder, runID, sessionID, 0, "count_messages", err)
		return BackgroundSessionResult{SessionID: sessionID, Error: err.Error()}
	}

	startOffset := normalizedCheckpointOffset(session.EpisodicCheckpointOffset, messageCount)
	eligible, reason := isSessionEligible(s.now(), session, messageCount, startOffset, opts)

	fields := map[string]any{"eligible": eligible, "reason": reason, "episodic_checkpoint_offset": startOffset}
	recordBackgroundTrace(recorder, trace.EvtMemoryEpisodicBackgroundEligibilityChecked,
		backgroundPayload(runID, sessionID, messageCount, reason, fields))
	logBackground("eligibility_checked", runID, sessionID, messageCount, reason, fields)

	sessionResult := BackgroundSessionResult{
		SessionID:    sessionID,
		MessageCount: messageCount,
		Eligible:     eligible,
		Reason:       reason,
	}
	if !eligible {
		return sessionResult
	}

	req := Request{
		SessionID:       sessionID,
		OffsetStart:     &startOffset,
		WindowSize:      opts.WindowSize,
		MaxWindows:      opts.MaxWindows,
		MaxWindowChars:  opts.MaxWindowChars,
		MaxWindowTokens: opts.MaxWindowTokens,
		Trigger:         backgroundTrigger,
		Trace:           recorder,
	}

	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			sessionResult.RetryCount++
			traceFields := map[string]any{"retry_count": sessionResult.RetryCount}
			recordBackgroundTrace(recorder, trace.EvtMemoryEpisodicBackgroundRetry, backgroundPayload(runID, sessionID, messageCount, "retry", traceFields))
			logBackground("retry", runID, sessionID, messageCount, "retry", traceFields)
		}

		traceFields := map[string]any{"attempt": attempt + 1}
		recordBackgroundTrace(recorder, trace.EvtMemoryEpisodicBackgroundExtractionAttempt, backgroundPayload(runID, sessionID, messageCount, "eligible", traceFields))
		logBackground("extraction_attempt", runID, sessionID, messageCount, "eligible", traceFields)

		extraction, err := s.Extract(ctx, req)
		if err == nil {
			sessionResult.Extraction = extraction
			for _, window := range extraction.Windows {
				fields := map[string]any{"offset_start": window.OffsetStart, "offset_end": window.OffsetEnd, "write_count": window.WriteCount, "skip_count": window.SkipCount}
				recordBackgroundTrace(recorder, trace.EvtMemoryEpisodicBackgroundWindowCheckpoint, backgroundPayload(runID, sessionID, messageCount, "processed", fields))
				logBackground("window_checkpoint", runID, sessionID, messageCount, "processed", fields)
			}
			return sessionResult
		}

		sessionResult.Error = err.Error()
		if attempt >= opts.MaxRetries {
			recordBackgroundFailure(recorder, runID, sessionID, messageCount, "extract", err)
			return sessionResult
		}
	}
}

// isSessionEligible checks if a session is eligible for episodic memory extraction.
func isSessionEligible(
	now time.Time,
	session storage.Session,
	messageCount int,
	checkpointOffset int,
	opts BackgroundOptions) (bool, string) {
	if strings.TrimSpace(session.ID) == "" {
		return false, "missing_session_id"
	}
	if messageCount < opts.MinMessages {
		return false, "insufficient_messages"
	}
	if checkpointOffset >= messageCount {
		return false, "checkpoint_complete"
	}
	if session.UpdatedAt.IsZero() {
		return true, "eligible"
	}
	if session.UpdatedAt.Add(opts.IdleAfter).After(now.UTC()) {
		return false, "session_not_idle"
	}
	return true, "eligible"
}

// normalizedCheckpointOffset normalizes the checkpoint offset to ensure it is within the valid range.
func normalizedCheckpointOffset(offset int, messageCount int) int {
	if offset < 0 {
		return 0
	}
	if offset > messageCount {
		return messageCount
	}
	return offset
}

func backgroundRunID(now time.Time) string {
	return "memory_bg_" + strconv.FormatInt(now.UTC().UnixNano(), 10)
}
