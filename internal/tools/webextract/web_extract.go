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
	MaxExtractCharPerResult        int
	MinSummarizeChars              int
	MaxSummaryChars                int
	SummarizeRefusalThresholdChars int
}

func Definition(provider webprovider.Provider, options ...Options) tools.Definition {
	type input struct {
		URLs        []string `json:"urls"`
		MaxChars    *int     `json:"max_chars"`
		Query       string   `json:"query"`
		Summarize   bool     `json:"summarize"`
		Format      string   `json:"format"`
		ExtractMode string   `json:"extract_mode"`
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

			"query": common.StringSchema("Optional focused extraction query. Providers that support it " +
				"use this to return content most relevant to the query."),

			"summarize": common.BooleanSchema("When true, summarize extracted content that exceeds the " +
				"configured minimum summarization size."),

			"format": map[string]any{
				"type":        "string",
				"description": "Optional output content format. Valid values are text or markdown.",
				"enum":        []string{"text", "markdown"},
			},

			"extract_mode": map[string]any{
				"type":        "string",
				"description": "Alias for format. Valid values are text or markdown.",
				"enum":        []string{"text", "markdown"},
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

			maxChars, validationErr := resolveRequestMaxChars(req.MaxChars, opts.MaxExtractCharPerResult)
			if validationErr != nil {
				return common.ToolError("invalid_input", validationErr.Error()), nil
			}

			format, validationErr := resolveFormat(req.Format, req.ExtractMode)
			if validationErr != nil {
				return common.ToolError("invalid_input", validationErr.Error()), nil
			}

			ctx = webprovider.WithExtractOptions(ctx, webprovider.ExtractOptions{
				Format:   format,
				MaxChars: maxChars,
				Query:    strings.TrimSpace(req.Query),
			})

			results, err := provider.Extract(ctx, urls)
			if err != nil {
				return common.ToolError("tool_error", err.Error()), nil
			}

			if req.Summarize {
				results, err = summarizeResults(ctx, results, summarizeOptions{
					Query:                          strings.TrimSpace(req.Query),
					MinSummarizeChars:              opts.MinSummarizeChars,
					MaxSummaryChars:                opts.MaxSummaryChars,
					SummarizeRefusalThresholdChars: opts.SummarizeRefusalThresholdChars,
				})
				if err != nil {
					return common.ToolError("tool_error", err.Error()), nil
				}
			}

			return common.EncodeOutput(map[string]any{"results": results})
		}),
	}
}

func resolveFormat(format, extractMode string) (string, error) {
	format = strings.TrimSpace(strings.ToLower(format))
	extractMode = strings.TrimSpace(strings.ToLower(extractMode))
	if format != "" && extractMode != "" && format != extractMode {
		return "", fmt.Errorf("format and extract_mode must match when both are provided")
	}
	if format == "" {
		format = extractMode
	}
	if format == "" {
		return "", nil
	}
	if format != "text" && format != "markdown" {
		return "", fmt.Errorf("format must be text or markdown")
	}

	return format, nil
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
