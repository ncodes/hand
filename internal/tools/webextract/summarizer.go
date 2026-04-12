package webextract

import (
	"context"
	"errors"
	"strings"

	"github.com/wandxy/hand/internal/config"
	instruct "github.com/wandxy/hand/internal/instructions"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
	webprovider "github.com/wandxy/hand/internal/providers/web"
)

type Summarizer interface {
	SummarizeExtract(context.Context, SummaryInput) (string, error)
}

type SummaryInput struct {
	URL             string
	Title           string
	Query           string
	Content         string
	MaxSummaryChars int
}

type summarizerContextKey struct{}

type summarizeOptions struct {
	Query                          string
	MinSummarizeChars              int
	MaxSummaryChars                int
	SummarizeRefusalThresholdChars int
}

type ModelSummarizer struct {
	Client        models.Client
	Model         string
	APIMode       string
	DebugRequests bool
}

func NewModelSummarizer(client models.Client, cfg *config.Config) Summarizer {
	if client == nil || cfg == nil {
		return nil
	}

	normalized := *cfg
	normalized.Normalize()

	return ModelSummarizer{
		Client:        client,
		Model:         normalized.SummaryModelEffective(),
		APIMode:       normalized.SummaryModelAPIModeEffective(),
		DebugRequests: normalized.DebugRequests,
	}
}

func WithSummarizer(ctx context.Context, summarizer Summarizer) context.Context {
	if summarizer == nil {
		return ctx
	}

	return context.WithValue(ctx, summarizerContextKey{}, summarizer)
}

func summarizerFromContext(ctx context.Context) Summarizer {
	if ctx == nil {
		return nil
	}

	summarizer, _ := ctx.Value(summarizerContextKey{}).(Summarizer)
	return summarizer
}

func (s ModelSummarizer) SummarizeExtract(ctx context.Context, input SummaryInput) (string, error) {
	if s.Client == nil {
		return "", errors.New("web extract summarizer is not configured")
	}

	resp, err := s.Client.Complete(ctx, models.Request{
		Model:        s.Model,
		APIMode:      s.APIMode,
		Instructions: instruct.BuildWebExtractSummary(input.MaxSummaryChars),
		Messages: []handmsg.Message{{
			Role:    handmsg.RoleUser,
			Content: renderSummaryPrompt(input),
		}},
		MaxOutputTokens: maxSummaryOutputTokens(input.MaxSummaryChars),
		Temperature:     0,
		DebugRequests:   s.DebugRequests,
	})
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", errors.New("web extract summary response is required")
	}
	if resp.RequiresToolCalls {
		return "", errors.New("web extract summary requested tool calls")
	}
	if strings.TrimSpace(resp.OutputText) == "" {
		return "", errors.New("web extract summary is empty")
	}

	return strings.TrimSpace(resp.OutputText), nil
}

func summarizeResults(
	ctx context.Context,
	results []webprovider.ExtractResult,
	options summarizeOptions,
) ([]webprovider.ExtractResult, error) {
	if len(results) == 0 {
		return results, nil
	}

	summarizer := summarizerFromContext(ctx)
	summarized := make([]webprovider.ExtractResult, len(results))
	copy(summarized, results)
	for idx := range summarized {
		result := &summarized[idx]
		if strings.TrimSpace(result.Error) != "" || strings.TrimSpace(result.Content) == "" {
			continue
		}

		sourceChars := runeLen(result.Content)
		if sourceChars < options.MinSummarizeChars {
			continue
		}
		result.SourceContentChars = sourceChars
		if options.SummarizeRefusalThresholdChars > 0 && sourceChars > options.SummarizeRefusalThresholdChars {
			result.SummaryRefused = true
			if strings.TrimSpace(result.Error) == "" {
				result.Error = "content exceeds summarization threshold"
			}
			continue
		}
		if summarizer == nil {
			return nil, errors.New("web extract summarizer is not configured")
		}

		summary, err := summarizer.SummarizeExtract(ctx, SummaryInput{
			URL:             result.URL,
			Title:           result.Title,
			Query:           options.Query,
			Content:         result.Content,
			MaxSummaryChars: options.MaxSummaryChars,
		})
		if err != nil {
			return nil, err
		}

		summary, truncated := truncateToChars(summary, options.MaxSummaryChars)
		result.Content = summary
		result.ContentFormat = "summary"
		result.Summarized = true
		result.SummaryChars = runeLen(summary)
		result.Truncated = result.Truncated || truncated
	}

	return summarized, nil
}

func renderSummaryPrompt(input SummaryInput) string {
	parts := []string{
		"URL: " + strings.TrimSpace(input.URL),
		"Title: " + strings.TrimSpace(input.Title),
	}
	if query := strings.TrimSpace(input.Query); query != "" {
		parts = append(parts, "Query: "+query)
	}
	parts = append(parts, "Content:\n"+strings.TrimSpace(input.Content))

	return strings.Join(parts, "\n\n")
}

func maxSummaryOutputTokens(maxSummaryChars int) int64 {
	if maxSummaryChars <= 0 {
		return 0
	}

	return int64(maxSummaryChars/4 + 128)
}

func truncateToChars(value string, maxChars int) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || maxChars <= 0 {
		return value, false
	}

	runes := []rune(value)
	if len(runes) <= maxChars {
		return value, false
	}

	return strings.TrimSpace(string(runes[:maxChars])), true
}

func runeLen(value string) int {
	return len([]rune(strings.TrimSpace(value)))
}
