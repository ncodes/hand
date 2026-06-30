package provider_ollama

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/wandxy/morph/pkg/stringx"
)

func ollamaStatusErrorForModel(resp *http.Response, model string) error {
	body, _ := io.ReadAll(resp.Body)
	detail := stringx.String(string(body)).Trim()
	if detail == "" {
		detail = resp.Status
	}

	return enrichOllamaProviderError(resp.StatusCode, detail, model)
}

func enrichOllamaProviderError(statusCode int, detail string, model string) error {
	detail = stringx.String(detail).Trim()
	base := fmt.Sprintf("ollama request failed with status %d: %s", statusCode, detail)
	model = stringx.String(model).Trim()
	if isOllamaMissingModelMessage(statusCode, detail) && model != "" {
		return fmt.Errorf(
			"ollama model %q is not installed or could not be found; run %s or ollama pull %s: %s",
			model,
			ollamaPullCommand(model),
			model,
			base,
		)
	}
	if isOllamaToolMessage(detail) {
		return fmt.Errorf(
			"ollama tool calling failed; choose a tool-capable Ollama model or disable tools: %s",
			base,
		)
	}

	return fmt.Errorf("%s", base)
}

func enrichOllamaPullError(detail string, model string) error {
	detail = stringx.String(detail).Trim()
	model = stringx.String(model).Trim()
	if isOllamaMissingModelMessage(http.StatusNotFound, detail) && model != "" {
		return fmt.Errorf(
			"ollama model %q is not installed or could not be found; run %s or ollama pull %s: ollama pull failed: %s",
			model,
			ollamaPullCommand(model),
			model,
			detail,
		)
	}

	return fmt.Errorf("ollama pull failed: %s", detail)
}

func ollamaRawToolJSONError(model string) error {
	model = stringx.String(model).Trim()
	if model == "" {
		return fmt.Errorf("ollama model returned raw tool JSON instead of a structured tool call; choose a tool-capable Ollama model or disable tools")
	}

	return fmt.Errorf("ollama model %q returned raw tool JSON instead of a structured tool call; choose a tool-capable Ollama model or disable tools", model)
}

func isRawToolJSONOutput(text string, tools []chatTool) bool {
	if len(tools) == 0 {
		return false
	}

	text = stringx.String(text).Trim()
	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") {
		return false
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return false
	}
	if hasToolJSONShape(payload) || hasToolNameField(payload, tools) {
		return true
	}

	return false
}

func hasToolJSONShape(payload map[string]any) bool {
	for key := range payload {
		switch stringx.String(key).Normalized() {
		case "tool", "tool_name", "tool_call", "tool_calls", "function", "arguments", "parameters":
			return true
		}
	}

	return false
}

func hasToolNameField(payload map[string]any, tools []chatTool) bool {
	toolNames := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		name := stringx.String(tool.Function.Name).Normalized()
		if name != "" {
			toolNames[name] = struct{}{}
		}
	}
	if len(toolNames) == 0 {
		return false
	}

	for _, key := range []string{"name", "toolName", "tool_name", "function"} {
		value, ok := payload[key]
		if !ok {
			continue
		}
		name, ok := value.(string)
		if !ok {
			continue
		}
		if _, ok := toolNames[stringx.String(name).Normalized()]; ok {
			return true
		}
	}

	return false
}

func isOllamaMissingModelMessage(statusCode int, detail string) bool {
	message := stringx.String(detail).Normalized()
	return statusCode == http.StatusNotFound ||
		strings.Contains(message, "model not found") ||
		strings.Contains(message, "not found") ||
		strings.Contains(message, "does not exist") ||
		strings.Contains(message, "file does not exist") ||
		strings.Contains(message, "pull model manifest")
}

func isOllamaToolMessage(detail string) bool {
	message := stringx.String(detail).Normalized()
	if !strings.Contains(message, "tool") && !strings.Contains(message, "function") {
		return false
	}

	return strings.Contains(message, "unsupported") ||
		strings.Contains(message, "not support") ||
		strings.Contains(message, "invalid") ||
		strings.Contains(message, "failed")
}

func ollamaPullCommand(model string) string {
	return fmt.Sprintf("morph setup provider --provider ollama --model %s --pull", stringx.String(model).Trim())
}
