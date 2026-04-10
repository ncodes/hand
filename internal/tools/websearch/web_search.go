package websearch

import (
	"context"
	"strings"

	webintegration "github.com/wandxy/hand/internal/providers/web"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/tools/common"
)

const (
	defaultCount = 5
	maxCount     = 10
)

func Definition(provider webintegration.Provider) tools.Definition {
	type input struct {
		Query string `json:"query"`
		Count int    `json:"count"`
	}

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

			results, err := provider.Search(ctx, query, count)
			if err != nil {
				return common.ToolError("tool_error", err.Error()), nil
			}

			return common.EncodeOutput(map[string]any{"results": results})
		}),
	}
}
