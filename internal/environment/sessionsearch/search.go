package sessionsearch

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	handmsg "github.com/wandxy/hand/internal/messages"
	sessionstore "github.com/wandxy/hand/internal/session"
	"github.com/wandxy/hand/internal/storage"
)

const (
	defaultSessionSearchMaxResults = 10
	maxSessionSearchResults        = 20
	sessionSearchSnippetRunes      = 200
	sessionSearchBatchSize         = 50
)

func Search(
	ctx context.Context,
	manager *sessionstore.Manager,
	req envtypes.SessionSearchRequest,
) ([]envtypes.SessionSearchResult, error) {
	if manager == nil {
		return nil, errors.New("session manager is required")
	}

	sessionID := normalizeSearchSessionID(req.SessionID)
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, errors.New("query is required")
	}

	role := strings.TrimSpace(strings.ToLower(req.Role))
	toolName := strings.TrimSpace(strings.ToLower(req.ToolName))
	limit := clampSearchResults(req.MaxResults)

	results := make([]envtypes.SessionSearchResult, 0, limit)
	batchSize := max(sessionSearchBatchSize, limit)
	for offset := 0; len(results) < limit; offset += batchSize {
		messages, err := manager.SearchMessages(ctx, sessionID, storage.SearchMessageOptions{
			Limit:    batchSize,
			Offset:   offset,
			Query:    query,
			Role:     handmsg.Role(role),
			ToolName: toolName,
		})
		if err != nil {
			return nil, err
		}
		if len(messages) == 0 {
			break
		}

		for _, message := range messages {
			if !matchesRole(message, role) {
				continue
			}

			searchText, matchedToolName := handmsg.SearchableMessageText(message, toolName)
			if searchText == "" {
				continue
			}

			matchIndex, matchLen := caseInsensitiveMatchIndex(searchText, query)
			if matchIndex < 0 {
				continue
			}

			results = append(results, envtypes.SessionSearchResult{
				MessageID:     message.ID,
				Role:          string(message.Role),
				ToolName:      matchedToolName,
				CreatedAt:     message.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
				Snippet:       snippetAround(searchText, matchIndex, matchLen, sessionSearchSnippetRunes),
				FullTextBytes: len(searchText),
				MatchIndex:    matchIndex,
			})
			if len(results) >= limit {
				break
			}
		}

		if len(messages) < batchSize {
			break
		}
	}

	return results, nil
}

func normalizeSearchSessionID(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return storage.DefaultSessionID
	}
	return sessionID
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

func matchesRole(message handmsg.Message, role string) bool {
	if role == "" {
		return true
	}
	return strings.ToLower(string(message.Role)) == role
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
