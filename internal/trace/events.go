package trace

import (
	"slices"
	"strings"
)

const (
	EvtChatStarted                                = "chat.started"
	EvtSessionFailed                              = "session.failed"
	EvtUserMessageAccepted                        = "user.message.accepted"
	EvtModelRequest                               = "model.request"
	EvtModelResponse                              = "model.response"
	EvtFinalAssistantResponse                     = "final.assistant.response"
	EvtToolInvocationStarted                      = "tool.invocation.started"
	EvtToolInvocationCompleted                    = "tool.invocation.completed"
	EvtSummaryFallbackStarted                     = "summary.fallback.started"
	EvtContextPreflight                           = "context.preflight"
	EvtContextPostflightUsage                     = "context.postflight.usage_recorded"
	EvtContextCompactionTriggered                 = "context.compaction.triggered"
	EvtContextCompactionWarning                   = "context.compaction.warning"
	EvtContextCompactionPending                   = "context.compaction.pending"
	EvtContextCompactionRunning                   = "context.compaction.running"
	EvtContextCompactionSucceeded                 = "context.compaction.succeeded"
	EvtContextCompactionFailed                    = "context.compaction.failed"
	EvtSummaryRequested                           = "context.summary.requested"
	EvtSummarySaved                               = "context.summary.saved"
	EvtSummaryFailed                              = "context.summary.failed"
	EvtSummaryParseFailed                         = "context.summary.parse_failed"
	EvtSummaryApplied                             = "context.summary.applied"
	EvtRecallSummaryRequested                     = "context.recall_summary.requested"
	EvtRecallSummarySaved                         = "context.recall_summary.saved"
	EvtRecallSummaryFailed                        = "context.recall_summary.failed"
	EvtMemoryRetrievalStarted                     = "memory.search.started"
	EvtMemoryRetrieved                            = "memory.retrieved"
	EvtMemoryRetrievalFailed                      = "memory.search.failed"
	EvtMemoryFlushStarted                         = "memory.flush.started"
	EvtMemoryFlushModelRequested                  = "memory.flush.model_requested"
	EvtMemoryFlushWriteRequested                  = "memory.flush.write_requested"
	EvtMemoryFlushSkipped                         = "memory.flush.skipped"
	EvtMemoryFlushFailed                          = "memory.flush.failed"
	EvtMemoryFlushTimeout                         = "memory.flush.timeout"
	EvtMemoryFlushCompleted                       = "memory.flush.completed"
	EvtMemoryExtractionStarted                    = "memory.extraction.started"
	EvtMemoryExtractionWindowLoaded               = "memory.extraction.window_loaded"
	EvtMemoryExtractionExtractorRequested         = "memory.extraction.extractor_requested"
	EvtMemoryExtractionCandidates                 = "memory.extraction.candidates"
	EvtMemoryExtractionCandidateGenerated         = "memory.extraction.candidate_generated"
	EvtMemoryExtractionCandidateRejected          = "memory.extraction.candidate_rejected"
	EvtMemoryExtractionConfidenceScored           = "memory.extraction.confidence_scored"
	EvtMemoryExtractionAdmissionHandoff           = "memory.extraction.admission_handoff"
	EvtMemoryExtractionMemoryWritten              = "memory.extraction.memory_written"
	EvtMemoryExtractionDuplicateSkipped           = "memory.extraction.duplicate_skipped"
	EvtMemoryExtractionFailed                     = "memory.extraction.failed"
	EvtMemoryExtractionCompleted                  = "memory.extraction.completed"
	EvtMemoryEpisodicBackgroundScheduled          = "memory.episodic_background.scheduled"
	EvtMemoryEpisodicBackgroundEligibilityChecked = "memory.episodic_background.eligibility_checked"
	EvtMemoryEpisodicBackgroundWindowCheckpoint   = "memory.episodic_background.window_checkpoint"
	EvtMemoryEpisodicBackgroundExtractionAttempt  = "memory.episodic_background.extraction_attempt"
	EvtMemoryEpisodicBackgroundRetry              = "memory.episodic_background.retry"
	EvtMemoryEpisodicBackgroundFailed             = "memory.episodic_background.failed"
	EvtMemoryEpisodicBackgroundCompleted          = "memory.episodic_background.completed"
	EvtMemoryReflectionStarted                    = "memory.reflection.started"
	EvtMemoryReflectionSourceLoaded               = "memory.reflection.source_loaded"
	EvtMemoryReflectionRelatedLoaded              = "memory.reflection.related_loaded"
	EvtMemoryReflectionCandidateGenerated         = "memory.reflection.candidate_generated"
	EvtMemoryReflectionCandidateRejected          = "memory.reflection.candidate_rejected"
	EvtMemoryReflectionMemoryWritten              = "memory.reflection.memory_written"
	EvtMemoryReflectionFailed                     = "memory.reflection.failed"
	EvtMemoryReflectionCompleted                  = "memory.reflection.completed"
	EvtMemoryPromotionStarted                     = "memory.promotion.started"
	EvtMemoryPromotionDecision                    = "memory.promotion.decision"
	EvtMemoryPromotionCompleted                   = "memory.promotion.completed"
	EvtMemoryPromotionFailed                      = "memory.promotion.failed"
	EvtMemoryPromotionFallback                    = "memory.promotion.fallback"
	EvtWorkspaceRulesTruncated                    = "workspace.rules.truncated"
	EvtPlanUpdated                                = "plan.updated"
	EvtPlanCleared                                = "plan.cleared"
	EvtPlanHydrated                               = "plan.hydrated"
)

var episodicMemoryTraceEventTypes = []string{
	EvtSessionFailed,
	EvtToolInvocationStarted,
	EvtToolInvocationCompleted,
	EvtContextCompactionTriggered,
	EvtContextCompactionWarning,
	EvtContextCompactionPending,
	EvtContextCompactionRunning,
	EvtContextCompactionSucceeded,
	EvtContextCompactionFailed,
	EvtSummaryFallbackStarted,
	EvtSummaryRequested,
	EvtSummarySaved,
	EvtSummaryFailed,
	EvtSummaryParseFailed,
	EvtSummaryApplied,
	EvtMemoryFlushStarted,
	EvtMemoryFlushModelRequested,
	EvtMemoryFlushWriteRequested,
	EvtMemoryFlushSkipped,
	EvtMemoryFlushFailed,
	EvtMemoryFlushTimeout,
	EvtMemoryFlushCompleted,
	EvtWorkspaceRulesTruncated,
	EvtPlanUpdated,
	EvtPlanCleared,
	EvtPlanHydrated,
}

func EpisodicMemoryTraceEventTypes() []string {
	return append([]string(nil), episodicMemoryTraceEventTypes...)
}

func IsEpisodicMemoryTraceEventType(eventType string) bool {
	eventType = strings.TrimSpace(eventType)
	return slices.Contains(episodicMemoryTraceEventTypes, eventType)
}
