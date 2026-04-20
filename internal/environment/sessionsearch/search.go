package sessionsearch

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	handmsg "github.com/wandxy/hand/internal/messages"
	sessionstore "github.com/wandxy/hand/internal/session"
	"github.com/wandxy/hand/internal/storage"
)

const (
	defaultSessionSearchMaxResults = 10
	maxSessionSearchResults        = 20
	maxSessionMatchedMessages      = 3
	sessionSearchSnippetRunes      = 200
)

func Search(
	ctx context.Context,
	manager *sessionstore.Manager,
	req envtypes.SessionSearchRequest,
) ([]envtypes.SessionSearchResult, error) {
	if manager == nil {
		return nil, errors.New("session manager is required")
	}

	sessionID := strings.TrimSpace(req.SessionID)
	ignoreSessionID := strings.TrimSpace(req.IgnoreSessionID)
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, errors.New("query is required")
	}

	role := strings.TrimSpace(strings.ToLower(req.Role))
	toolName := strings.TrimSpace(strings.ToLower(req.ToolName))
	limit := clampSearchResults(req.MaxResults)

	hits, err := manager.SearchMessages(ctx, sessionID, storage.SearchMessageOptions{
		IgnoreSessionID: ignoreSessionID,
		Query:           query,
		Role:            handmsg.Role(role),
		ToolName:        toolName,
	})
	if err != nil {
		return nil, err
	}

	if len(hits) == 0 {
		return nil, nil
	}

	type groupedSessionResult struct {
		result        envtypes.SessionSearchResult
		lastMatchedAt time.Time
	}

	grouped := make(map[string]*groupedSessionResult)
	for _, hit := range hits {
		if strings.TrimSpace(hit.MatchedText) == "" {
			continue
		}

		group, ok := grouped[hit.SessionID]
		if !ok {
			session, found, err := manager.Get(ctx, hit.SessionID)
			if err != nil {
				return nil, err
			}
			if !found {
				continue
			}

			summary, _, err := manager.GetSummary(ctx, hit.SessionID)
			if err != nil {
				return nil, err
			}

			group = &groupedSessionResult{
				result: envtypes.SessionSearchResult{
					SessionID:      hit.SessionID,
					SessionCreated: formatSearchTime(session.CreatedAt),
					SessionUpdated: formatSearchTime(session.UpdatedAt),
					SessionSummary: strings.TrimSpace(summary.SessionSummary),
					Messages:       make([]envtypes.SessionSearchMessageHit, 0, maxSessionMatchedMessages),
				},
			}
			grouped[hit.SessionID] = group
		}

		group.result.MatchCount++
		if hit.Message.CreatedAt.After(group.lastMatchedAt) {
			group.lastMatchedAt = hit.Message.CreatedAt
		}

		if len(group.result.Messages) >= maxSessionMatchedMessages {
			continue
		}

		matchIndex, matchLen := caseInsensitiveMatchIndex(hit.MatchedText, query)
		snippet := snippetAround(hit.MatchedText, 0, 0, sessionSearchSnippetRunes)
		if matchIndex >= 0 {
			snippet = snippetAround(hit.MatchedText, matchIndex, matchLen, sessionSearchSnippetRunes)
		}

		group.result.Messages = append(group.result.Messages, envtypes.SessionSearchMessageHit{
			MessageID:     hit.Message.ID,
			Role:          string(hit.Message.Role),
			ToolName:      hit.MatchedToolName,
			CreatedAt:     formatSearchTime(hit.Message.CreatedAt),
			Snippet:       snippet,
			FullTextBytes: len(hit.MatchedText),
			MatchIndex:    matchIndex,
		})
	}

	results := make([]groupedSessionResult, 0, len(grouped))
	for _, group := range grouped {
		sort.Slice(group.result.Messages, func(i, j int) bool {
			if group.result.Messages[i].CreatedAt == group.result.Messages[j].CreatedAt {
				return group.result.Messages[i].MessageID > group.result.Messages[j].MessageID
			}
			return group.result.Messages[i].CreatedAt > group.result.Messages[j].CreatedAt
		})
		results = append(results, *group)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].lastMatchedAt.Equal(results[j].lastMatchedAt) {
			return results[i].result.SessionID < results[j].result.SessionID
		}
		return results[i].lastMatchedAt.After(results[j].lastMatchedAt)
	})

	if len(results) > limit {
		results = results[:limit]
	}

	groupedResults := make([]envtypes.SessionSearchResult, 0, len(results))
	for _, result := range results {
		groupedResults = append(groupedResults, result.result)
	}

	return groupedResults, nil
}

func formatSearchTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02T15:04:05Z07:00")
}

func clampSearchResults(value int) int {
	if value <= 0 {
		return defaultSessionSearchMaxResults
	}
	if value > maxSessionSearchResults {
		return maxSessionSearchResults
	}
	return value
}

func caseInsensitiveMatchIndex(text string, query string) (int, int) {
	query = strings.TrimSpace(query)
	if query == "" {
		return -1, 0
	}

	queryRunes := utf8.RuneCountInString(query)

	offsets := make([]int, 0, utf8.RuneCountInString(text)+1)
	for offset := range text {
		offsets = append(offsets, offset)
	}
	offsets = append(offsets, len(text))

	for i := 0; i+queryRunes < len(offsets); i++ {
		start := offsets[i]
		end := offsets[i+queryRunes]
		if strings.EqualFold(text[start:end], query) {
			return start, end - start
		}
	}

	return -1, 0
}

func snippetAround(text string, matchIndex int, matchLen int, maxRunes int) string {
	if text == "" || maxRunes <= 0 {
		return ""
	}

	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "")
	}

	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}

	startRune := utf8.RuneCountInString(text[:matchIndex])
	matchRunes := utf8.RuneCountInString(text[matchIndex : matchIndex+matchLen])
	if matchRunes == 0 {
		matchRunes = 1
	}

	windowStart := max(startRune-(maxRunes/2), 0)
	windowEnd := min(windowStart+maxRunes, len(runes))
	if windowEnd-windowStart < maxRunes && windowEnd == len(runes) {
		windowStart = max(windowEnd-maxRunes, 0)
	}

	snippet := string(runes[windowStart:windowEnd])
	if windowStart > 0 {
		snippet = "..." + snippet
	}
	if windowEnd < len(runes) {
		snippet += "..."
	}

	return snippet
}
