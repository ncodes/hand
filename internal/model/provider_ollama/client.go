package provider_ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	models "github.com/wandxy/morph/internal/model"
	morphmsg "github.com/wandxy/morph/pkg/agent/message"
)

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type OllamaClient struct {
	baseURL    string
	headers    map[string]string
	httpClient httpDoer
}

type chatRequest struct {
	Model    string         `json:"model"`
	Messages []chatMessage  `json:"messages"`
	Stream   bool           `json:"stream"`
	Tools    []chatTool     `json:"tools,omitempty"`
	Format   any            `json:"format,omitempty"`
	Options  map[string]any `json:"options,omitempty"`
}

type chatMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content,omitempty"`
	Thinking  string         `json:"thinking,omitempty"`
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type chatToolCall struct {
	Function chatToolCallFunction `json:"function"`
}

type chatToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type chatResponse struct {
	Model           string      `json:"model"`
	Message         chatMessage `json:"message"`
	Done            bool        `json:"done"`
	DoneReason      string      `json:"done_reason"`
	PromptEvalCount int         `json:"prompt_eval_count"`
	EvalCount       int         `json:"eval_count"`
}

type normalizedRequest struct {
	Model            string
	Instructions     string
	Messages         []morphmsg.Message
	Tools            []ToolDefinition
	StructuredOutput *StructuredOutput
	ContextLength    int
	MaxOutputTokens  int64
	Temperature      float64
}

func NewOllamaClient(baseURL string, headers map[string]string) (*OllamaClient, error) {
	return newOllamaClient(baseURL, headers, http.DefaultClient)
}

func newOllamaClient(baseURL string, headers map[string]string, httpClient httpDoer) (*OllamaClient, error) {
	normalizedBaseURL, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if httpClient == nil {
		return nil, errors.New("ollama HTTP client is required")
	}

	return &OllamaClient{
		baseURL:    normalizedBaseURL,
		headers:    normalizeHeaders(headers),
		httpClient: httpClient,
	}, nil
}

func (c *OllamaClient) Complete(ctx context.Context, req Request) (*Response, error) {
	return c.complete(ctx, req, false, nil)
}

func (c *OllamaClient) CompleteStream(
	ctx context.Context,
	req Request,
	onTextDelta func(StreamDelta),
) (*Response, error) {
	return c.complete(ctx, req, true, onTextDelta)
}

func (c *OllamaClient) complete(
	ctx context.Context,
	req Request,
	stream bool,
	onTextDelta func(StreamDelta),
) (*Response, error) {
	if c == nil {
		return nil, errors.New("model client is required")
	}

	normalizedReq, err := normalizeRequest(req)
	if err != nil {
		return nil, err
	}

	providerReq := buildChatRequest(normalizedReq, stream)
	if stream {
		return c.completeStream(ctx, providerReq, onTextDelta)
	}

	providerResp, err := c.postChat(ctx, providerReq)
	if err != nil {
		return nil, err
	}

	return responseFromChatResponse(providerResp)
}

func (c *OllamaClient) postChat(ctx context.Context, providerReq chatRequest) (chatResponse, error) {
	var providerResp chatResponse
	resp, err := c.doJSON(ctx, "/api/chat", providerReq)
	if err != nil {
		return chatResponse{}, err
	}
	defer resp.Body.Close()

	if err := decodeOllamaResponse(resp, &providerResp); err != nil {
		return chatResponse{}, err
	}

	return providerResp, nil
}

func (c *OllamaClient) completeStream(
	ctx context.Context,
	providerReq chatRequest,
	onTextDelta func(StreamDelta),
) (*Response, error) {
	resp, err := c.doJSON(ctx, "/api/chat", providerReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, ollamaStatusError(resp)
	}

	var acc chatStreamAccumulator
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var chunk chatResponse
		if err := json.Unmarshal(line, &chunk); err != nil {
			return nil, fmt.Errorf("decode Ollama stream chunk: %w", err)
		}
		acc.add(chunk)
		if onTextDelta != nil && chunk.Message.Thinking != "" {
			onTextDelta(StreamDelta{Channel: models.StreamChannelReasoning, Text: chunk.Message.Thinking})
		}
		if onTextDelta != nil && chunk.Message.Content != "" {
			onTextDelta(StreamDelta{Channel: models.StreamChannelAssistant, Text: chunk.Message.Content})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return acc.response()
}

func (c *OllamaClient) doJSON(ctx context.Context, path string, body any) (*http.Response, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, enrichOllamaConnectionError(c.baseURL, err)
	}

	return resp, nil
}

type chatStreamAccumulator struct {
	model            string
	outputText       strings.Builder
	toolCalls        []ToolCall
	promptTokens     int
	completionTokens int
}

func (a *chatStreamAccumulator) add(chunk chatResponse) {
	if chunk.Model != "" {
		a.model = chunk.Model
	}
	if chunk.Message.Content != "" {
		a.outputText.WriteString(chunk.Message.Content)
	}
	if len(chunk.Message.ToolCalls) > 0 {
		a.toolCalls = append(a.toolCalls, toolCallsFromChatToolCalls(chunk.Message.ToolCalls, len(a.toolCalls))...)
	}
	if chunk.PromptEvalCount > 0 {
		a.promptTokens = chunk.PromptEvalCount
	}
	if chunk.EvalCount > 0 {
		a.completionTokens = chunk.EvalCount
	}
}

func (a *chatStreamAccumulator) response() (*Response, error) {
	resp := &Response{
		Model:             a.model,
		OutputText:        strings.TrimSpace(a.outputText.String()),
		ToolCalls:         a.toolCalls,
		RequiresToolCalls: len(a.toolCalls) > 0,
		PromptTokens:      a.promptTokens,
		CompletionTokens:  a.completionTokens,
		TotalTokens:       a.promptTokens + a.completionTokens,
	}
	if resp.OutputText == "" && len(resp.ToolCalls) == 0 {
		return nil, errors.New("model returned empty response")
	}

	return resp, nil
}

func normalizeRequest(req Request) (normalizedRequest, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return normalizedRequest{}, errors.New("model is required")
	}
	if len(req.Messages) == 0 {
		return normalizedRequest{}, errors.New("messages are required")
	}

	messages, err := normalizeMessages(req.Messages)
	if err != nil {
		return normalizedRequest{}, err
	}
	tools, err := normalizeToolDefinitions(req.Tools)
	if err != nil {
		return normalizedRequest{}, err
	}

	return normalizedRequest{
		Model:            model,
		Instructions:     strings.TrimSpace(req.Instructions),
		Messages:         messages,
		Tools:            tools,
		StructuredOutput: normalizeStructuredOutput(req.StructuredOutput),
		ContextLength:    req.ContextLength,
		MaxOutputTokens:  req.MaxOutputTokens,
		Temperature:      req.Temperature,
	}, nil
}

func normalizeMessages(messages []morphmsg.Message) ([]morphmsg.Message, error) {
	normalized := make([]morphmsg.Message, 0, len(messages))
	for _, message := range messages {
		role := morphmsg.Role(strings.TrimSpace(strings.ToLower(string(message.Role))))
		content := strings.TrimSpace(message.Content)
		toolCallID := strings.TrimSpace(message.ToolCallID)
		toolCalls, err := normalizeToolCalls(message.ToolCalls)
		if err != nil {
			return nil, err
		}

		switch role {
		case morphmsg.RoleDeveloper:
			return nil, errors.New("developer messages must be provided via instructions")
		case morphmsg.RoleUser, morphmsg.RoleAssistant, morphmsg.RoleTool:
		default:
			return nil, errors.New("message role must be one of user, assistant, or tool; developer messages must be provided via instructions")
		}
		if content == "" && !(role == morphmsg.RoleAssistant && len(toolCalls) > 0) {
			return nil, errors.New("message content is required")
		}
		if role == morphmsg.RoleTool && toolCallID == "" {
			return nil, errors.New("tool call id is required")
		}

		normalized = append(normalized, morphmsg.Message{
			Role:       role,
			Content:    content,
			Name:       strings.TrimSpace(message.Name),
			ToolCallID: toolCallID,
			ToolCalls:  toolCalls,
			CreatedAt:  message.CreatedAt,
		})
	}

	return normalized, nil
}

func normalizeToolDefinitions(definitions []ToolDefinition) ([]ToolDefinition, error) {
	if len(definitions) == 0 {
		return nil, nil
	}

	normalized := make([]ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		name := strings.TrimSpace(definition.Name)
		if name == "" {
			return nil, errors.New("tool name is required")
		}
		normalized = append(normalized, ToolDefinition{
			Name:         name,
			Description:  strings.TrimSpace(definition.Description),
			InputSchema:  definition.InputSchema,
			ParallelSafe: definition.ParallelSafe,
		})
	}

	return normalized, nil
}

func normalizeToolCalls(toolCalls []morphmsg.ToolCall) ([]morphmsg.ToolCall, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}

	normalized := make([]morphmsg.ToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		id := strings.TrimSpace(toolCall.ID)
		name := strings.TrimSpace(toolCall.Name)
		if id == "" {
			return nil, errors.New("tool call id is required")
		}
		if name == "" {
			return nil, errors.New("tool call name is required")
		}
		normalized = append(normalized, morphmsg.ToolCall{
			ID:    id,
			Name:  name,
			Input: strings.TrimSpace(toolCall.Input),
		})
	}

	return normalized, nil
}

func normalizeStructuredOutput(value *StructuredOutput) *StructuredOutput {
	if value == nil {
		return nil
	}

	name := strings.TrimSpace(value.Name)
	if name == "" || len(value.Schema) == 0 {
		return nil
	}

	return &StructuredOutput{
		Name:        name,
		Description: strings.TrimSpace(value.Description),
		Schema:      value.Schema,
		Strict:      value.Strict,
	}
}

func buildChatRequest(req normalizedRequest, stream bool) chatRequest {
	messages := make([]chatMessage, 0, len(req.Messages)+1)
	if req.Instructions != "" {
		messages = append(messages, chatMessage{Role: "system", Content: req.Instructions})
	}
	for _, message := range req.Messages {
		messages = append(messages, messageToChatMessage(message))
	}

	providerReq := chatRequest{
		Model:    req.Model,
		Messages: messages,
		Stream:   stream,
		Tools:    toolsToChatTools(req.Tools),
		Format:   structuredOutputToFormat(req.StructuredOutput),
		Options:  buildOptions(req),
	}

	return providerReq
}

func messageToChatMessage(message morphmsg.Message) chatMessage {
	converted := chatMessage{
		Role:    string(message.Role),
		Content: strings.TrimSpace(message.Content),
	}
	if len(message.ToolCalls) > 0 {
		converted.ToolCalls = make([]chatToolCall, 0, len(message.ToolCalls))
		for _, toolCall := range message.ToolCalls {
			converted.ToolCalls = append(converted.ToolCalls, chatToolCall{
				Function: chatToolCallFunction{
					Name:      toolCall.Name,
					Arguments: json.RawMessage(defaultToolArguments(toolCall.Input)),
				},
			})
		}
	}

	return converted
}

func toolsToChatTools(definitions []ToolDefinition) []chatTool {
	if len(definitions) == 0 {
		return nil
	}

	tools := make([]chatTool, 0, len(definitions))
	for _, definition := range definitions {
		tools = append(tools, chatTool{
			Type: "function",
			Function: chatFunction{
				Name:        definition.Name,
				Description: definition.Description,
				Parameters:  definition.InputSchema,
			},
		})
	}

	return tools
}

func structuredOutputToFormat(output *StructuredOutput) any {
	if output == nil {
		return nil
	}

	return output.Schema
}

func buildOptions(req normalizedRequest) map[string]any {
	options := make(map[string]any)
	if req.ContextLength > 0 {
		options["num_ctx"] = req.ContextLength
	}
	if req.MaxOutputTokens > 0 {
		options["num_predict"] = req.MaxOutputTokens
	}
	if req.Temperature > 0 {
		options["temperature"] = req.Temperature
	}
	if len(options) == 0 {
		return nil
	}

	return options
}

func responseFromChatResponse(resp chatResponse) (*Response, error) {
	toolCalls := toolCallsFromChatToolCalls(resp.Message.ToolCalls, 0)
	outputText := strings.TrimSpace(resp.Message.Content)
	if outputText == "" && len(toolCalls) == 0 {
		return nil, errors.New("model returned empty response")
	}

	return &Response{
		Model:             resp.Model,
		OutputText:        outputText,
		ToolCalls:         toolCalls,
		RequiresToolCalls: len(toolCalls) > 0,
		PromptTokens:      resp.PromptEvalCount,
		CompletionTokens:  resp.EvalCount,
		TotalTokens:       resp.PromptEvalCount + resp.EvalCount,
	}, nil
}

func toolCallsFromChatToolCalls(providerToolCalls []chatToolCall, offset int) []ToolCall {
	if len(providerToolCalls) == 0 {
		return nil
	}

	toolCalls := make([]ToolCall, 0, len(providerToolCalls))
	for idx, providerToolCall := range providerToolCalls {
		name := strings.TrimSpace(providerToolCall.Function.Name)
		if name == "" {
			continue
		}
		toolCalls = append(toolCalls, ToolCall{
			ID:    fmt.Sprintf("ollama_%s_%d", name, offset+idx),
			Name:  name,
			Input: defaultToolArguments(stringFromRawArguments(providerToolCall.Function.Arguments)),
		})
	}

	return toolCalls
}

func stringFromRawArguments(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err == nil {
		return compact.String()
	}

	return string(raw)
}

func defaultToolArguments(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "{}"
	}

	return value
}

func decodeOllamaResponse(resp *http.Response, target any) error {
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ollamaStatusError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode Ollama response: %w", err)
	}

	return nil
}

func ollamaStatusError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	detail := strings.TrimSpace(string(body))
	if detail == "" {
		detail = resp.Status
	}

	return fmt.Errorf("ollama request failed with status %d: %s", resp.StatusCode, detail)
}

func normalizeBaseURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		return "", errors.New("ollama base URL is required")
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("ollama base URL %q is invalid", value)
	}
	if strings.EqualFold(strings.TrimRight(parsed.Path, "/"), "/v1") {
		return "", fmt.Errorf("ollama native API base URL must not include /v1: %s", value)
	}

	return value, nil
}

func normalizeHeaders(headers map[string]string) map[string]string {
	normalized := make(map[string]string)
	for key, value := range headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		normalized[key] = value
	}
	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

func enrichOllamaConnectionError(baseURL string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	return fmt.Errorf("ollama is not reachable at %s; start Ollama or update the base URL: %w", baseURL, err)
}
