package state

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/state/retrieval"
)

const (
	DefaultHybridRetrievalCandidateLimit = 100
	MaxHybridRetrievalCandidateLimit     = 1000
	DefaultRerankCandidateLimit          = 100
	ReciprocalRankFusionConstant         = 60
)

// MessageIndexRow is one searchable text row derived from a session message.
type MessageIndexRow struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	MessageID uint
	SessionID string
	Role      string
	ToolName  string
	Body      string
}

// VectorInput is the normalized embedding input for one searchable message row.
type VectorInput struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	ID        string
	SourceID  string
	SessionID string
	Role      string
	ToolName  string
	Text      string
}

// VectorStoreOptions configures vector indexing, retrieval, and reranking for a store.
type VectorStoreOptions struct {
	Embedder            retrieval.Embedder
	Reranker            retrieval.Reranker
	VectorStore         retrieval.VectorStore
	EnableRerank        *bool
	EmbeddingModel      string
	RebuildBatchSize    int
	RerankMaxCandidates int
	Diagnostics         bool
	Required            bool
}

// VectorConfig is the normalized vector configuration kept by concrete stores.
type VectorConfig struct {
	Provider    retrieval.Embedder
	Reranker    retrieval.Reranker
	Store       retrieval.VectorStore
	Model       string
	RerankMax   int
	Diagnostics bool
	Rerank      bool
	Required    bool
}

// CandidateMatch stores ranking metadata for a lexical, vector, or reranked search candidate.
type CandidateMatch struct {
	SessionID       string
	MatchedText     string
	MatchedToolName string
	LexicalScore    float64
	RerankScore     float64
	VectorScore     float64
	FusedScore      float64
	LexicalRank     int
	VectorRank      int
	HasLexical      bool
	HasRerank       bool
	HasVector       bool
}

// SearchCandidate exposes mutable ranking metadata for shared candidate set helpers.
type SearchCandidate interface {
	CandidateMatchRef() *CandidateMatch
}

// SearchCandidateSet stores candidates keyed by their stable source identifier.
type SearchCandidateSet[K comparable, C SearchCandidate] map[K]C

// Merge folds vector candidates into the set, preserving lexical metadata when present.
func (candidates SearchCandidateSet[K, C]) Merge(vectorCandidates []C, keyForCandidate func(C) K) {
	for _, vectorCandidate := range vectorCandidates {
		vectorMatch := vectorCandidate.CandidateMatchRef()
		if vectorMatch == nil {
			continue
		}

		key := keyForCandidate(vectorCandidate)
		candidate, ok := candidates[key]
		if !ok {
			candidates[key] = vectorCandidate
			continue
		}

		match := candidate.CandidateMatchRef()
		if match == nil {
			continue
		}

		match.VectorScore = vectorMatch.VectorScore
		match.VectorRank = vectorMatch.VectorRank
		match.HasVector = true
		if strings.TrimSpace(match.MatchedText) == "" {
			match.MatchedText = vectorMatch.MatchedText
			match.MatchedToolName = vectorMatch.MatchedToolName
		}
	}
}

// Sorted computes fused scores and returns candidates ordered by the supplied comparator.
func (candidates SearchCandidateSet[K, C]) Sorted(less func(C, C) bool) []C {
	items := make([]C, 0, len(candidates))
	for _, candidate := range candidates {
		match := candidate.CandidateMatchRef()
		if match == nil {
			continue
		}

		match.FusedScore = FusedCandidateScore(
			match.HasLexical,
			match.LexicalRank,
			match.HasVector,
			match.VectorRank,
		)
		items = append(items, candidate)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return less(items[i], items[j])
	})

	return items
}

// MessageIndexRowsFromMessage converts a message into the text rows used by lexical and vector search.
func MessageIndexRowsFromMessage(sessionID string, message handmsg.Message) []MessageIndexRow {
	baseRow := MessageIndexRow{
		CreatedAt: message.CreatedAt,
		UpdatedAt: message.CreatedAt,
		MessageID: message.ID,
		SessionID: strings.TrimSpace(sessionID),
		Role:      NormalizeMatchValue(string(message.Role)),
	}

	body := strings.TrimSpace(message.Content)
	switch message.Role {
	case handmsg.RoleAssistant:
		rows := make([]MessageIndexRow, 0, len(message.ToolCalls)+1)
		if body != "" {
			row := baseRow
			row.Body = body
			rows = append(rows, row)
		}
		for _, toolCall := range message.ToolCalls {
			toolBody := strings.TrimSpace(handmsg.ToolCallSearchText(toolCall))
			if toolBody == "" {
				continue
			}
			row := baseRow
			row.ToolName = NormalizeMatchValue(toolCall.Name)
			row.Body = toolBody
			rows = append(rows, row)
		}
		if len(rows) == 0 {
			return nil
		}
		return rows
	case handmsg.RoleTool:
		if body == "" {
			return nil
		}
		row := baseRow
		row.ToolName = NormalizeMatchValue(message.Name)
		row.Body = body
		return []MessageIndexRow{row}
	default:
		if body == "" {
			return nil
		}
		row := baseRow
		row.Body = body
		return []MessageIndexRow{row}
	}
}

// VectorInputsFromIndexRows converts searchable rows into stable embedding inputs.
func VectorInputsFromIndexRows(rows []MessageIndexRow) []VectorInput {
	if len(rows) == 0 {
		return nil
	}

	countsByMessageID := make(map[uint]int, len(rows))
	inputs := make([]VectorInput, 0, len(rows))
	for _, row := range rows {
		sourceID := SourceIDForMessage(row.SessionID, row.MessageID)
		countsByMessageID[row.MessageID]++
		inputs = append(inputs, VectorInput{
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
			ID:        fmt.Sprintf("%s:row:%d", sourceID, countsByMessageID[row.MessageID]),
			SourceID:  sourceID,
			SessionID: row.SessionID,
			Role:      row.Role,
			ToolName:  row.ToolName,
			Text:      row.Body,
		})
	}

	return inputs
}

// MessageIndexRowForVectorRecord returns the searchable row represented by a vector record ID.
func MessageIndexRowForVectorRecord(rows []MessageIndexRow, vectorID string) (MessageIndexRow, bool) {
	if len(rows) == 0 {
		return MessageIndexRow{}, false
	}

	idx := strings.LastIndex(vectorID, ":row:")
	if idx < 0 {
		return MessageIndexRow{}, false
	}
	rowNumber, err := strconv.Atoi(vectorID[idx+5:])
	if err != nil || rowNumber <= 0 || rowNumber > len(rows) {
		return MessageIndexRow{}, false
	}

	return rows[rowNumber-1], true
}

// MessageIndexRowMatchesSearchOptions reports whether a row satisfies non-text search filters.
func MessageIndexRowMatchesSearchOptions(row MessageIndexRow, opts SearchMessageOptions) bool {
	if toolName := NormalizeMatchValue(opts.ToolName); toolName != "" && row.ToolName != toolName {
		return false
	}

	return true
}

// SourceIDsFromMessages returns stable vector source IDs for the supplied messages.
func SourceIDsFromMessages(sessionID string, messages []handmsg.Message) []string {
	if len(messages) == 0 {
		return nil
	}

	sourceIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		sourceIDs = append(sourceIDs, SourceIDForMessage(sessionID, message.ID))
	}

	return sourceIDs
}

// SourceIDForMessage returns the stable vector source ID for a session message.
func SourceIDForMessage(sessionID string, messageID uint) string {
	return retrieval.StableSessionMessageID(strings.TrimSpace(sessionID), messageID)
}

// MessageRefFromSourceID parses a session ID and message ID from a vector source ID.
func MessageRefFromSourceID(sourceID string) (string, uint, bool) {
	value, ok := strings.CutPrefix(sourceID, string(retrieval.SourceKindSessionMessage)+":")
	if !ok {
		return "", 0, false
	}
	idx := strings.LastIndex(value, ":")
	if idx <= 0 || idx == len(value)-1 {
		return "", 0, false
	}
	messageID, err := strconv.ParseUint(value[idx+1:], 10, 64)
	if err != nil || messageID == 0 {
		return "", 0, false
	}

	return value[:idx], uint(messageID), true
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

// HybridRetrievalCandidateLimit returns the lexical/vector candidate limit for a search request.
func HybridRetrievalCandidateLimit(opts SearchMessageOptions) int {
	limit := DefaultHybridRetrievalCandidateLimit
	if opts.MaxSessions > 0 && opts.MaxMessagesPerSession > 0 {
		limit = max(limit, opts.MaxSessions*opts.MaxMessagesPerSession)
	}
	if limit > MaxHybridRetrievalCandidateLimit {
		return MaxHybridRetrievalCandidateLimit
	}

	return limit
}

// FusedCandidateScore combines lexical and vector ranks with reciprocal rank fusion.
func FusedCandidateScore(
	hasLexical bool,
	lexicalRank int,
	hasVector bool,
	vectorRank int,
) float64 {
	var score float64
	if hasLexical && lexicalRank > 0 {
		score += 1 / float64(ReciprocalRankFusionConstant+lexicalRank)
	}
	if hasVector && vectorRank > 0 {
		score += 1 / float64(ReciprocalRankFusionConstant+vectorRank)
	}

	return score
}

// CandidateRankingScore returns the score used for final candidate ordering.
func CandidateRankingScore(hasRerank bool, rerankScore float64, fusedScore float64) float64 {
	if hasRerank {
		return rerankScore
	}

	return fusedScore
}

// CompareCandidateOrder orders candidates by score, recency, session ID, and message ID.
func CompareCandidateOrder(
	leftScore float64,
	rightScore float64,
	leftCreatedAt time.Time,
	rightCreatedAt time.Time,
	leftSessionID string,
	rightSessionID string,
	leftMessageID uint,
	rightMessageID uint,
) int {
	if leftScore != rightScore {
		if leftScore > rightScore {
			return -1
		}
		return 1
	}
	if !leftCreatedAt.Equal(rightCreatedAt) {
		if leftCreatedAt.After(rightCreatedAt) {
			return -1
		}
		return 1
	}
	if leftSessionID != rightSessionID {
		return strings.Compare(leftSessionID, rightSessionID)
	}
	if leftMessageID > rightMessageID {
		return -1
	}
	if leftMessageID < rightMessageID {
		return 1
	}

	return 0
}
