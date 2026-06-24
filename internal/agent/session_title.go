package agent

import (
	"context"
	"reflect"
	"strings"
	"unicode"

	instruct "github.com/wandxy/morph/internal/instructions"
	models "github.com/wandxy/morph/internal/model"
	storage "github.com/wandxy/morph/internal/state/core"
	morphmsg "github.com/wandxy/morph/pkg/agent/message"
)

const maxSessionTitleRunes = 80

// maybeGenerateSessionTitle fills an empty session title after the first useful exchange.
func (a *Agent) maybeGenerateSessionTitle(ctx context.Context, sessionID string) {
	if a == nil || a.cfg == nil || a.stateMgr == nil || a.summaryClient == nil {
		return
	}
	if isSameModelClient(a.summaryClient, a.modelClient) && strings.TrimSpace(a.cfg.Models.Summary.Name) == "" {
		return
	}

	// Titles are generated once. User/session-provided titles are left intact.
	session, ok, err := a.stateMgr.Get(ctx, sessionID, storage.SessionGetOptions{})
	if err != nil || !ok || strings.TrimSpace(session.Title) != "" {
		return
	}

	messages, err := a.stateMgr.GetMessages(ctx, session.ID, storage.MessageQueryOptions{
		Order: storage.MessageOrderAsc,
		Limit: 8,
	})
	if err != nil || len(messages) == 0 {
		return
	}

	contextText, fallback := getSessionTitleContext(messages)
	if fallback == "" {
		return
	}

	// The fallback keeps the UI useful even if the summary model fails or
	// returns a generic/banned title.
	title := a.generateSessionTitle(ctx, contextText)
	if title == "" {
		title = fallback
	}

	session.Title = title
	session.TitleSource = storage.SessionTitleSourceGenerated
	_ = a.stateMgr.Save(ctx, session)
}

// isSameModelClient reports whether two model clients are the same comparable value.
func isSameModelClient(left models.Client, right models.Client) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	leftValue := reflect.ValueOf(left)
	rightValue := reflect.ValueOf(right)
	if leftValue.Type() != rightValue.Type() || !leftValue.Type().Comparable() {
		return false
	}

	return leftValue.Interface() == rightValue.Interface()
}

// generateSessionTitle asks the summary model for a short title.
func (a *Agent) generateSessionTitle(ctx context.Context, contextText string) string {
	resp, err := a.summaryClient.Complete(ctx, models.Request{
		Model:           a.cfg.SummaryModelEffective(),
		API:             a.cfg.SummaryModelAPIEffective(),
		Instructions:    instruct.BuildSessionTitle().String(),
		Messages:        []morphmsg.Message{{Role: morphmsg.RoleUser, Content: contextText}},
		MaxOutputTokens: a.cfg.SummaryModelMaxOutputTokensEffective(24),
		Temperature:     0,
		DebugRequests:   a.cfg.Debug.Requests,
	})
	if err != nil || resp == nil {
		return ""
	}

	return normalizeGeneratedSessionTitle(resp.OutputText)
}

// getSessionTitleContext builds a compact title prompt from early user/assistant messages.
func getSessionTitleContext(messages []morphmsg.Message) (string, string) {
	userText := ""
	assistantText := ""
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}

		switch message.Role {
		case morphmsg.RoleUser:
			if userText == "" {
				userText = content
			}
		case morphmsg.RoleAssistant:
			if assistantText == "" {
				assistantText = content
			}
		}

		if userText != "" && assistantText != "" {
			break
		}
	}
	if userText == "" {
		return "", ""
	}

	contextText := "User: " + userText
	if assistantText != "" {
		contextText += "\nAssistant: " + assistantText
	}

	return contextText, fallbackSessionTitleFromUserMessage(userText)
}

// normalizeGeneratedSessionTitle trims model punctuation, whitespace, and generic titles.
func normalizeGeneratedSessionTitle(title string) string {
	title = strings.TrimSpace(title)
	title = strings.Trim(title, "\"'`")
	title = strings.TrimSpace(title)
	title = strings.TrimRightFunc(title, func(r rune) bool {
		return r == '.' || r == ':' || r == ';' || r == '!' || r == '?'
	})
	title = strings.Join(strings.Fields(title), " ")
	title = trimTitleRunes(title, maxSessionTitleRunes)
	if title == "" || hasBannedSessionTitleWord(title) {
		return ""
	}

	return title
}

// fallbackSessionTitleFromUserMessage derives a title from the first user message.
func fallbackSessionTitleFromUserMessage(message string) string {
	words := strings.Fields(strings.TrimSpace(message))
	if len(words) == 0 {
		return ""
	}
	if len(words) > 8 {
		words = words[:8]
	}

	title := strings.Join(words, " ")
	title = strings.TrimRightFunc(title, func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSymbol(r)
	})

	return trimTitleRunes(title, maxSessionTitleRunes)
}

// hasBannedSessionTitleWord rejects overly generic generated titles.
func hasBannedSessionTitleWord(title string) bool {
	for _, word := range strings.Fields(strings.ToLower(title)) {
		word = strings.TrimFunc(word, func(r rune) bool {
			return unicode.IsPunct(r) || unicode.IsSymbol(r)
		})
		switch word {
		case "chat", "conversation", "session":
			return true
		}
	}

	return false
}

// trimTitleRunes trims a title to a rune limit without changing encoding boundaries.
func trimTitleRunes(title string, limit int) string {
	if limit <= 0 {
		return ""
	}

	runes := []rune(strings.TrimSpace(title))
	if len(runes) <= limit {
		return string(runes)
	}

	return strings.TrimSpace(string(runes[:limit]))
}
