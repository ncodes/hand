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

type Options struct {
	MaxExtractCharPerResult int
}

func Definition(provider webprovider.Provider, options ...Options) tools.Definition {
	type input struct {
		URLs     []string `json:"urls"`
		MaxChars *int     `json:"max_chars"`
	}

	opts := resolveOptions(options)

	return tools.Definition{
		Name: "web_extract",
		Description: "Extract readable content from one or more URLs. " +
			"Use this to retrieve and read page contents after discovery.",
		Groups:   []string{"core"},
		Requires: tools.Capabilities{Network: true},
		InputSchema: common.ObjectSchema(map[string]any{
			"urls": map[string]any{
				"type":        "array",
				"description": "URLs to extract readable content from.",
				"items":       common.StringSchema("URL to fetch and extract."),
			},
			"max_chars": common.IntegerSchema("Optional maximum characters to return per extracted page. " +
				"Values above configured maxExtractCharPerResult are clamped."),
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

			maxChars, validationErr := resolveRequestMaxChars(req.MaxChars, opts.MaxExtractCharPerResult)
			if validationErr != nil {
				return common.ToolError("invalid_input", validationErr.Error()), nil
			}

			results, err := provider.Extract(ctx, urls)
			if err != nil {
				return common.ToolError("tool_error", err.Error()), nil
			}

			results = limitResults(results, maxChars)

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

func resolveRequestMaxChars(requested *int, configuredMax int) (int, error) {
	if requested == nil {
		return 0, nil
	}
	if *requested <= 0 {
		return 0, fmt.Errorf("max_chars must be greater than zero")
	}
	if configuredMax > 0 && *requested > configuredMax {
		return configuredMax, nil
	}

	return *requested, nil
}

func limitResults(results []webprovider.ExtractResult, maxChars int) []webprovider.ExtractResult {
	if maxChars <= 0 {
		return results
	}

	limited := make([]webprovider.ExtractResult, len(results))
	copy(limited, results)
	for idx := range limited {
		content, truncated := truncateContent(limited[idx].Content, maxChars)
		limited[idx].Content = content
		limited[idx].Truncated = limited[idx].Truncated || truncated
	}

	return limited
}

func truncateContent(value string, maxChars int) (string, bool) {
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
