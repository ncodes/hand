package agent

import (
	"context"
	"reflect"
	"strings"
	"unicode"

	instruct "github.com/wandxy/hand/internal/instructions"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	storage "github.com/wandxy/hand/internal/state/core"
)

const maxSessionTitleRunes = 80

func (a *Agent) maybeGenerateSessionTitle(ctx context.Context, sessionID string) {
	if a == nil || a.cfg == nil || a.stateMgr == nil || a.summaryClient == nil {
		return
	}
	if isSameModelClient(a.summaryClient, a.modelClient) && strings.TrimSpace(a.cfg.Models.Summary.Name) == "" {
		return
	}

	session, ok, err := a.stateMgr.Get(ctx, sessionID)
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

	title := a.generateSessionTitle(ctx, contextText)
	if title == "" {
		title = fallback
	}

	session.Title = title
	session.TitleSource = storage.SessionTitleSourceGenerated
	_ = a.stateMgr.Save(ctx, session)
}

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

func (a *Agent) generateSessionTitle(ctx context.Context, contextText string) string {
	resp, err := a.summaryClient.Complete(ctx, models.Request{
		Model:           a.cfg.SummaryModelEffective(),
		APIMode:         a.cfg.SummaryModelAPIModeEffective(),
		Instructions:    instruct.BuildSessionTitle().String(),
		Messages:        []handmsg.Message{{Role: handmsg.RoleUser, Content: contextText}},
		MaxOutputTokens: 24,
		Temperature:     0,
		DebugRequests:   a.cfg.Debug.Requests,
	})
	if err != nil || resp == nil {
		return ""
	}

	return normalizeGeneratedSessionTitle(resp.OutputText)
}

func getSessionTitleContext(messages []handmsg.Message) (string, string) {
	userText := ""
	assistantText := ""
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}

		switch message.Role {
		case handmsg.RoleUser:
			if userText == "" {
				userText = content
			}
		case handmsg.RoleAssistant:
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
