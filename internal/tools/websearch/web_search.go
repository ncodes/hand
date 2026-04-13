package websearch

import (
	"context"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/wandxy/hand/internal/guardrails"
	webintegration "github.com/wandxy/hand/internal/providers/web"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/tools/common"
)

const (
	defaultCount = 5
	maxCount     = 10
)

type Options struct {
	WebsitePolicy guardrails.WebsitePolicy
}

func Definition(provider webintegration.Provider, options ...Options) tools.Definition {
	type input struct {
		Query string `json:"query"`
		Count int    `json:"count"`
	}

	opts := resolveOptions(options)

	return tools.Definition{
		Name:        "web_search",
		Description: "Search the web for relevant pages. Use this for discovery and result-finding, not for full-page extraction.",
		Groups:      []string{"core"},
		Requires:    tools.Capabilities{Network: true},
		InputSchema: common.ObjectSchema(map[string]any{
			"query": common.StringSchema("Search query to run."),
			"count": common.IntegerSchema("Maximum number of results to return. Defaults to 5 and is capped at 10."),
		}, "query"),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input

			if result := common.DecodeInput(call, &req); result.Error != "" {
				return result, nil
			}

			if provider == nil {
				return common.ToolError("tool_error", "web search provider is not configured"), nil
			}

			query := strings.TrimSpace(req.Query)
			if query == "" {
				return common.ToolError("invalid_input", "query is required"), nil
			}

			count := req.Count
			if count <= 0 {
				count = defaultCount
			}
			if count > maxCount {
				count = maxCount
			}

			log.Info().
				Str("tool", "web_search").
				Str("phase", "start").
				Int("query_chars", len([]rune(query))).
				Int("count", count).
				Bool("website_policy_enabled", opts.WebsitePolicy.Enabled).
				Msg("tool call started")

			log.Debug().
				Str("tool", "web_search").
				Str("phase", "execute").
				Msg("web search provider request started")
			results, err := provider.Search(ctx, query, count)
			if err != nil {
				log.Warn().
					Err(err).
					Str("tool", "web_search").
					Str("phase", "error").
					Msg("web search provider request failed")
				return common.ToolError("tool_error", err.Error()), nil
			}

			results, blocked := filterBlockedResults(results, opts.WebsitePolicy)

			log.Info().
				Str("tool", "web_search").
				Str("phase", "complete").
				Int("result_count", len(results)).
				Int("blocked_results", blocked).
				Msg("tool call completed")

			return common.EncodeOutput(map[string]any{"results": results})
		}),
	}
}

func resolveOptions(options []Options) Options {
	if len(options) == 0 {
		return Options{}
	}

	return options[0]
}

func filterBlockedResults(
	results []webintegration.SearchResult,
	policy guardrails.WebsitePolicy,
) ([]webintegration.SearchResult, int) {
	if len(results) == 0 || !policy.Enabled {
		return results, 0
	}

	blockedCount := 0
	filtered := make([]webintegration.SearchResult, 0, len(results))
	for _, result := range results {
		if _, blocked := policy.Check(result.URL); blocked {
			blockedCount++
			continue
		}
		filtered = append(filtered, result)
	}

	return filtered, blockedCount
}
