package memorysearch

import (
	"context"
	"fmt"
	"strings"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/memory"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/tools/common"
)

const (
	defaultLimit    = 10
	maxLimit        = 50
	defaultMaxChars = 800
	maxMaxChars     = 4000
)

var sanitizeValue = guardrails.Sanitize

type input struct {
	Query    string   `json:"query"`
	Kinds    []string `json:"kinds"`
	Filters  filters  `json:"filters"`
	Limit    int      `json:"limit"`
	MaxChars int      `json:"max_chars"`
}

type filters struct {
	Tags []string `json:"tags"`
}

type output struct {
	Results []result `json:"results"`
}

type result struct {
	ID          string       `json:"id"`
	Kind        string       `json:"kind"`
	Status      string       `json:"status"`
	Title       string       `json:"title,omitempty"`
	Text        string       `json:"text"`
	Tags        []string     `json:"tags,omitempty"`
	SourceLinks []sourceLink `json:"source_links,omitempty"`
	Score       float64      `json:"score"`
}

type sourceLink struct {
	SessionID     string `json:"session_id,omitempty"`
	MessageIDs    []uint `json:"message_ids,omitempty"`
	Offsets       []int  `json:"offsets,omitempty"`
	SummaryID     string `json:"summary_id,omitempty"`
	CreatedBy     string `json:"created_by,omitempty"`
	CreatedReason string `json:"created_reason,omitempty"`
}

func Definition(runtime envtypes.Runtime) tools.Definition {
	return tools.Definition{
		Name:        "memory_search",
		Description: "Search durable memory for relevant pinned, semantic, episodic, or procedural memories.",
		Groups:      []string{"core"},
		Requires:    tools.Capabilities{Memory: true},
		InputSchema: common.ObjectSchema(map[string]any{
			"query": common.StringSchema("Search query for durable memory."),
			"kinds": map[string]any{
				"type":        "array",
				"description": "Optional memory kind filters: pinned, semantic, episodic, or procedural.",
				"items": map[string]any{
					"type": "string",
					"enum": []string{
						string(memory.KindPinned),
						string(memory.KindSemantic),
						string(memory.KindEpisodic),
						string(memory.KindProcedural),
					},
				},
			},
			"filters": common.ObjectSchema(map[string]any{
				"tags": map[string]any{
					"type":        "array",
					"description": "Optional memory tag filters. Results must include all provided tags.",
					"items":       common.StringSchema("Memory tag."),
				},
			}),
			"limit":     common.IntegerSchema("Optional maximum number of memories to return. Defaults to 10 and is capped at 50."),
			"max_chars": common.IntegerSchema("Optional maximum characters per memory text. Defaults to 800 and is capped at 4000."),
		}, "query"),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input
			if result := common.DecodeInput(call, &req); result.Error != "" {
				return result, nil
			}

			if runtime == nil {
				return common.ToolError("tool_error", "memory search is not configured"), nil
			}

			query, errResult := searchQuery(req)
			if errResult.Error != "" {
				return errResult, nil
			}

			result, err := runtime.SearchMemory(ctx, query)
			if err != nil {
				return common.ToolError("tool_error", err.Error()), nil
			}

			return common.EncodeOutput(output{Results: outputResults(result.Hits, query.MaxChars)})
		}),
	}
}

func searchQuery(req input) (memory.SearchQuery, tools.Result) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return memory.SearchQuery{}, common.ToolError("invalid_input", "query is required")
	}

	kinds, err := parseKinds(req.Kinds)
	if err != nil {
		return memory.SearchQuery{}, common.ToolError("invalid_input", err.Error())
	}

	limit, err := boundedPositive(req.Limit, defaultLimit, maxLimit, "limit")
	if err != nil {
		return memory.SearchQuery{}, common.ToolError("invalid_input", err.Error())
	}

	maxChars, err := boundedPositive(req.MaxChars, defaultMaxChars, maxMaxChars, "max_chars")
	if err != nil {
		return memory.SearchQuery{}, common.ToolError("invalid_input", err.Error())
	}

	return memory.SearchQuery{
		Text:     query,
		Kinds:    kinds,
		Statuses: []memory.Status{memory.StatusActive},
		Tags:     cleanStrings(req.Filters.Tags),
		Limit:    limit,
		MaxChars: maxChars,
	}, tools.Result{}
}

func parseKinds(values []string) ([]memory.Kind, error) {
	kinds := make([]memory.Kind, 0, len(values))
	for _, value := range cleanStrings(values) {
		kind := memory.Kind(strings.ToLower(value))
		switch kind {
		case memory.KindPinned, memory.KindSemantic, memory.KindEpisodic, memory.KindProcedural:
			kinds = append(kinds, kind)
		default:
			return nil, fmt.Errorf("unsupported memory kind %q", value)
		}
	}
	return kinds, nil
}

func boundedPositive(value int, fallback int, max int, name string) (int, error) {
	if value < 0 {
		return 0, fmt.Errorf("%s must be greater than or equal to 0", name)
	}
	if value == 0 {
		return fallback, nil
	}
	if value > max {
		return max, nil
	}
	return value, nil
}

func outputResults(hits []memory.SearchHit, maxChars int) []result {
	results := make([]result, 0, len(hits))
	for _, hit := range hits {
		item, ok := outputItem(hit.Item, maxChars)
		if !ok {
			continue
		}

		results = append(results, result{
			ID:          item.ID,
			Kind:        string(item.Kind),
			Status:      string(item.Status),
			Title:       item.Title,
			Text:        item.Text,
			Tags:        append([]string(nil), item.Tags...),
			SourceLinks: outputSourceLinks(item.SourceLinks),
			Score:       hit.Score,
		})
	}
	return results
}

func outputItem(item memory.MemoryItem, maxChars int) (memory.MemoryItem, bool) {
	if item.Status != memory.StatusActive {
		return memory.MemoryItem{}, false
	}

	item.Title = sanitizedString(item.Title)
	item.Text = sanitizedString(item.Text)
	if maxChars > 0 && len([]rune(item.Text)) > maxChars {
		item.Text = string([]rune(item.Text)[:maxChars])
	}
	item.Tags = sanitizedStrings(item.Tags)
	if strings.TrimSpace(item.Title) == "" && strings.TrimSpace(item.Text) == "" {
		return memory.MemoryItem{}, false
	}

	scanned := guardrails.SafetyScan(
		strings.Join([]string{item.Title, item.Text}, "\n"),
		item.GuardrailSource(),
	)
	if scanned.Blocked {
		return memory.MemoryItem{}, false
	}

	return item, true
}

func outputSourceLinks(links []memory.SourceLink) []sourceLink {
	if len(links) == 0 {
		return nil
	}

	results := make([]sourceLink, 0, len(links))
	for _, link := range links {
		results = append(results, sourceLink{
			SessionID:     link.SessionID,
			MessageIDs:    append([]uint(nil), link.MessageIDs...),
			Offsets:       append([]int(nil), link.Offsets...),
			SummaryID:     link.SummaryID,
			CreatedBy:     link.CreatedBy,
			CreatedReason: link.CreatedReason,
		})
	}
	return results
}

func sanitizedString(value string) string {
	sanitized, ok := sanitizeValue(value).(string)
	if !ok {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(sanitized)
}

func sanitizedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	sanitized := make([]string, 0, len(values))
	for _, value := range values {
		value = sanitizedString(value)
		if value == "" {
			continue
		}
		sanitized = append(sanitized, value)
	}
	return sanitized
}

func cleanStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		cleaned = append(cleaned, value)
	}
	return cleaned
}
