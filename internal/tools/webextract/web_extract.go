package webextract

import (
	"context"
	"fmt"
	"strings"

	webprovider "github.com/wandxy/hand/internal/providers/web"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/tools/common"
)

const maxURLs = 5

func Definition(provider webprovider.Provider) tools.Definition {
	type input struct {
		URLs []string `json:"urls"`
	}

	return tools.Definition{
		Name:        "web_extract",
		Description: "Extract readable content from one or more URLs. Use this to retrieve and read page contents after discovery.",
		Groups:      []string{"core"},
		Requires:    tools.Capabilities{Network: true},
		InputSchema: common.ObjectSchema(map[string]any{
			"urls": map[string]any{
				"type":        "array",
				"description": "URLs to extract readable content from.",
				"items":       common.StringSchema("URL to fetch and extract."),
			},
		}, "urls"),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input
			if result := common.DecodeInput(call, &req); result.Error != "" {
				return result, nil
			}

			if provider == nil {
				return common.ToolError("tool_error", "web extract provider is not configured"), nil
			}
			if len(req.URLs) == 0 {
				return common.ToolError("invalid_input", "urls is required"), nil
			}
			if len(req.URLs) > maxURLs {
				return common.ToolError("invalid_input", "too many urls"), nil
			}

			urls := make([]string, 0, len(req.URLs))
			for idx, rawURL := range req.URLs {
				url := strings.TrimSpace(rawURL)
				if url == "" {
					return common.ToolError("invalid_input", fmt.Sprintf("url at index %d is required", idx)), nil
				}
				urls = append(urls, url)
			}

			results, err := provider.Extract(ctx, urls)
			if err != nil {
				return common.ToolError("tool_error", err.Error()), nil
			}

			return common.EncodeOutput(map[string]any{"results": results})
		}),
	}
}
