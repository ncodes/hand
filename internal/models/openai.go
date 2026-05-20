package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/rs/zerolog/log"

	handmsg "github.com/wandxy/hand/internal/messages"
)

type OpenAIClient struct {
	createChatCompletion func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)
	createChatStream     func(context.Context, openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk]
	createResponse       func(context.Context, responses.ResponseNewParams) (*responses.Response, error)
	createResponseStream func(context.Context, responses.ResponseNewParams) *ssestream.Stream[responses.ResponseStreamEventUnion]
}

var newOpenAICompletionCaller = func(opts ...option.RequestOption) func(
	context.Context,
	openai.ChatCompletionNewParams,
) (*openai.ChatCompletion, error) {
	client := openai.NewClient(opts...)
	return func(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
		return client.Chat.Completions.New(ctx, params)
	}
}

var newOpenAIResponseCaller = func(opts ...option.RequestOption) func(
	context.Context,
	responses.ResponseNewParams,
) (*responses.Response, error) {
	client := openai.NewClient(opts...)
	return func(ctx context.Context, params responses.ResponseNewParams) (*responses.Response, error) {
		return client.Responses.New(ctx, params)
	}
}

var newOpenAICompletionStreamCaller = func(opts ...option.RequestOption) func(
	context.Context,
	openai.ChatCompletionNewParams,
) *ssestream.Stream[openai.ChatCompletionChunk] {
	client := openai.NewClient(opts...)
	return func(ctx context.Context, params openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
		return client.Chat.Completions.NewStreaming(ctx, params)
	}
}

var newOpenAIResponseStreamCaller = func(opts ...option.RequestOption) func(
	context.Context,
	responses.ResponseNewParams,
) *ssestream.Stream[responses.ResponseStreamEventUnion] {
	client := openai.NewClient(opts...)
	return func(ctx context.Context, params responses.ResponseNewParams) *ssestream.Stream[responses.ResponseStreamEventUnion] {
		return client.Responses.NewStreaming(ctx, params)
	}
}

type normalizedGenerateRequest struct {
	Model            string
	APIMode          string
	Instructions     string
	Messages         []handmsg.Message
	Tools            []ToolDefinition
	StructuredOutput *StructuredOutput
	MaxOutputTokens  int64
	Temperature      float64
	DebugRequests    bool
}

// NewOpenAIClient creates a new OpenAI client.
func NewOpenAIClient(apiKey string, opts ...option.RequestOption) (*OpenAIClient, error) {
	clientOptions := make([]option.RequestOption, 0, len(opts)+1)
	if trimmed := strings.TrimSpace(apiKey); trimmed != "" {
		clientOptions = append(clientOptions, option.WithAPIKey(trimmed))
	}
	clientOptions = append(clientOptions, opts...)

	return &OpenAIClient{
		createChatCompletion: newOpenAICompletionCaller(clientOptions...),
		createChatStream:     newOpenAICompletionStreamCaller(clientOptions...),
		createResponse:       newOpenAIResponseCaller(clientOptions...),
		createResponseStream: newOpenAIResponseStreamCaller(clientOptions...),
	}, nil
}

// Complete sends a request to the configured OpenAI-compatible API mode and returns the normalized response.
func (c *OpenAIClient) Complete(ctx context.Context, req Request) (*Response, error) {
	return c.complete(ctx, req, nil, false)
}

func (c *OpenAIClient) CompleteStream(ctx context.Context, req Request, onTextDelta func(StreamDelta)) (*Response, error) {
	return c.complete(ctx, req, onTextDelta, true)
}

func (c *OpenAIClient) complete(
	ctx context.Context,
	req Request,
	onTextDelta func(StreamDelta),
	stream bool,
) (resp *Response, err error) {
	if c == nil {
		return nil, errors.New("model client is required")
	}

	normalizedReq, err := normalizeGenerateRequest(req)
	if err != nil {
		return nil, err
	}
	logModelClientRequestStarted(normalizedReq, stream)
	defer func() {
		if err != nil {
			logModelClientRequestFailed(normalizedReq, stream, err)
			return
		}
		logModelClientRequestCompleted(normalizedReq, stream, resp)
	}()

	if normalizedReq.APIMode == APIModeResponses {
		params := buildResponsesRequest(normalizedReq)
		if normalizedReq.DebugRequests {
			logRequestDebugMetadata(normalizedReq.APIMode)
		}

		if stream {
			if c.createResponseStream == nil {
				return nil, errors.New("model client is required")
			}
			return c.completeResponsesStream(ctx, params, onTextDelta)
		}

		if c.createResponse == nil {
			return nil, errors.New("model client is required")
		}
		providerResp, callErr := c.createResponse(ctx, params)
		if callErr != nil {
			return nil, callErr
		}
		if providerResp == nil {
			return nil, errors.New("model response is required")
		}
		return extractResponsesResponse(providerResp)
	}

	params := buildChatCompletionsRequest(normalizedReq)
	if stream {
		params.StreamOptions = openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		}
	}
	if normalizedReq.DebugRequests {
		logRequestDebugMetadata(normalizedReq.APIMode)
	}

	if stream {
		if c.createChatStream == nil {
			return nil, errors.New("model client is required")
		}
		return c.completeChatStream(ctx, params, onTextDelta)
	}

	if c.createChatCompletion == nil {
		return nil, errors.New("model client is required")
	}
	providerResp, callErr := c.createChatCompletion(ctx, params)
	if callErr != nil {
		return nil, callErr
	}
	if providerResp == nil {
		return nil, errors.New("model response is required")
	}
	return extractChatCompletionsResponse(providerResp)
}

func (c *OpenAIClient) completeChatStream(
	ctx context.Context,
	params openai.ChatCompletionNewParams,
	onTextDelta func(StreamDelta),
) (*Response, error) {
	stream := c.createChatStream(ctx, params)
	if stream == nil {
		return nil, errors.New("model response is required")
	}

	acc := openai.ChatCompletionAccumulator{}
	for stream.Next() {
		chunk := stream.Current()
		if !acc.AddChunk(chunk) {
			return nil, errors.New("failed to accumulate chat completion stream")
		}

		if onTextDelta != nil {
			for _, choice := range chunk.Choices {
				if reasoning := getChatStreamReasoningDelta(choice.Delta); reasoning != "" {
					onTextDelta(StreamDelta{Channel: StreamChannelReasoning, Text: reasoning})
				}
				if choice.Delta.Content != "" {
					onTextDelta(StreamDelta{Channel: StreamChannelAssistant, Text: choice.Delta.Content})
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}

	return extractChatCompletionsResponse(&acc.ChatCompletion)
}

func getChatStreamReasoningDelta(delta openai.ChatCompletionChunkChoiceDelta) string {
	for _, key := range []string{"reasoning", "reasoning_content", "reasoning_text"} {
		field, ok := delta.JSON.ExtraFields[key]
		if !ok || !field.Valid() {
			continue
		}

		var text string
		if err := json.Unmarshal([]byte(field.Raw()), &text); err == nil {
			return text
		}
	}

	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(delta.RawJSON()), &rawFields); err != nil {
		return ""
	}
	for _, key := range []string{"reasoning", "reasoning_content", "reasoning_text"} {
		raw, ok := rawFields[key]
		if !ok {
			continue
		}

		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			return text
		}
	}

	return ""
}

func (c *OpenAIClient) completeResponsesStream(
	ctx context.Context,
	params responses.ResponseNewParams,
	onTextDelta func(StreamDelta),
) (*Response, error) {
	stream := c.createResponseStream(ctx, params)
	if stream == nil {
		return nil, errors.New("model response is required")
	}

	var finalResponse *responses.Response
	for stream.Next() {
		event := stream.Current()
		if textDelta, terminalResponse, err := handleResponsesStreamEvent(event); err != nil {
			return nil, err
		} else {
			if onTextDelta != nil && textDelta.Text != "" {
				onTextDelta(textDelta)
			}
			if terminalResponse != nil {
				finalResponse = terminalResponse
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	if finalResponse == nil {
		return nil, errors.New("model response is required")
	}

	return extractResponsesResponse(finalResponse)
}

func handleResponsesStreamEvent(event responses.ResponseStreamEventUnion) (StreamDelta, *responses.Response, error) {
	switch event.Type {
	case "response.output_text.delta":
		return StreamDelta{Channel: StreamChannelAssistant, Text: event.AsResponseOutputTextDelta().Delta}, nil, nil
	case "response.reasoning_text.delta":
		return StreamDelta{Channel: StreamChannelReasoning, Text: event.AsResponseReasoningTextDelta().Delta}, nil, nil
	case "response.reasoning_summary_text.delta":
		return StreamDelta{Channel: StreamChannelReasoning, Text: event.AsResponseReasoningSummaryTextDelta().Delta}, nil, nil
	case "response.completed":
		completed := event.AsResponseCompleted()
		return StreamDelta{}, &completed.Response, nil
	case "response.failed":
		failed := event.AsResponseFailed()
		return StreamDelta{}, &failed.Response, nil
	case "response.incomplete":
		incomplete := event.AsResponseIncomplete()
		return StreamDelta{}, &incomplete.Response, nil
	case "error":
		apierr := event.AsError()
		message := strings.TrimSpace(apierr.Message)
		if message == "" {
			message = "response failed"
		}
		return StreamDelta{}, nil, errors.New(message)
	default:
		return StreamDelta{}, nil, nil
	}
}

func normalizeGenerateRequest(req Request) (normalizedGenerateRequest, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return normalizedGenerateRequest{}, errors.New("model is required")
	}

	if len(req.Messages) == 0 {
		return normalizedGenerateRequest{}, errors.New("messages are required")
	}

	messages, err := normalizeMessages(req.Messages)
	if err != nil {
		return normalizedGenerateRequest{}, err
	}
	tools, err := normalizeToolDefinitions(req.Tools)
	if err != nil {
		return normalizedGenerateRequest{}, err
	}

	mode := strings.TrimSpace(strings.ToLower(req.APIMode))
	if mode == "" {
		mode = APIModeCompletions
	}
	if mode != APIModeCompletions && mode != APIModeResponses {
		return normalizedGenerateRequest{}, errors.New("model api mode must be one of: completions, responses")
	}

	return normalizedGenerateRequest{
		Model:            model,
		APIMode:          mode,
		Instructions:     strings.TrimSpace(req.Instructions),
		Messages:         messages,
		Tools:            tools,
		StructuredOutput: normalizeStructuredOutput(req.StructuredOutput),
		MaxOutputTokens:  req.MaxOutputTokens,
		Temperature:      req.Temperature,
		DebugRequests:    req.DebugRequests,
	}, nil
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

func normalizeMessages(messages []handmsg.Message) ([]handmsg.Message, error) {
	normalized := make([]handmsg.Message, 0, len(messages))
	for _, message := range messages {
		role := handmsg.Role(strings.TrimSpace(strings.ToLower(string(message.Role))))
		content := strings.TrimSpace(message.Content)
		toolCallID := strings.TrimSpace(message.ToolCallID)
		toolCalls, err := normalizeToolCalls(message.ToolCalls)
		if err != nil {
			return nil, err
		}

		switch role {
		case handmsg.RoleDeveloper:
			return nil, errors.New("developer messages must be provided via instructions")
		case handmsg.RoleUser, handmsg.RoleAssistant, handmsg.RoleTool:
		default:
			return nil, errors.New("message role must be one of user, assistant, or tool; developer messages must be provided via instructions")
		}

		if content == "" && !(role == handmsg.RoleAssistant && len(toolCalls) > 0) {
			return nil, errors.New("message content is required")
		}
		if role == handmsg.RoleTool && toolCallID == "" {
			return nil, errors.New("tool call id is required")
		}

		normalized = append(normalized, handmsg.Message{
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
			Name:        name,
			Description: strings.TrimSpace(definition.Description),
			InputSchema: definition.InputSchema,
		})
	}

	return normalized, nil
}

func buildChatCompletionsRequest(req normalizedGenerateRequest) openai.ChatCompletionNewParams {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)
	if req.Instructions != "" {
		messages = append(messages, openai.DeveloperMessage(req.Instructions))
	}

	for _, message := range req.Messages {
		switch message.Role {
		case handmsg.RoleUser:
			messages = append(messages, openai.UserMessage(message.Content))
		case handmsg.RoleAssistant:
			if len(message.ToolCalls) == 0 {
				messages = append(messages, openai.AssistantMessage(message.Content))
				continue
			}

			toolCalls := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(message.ToolCalls))
			for _, toolCall := range message.ToolCalls {
				toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
						ID: toolCall.ID,
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      toolCall.Name,
							Arguments: toolCall.Input,
						},
					},
				})
			}

			assistant := openai.ChatCompletionAssistantMessageParam{ToolCalls: toolCalls}
			if message.Content != "" {
				assistant.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: openai.String(message.Content),
				}
			}
			messages = append(messages, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant})
		case handmsg.RoleTool:
			messages = append(messages, openai.ToolMessage(message.Content, message.ToolCallID))
		}
	}

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(req.Model),
		Messages: messages,
	}
	if len(req.Tools) > 0 {
		params.Tools = buildChatCompletionsTools(req.Tools)
	}
	if req.MaxOutputTokens > 0 {
		params.MaxCompletionTokens = openai.Int(req.MaxOutputTokens)
	}
	if req.Temperature > 0 {
		params.Temperature = openai.Float(req.Temperature)
	}
	if req.StructuredOutput != nil {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:        req.StructuredOutput.Name,
					Description: openai.String(req.StructuredOutput.Description),
					Schema:      req.StructuredOutput.Schema,
					Strict:      openai.Bool(req.StructuredOutput.Strict),
				},
			},
		}
	}

	return params
}

func buildResponsesRequest(req normalizedGenerateRequest) responses.ResponseNewParams {
	items := make(responses.ResponseInputParam, 0, len(req.Messages)*2)
	assistantIndex := 0

	for _, message := range req.Messages {
		switch message.Role {
		case handmsg.RoleDeveloper, handmsg.RoleUser:
			items = append(items, responses.ResponseInputItemParamOfInputMessage(
				responses.ResponseInputMessageContentListParam{
					{
						OfInputText: &responses.ResponseInputTextParam{Text: message.Content},
					},
				},
				string(message.Role),
			))
		case handmsg.RoleAssistant:
			if message.Content != "" {
				assistantIndex++
				items = append(items, responses.ResponseInputItemParamOfOutputMessage(
					[]responses.ResponseOutputMessageContentUnionParam{{
						OfOutputText: &responses.ResponseOutputTextParam{
							Annotations: []responses.ResponseOutputTextAnnotationUnionParam{},
							Text:        message.Content,
						},
					}},
					fmt.Sprintf("assistant_%d", assistantIndex),
					responses.ResponseOutputMessageStatusCompleted,
				))
			}
			for _, toolCall := range message.ToolCalls {
				items = append(items, responses.ResponseInputItemParamOfFunctionCall(toolCall.Input, toolCall.ID, toolCall.Name))
			}
		case handmsg.RoleTool:
			items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(message.ToolCallID, message.Content))
		}
	}

	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(req.Model),
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: items},
	}
	if req.Instructions != "" {
		params.Instructions = openai.String(req.Instructions)
	}
	if len(req.Tools) > 0 {
		params.Tools = buildResponsesTools(req.Tools)
	}
	if req.MaxOutputTokens > 0 {
		params.MaxOutputTokens = openai.Int(req.MaxOutputTokens)
	}
	if req.Temperature > 0 {
		params.Temperature = openai.Float(req.Temperature)
	}
	if req.StructuredOutput != nil {
		params.Text = responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
					Name:        req.StructuredOutput.Name,
					Schema:      req.StructuredOutput.Schema,
					Description: openai.String(req.StructuredOutput.Description),
					Strict:      openai.Bool(req.StructuredOutput.Strict),
				},
			},
		}
	}

	return params
}

func extractChatCompletionsResponse(resp *openai.ChatCompletion) (*Response, error) {
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("chat completion response %q contained no choices", resp.ID)
	}

	message := resp.Choices[0].Message
	toolCalls, err := extractChatCompletionsToolCalls(message.ToolCalls)
	if err != nil {
		return nil, err
	}

	outputText := strings.TrimSpace(message.Content)
	if outputText == "" {
		outputText = strings.TrimSpace(message.Refusal)
	}
	if outputText == "" && len(toolCalls) == 0 {
		return nil, errors.New("model returned empty response")
	}

	return &Response{
		ID:                resp.ID,
		Model:             resp.Model,
		OutputText:        outputText,
		ToolCalls:         toolCalls,
		RequiresToolCalls: len(toolCalls) > 0,
		PromptTokens:      int(resp.Usage.PromptTokens),
		CompletionTokens:  int(resp.Usage.CompletionTokens),
		TotalTokens:       int(resp.Usage.TotalTokens),
	}, nil
}

func extractResponsesResponse(resp *responses.Response) (*Response, error) {
	var toolCalls []ToolCall
	for idx, item := range resp.Output {
		if item.Type != "function_call" {
			continue
		}
		functionCall := item.AsFunctionCall()
		callID := strings.TrimSpace(functionCall.CallID)
		name := strings.TrimSpace(functionCall.Name)
		if name == "" {
			return nil, errors.New("tool call name is required")
		}
		if callID == "" {
			callID = getFallbackToolCallID(name, idx)
		}
		toolCalls = append(toolCalls, ToolCall{
			ID:    callID,
			Name:  name,
			Input: strings.TrimSpace(functionCall.Arguments),
		})
	}

	outputText := strings.TrimSpace(resp.OutputText())
	switch resp.Status {
	case "", responses.ResponseStatusCompleted:
	case responses.ResponseStatusFailed:
		message := strings.TrimSpace(resp.Error.Message)
		if message == "" {
			message = "response failed"
		}

		return nil, errors.New(message)
	case responses.ResponseStatusIncomplete:
		if outputText == "" && len(toolCalls) == 0 {
			reason := strings.TrimSpace(resp.IncompleteDetails.Reason)
			if reason == "" {
				reason = "unknown"
			}

			return nil, fmt.Errorf("response incomplete: %s", reason)
		}
	default:
		return nil, fmt.Errorf("response status is %s", resp.Status)
	}

	return &Response{
		ID:                resp.ID,
		Model:             string(resp.Model),
		OutputText:        outputText,
		ToolCalls:         toolCalls,
		RequiresToolCalls: len(toolCalls) > 0,
		PromptTokens:      int(resp.Usage.InputTokens),
		CompletionTokens:  int(resp.Usage.OutputTokens),
		TotalTokens:       int(resp.Usage.TotalTokens),
	}, nil
}

func normalizeToolCalls(toolCalls []handmsg.ToolCall) ([]handmsg.ToolCall, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}

	normalized := make([]handmsg.ToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		id := strings.TrimSpace(toolCall.ID)
		name := strings.TrimSpace(toolCall.Name)
		if id == "" {
			return nil, errors.New("tool call id is required")
		}
		if name == "" {
			return nil, errors.New("tool call name is required")
		}
		normalized = append(normalized, handmsg.ToolCall{
			ID:    id,
			Name:  name,
			Input: strings.TrimSpace(toolCall.Input),
		})
	}

	return normalized, nil
}

func buildChatCompletionsTools(definitions []ToolDefinition) []openai.ChatCompletionToolUnionParam {
	tools := make([]openai.ChatCompletionToolUnionParam, 0, len(definitions))
	for _, definition := range definitions {
		tools = append(tools, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        definition.Name,
			Description: openai.String(definition.Description),
			Parameters:  shared.FunctionParameters(normalizeStrictJSONSchema(definition.InputSchema)),
			Strict:      openai.Bool(true),
		}))
	}

	return tools
}

func buildResponsesTools(definitions []ToolDefinition) []responses.ToolUnionParam {
	tools := make([]responses.ToolUnionParam, 0, len(definitions))
	for _, definition := range definitions {
		tools = append(tools, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        definition.Name,
				Description: openai.String(definition.Description),
				Parameters:  normalizeStrictJSONSchema(definition.InputSchema),
				Strict:      openai.Bool(true),
			},
		})
	}

	return tools
}

func normalizeStrictJSONSchema(schema map[string]any) map[string]any {
	if len(schema) == 0 {
		return nil
	}

	return normalizeStrictJSONSchemaValue(schema).(map[string]any)
}

func normalizeStrictJSONSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, item := range typed {
			cloned[key] = normalizeStrictJSONSchemaValue(item)
		}

		schemaType, _ := cloned["type"].(string)
		properties, _ := cloned["properties"].(map[string]any)
		if schemaType == "object" && len(properties) > 0 {
			for key, property := range properties {
				if propertySchema, ok := property.(map[string]any); ok && isUnsupportedStrictJSONObjectProperty(propertySchema) {
					delete(properties, key)
				}
			}

			required := make([]string, 0, len(properties))
			for key := range properties {
				required = append(required, key)
			}
			slices.Sort(required)
			cloned["required"] = required
		}

		return cloned
	case []any:
		cloned := make([]any, 0, len(typed))
		for _, item := range typed {
			cloned = append(cloned, normalizeStrictJSONSchemaValue(item))
		}
		return cloned
	default:
		return value
	}
}

func isUnsupportedStrictJSONObjectProperty(schema map[string]any) bool {
	if len(schema) == 0 {
		return false
	}

	schemaType, _ := schema["type"].(string)
	if schemaType != "object" {
		return false
	}

	properties, _ := schema["properties"].(map[string]any)
	if len(properties) > 0 {
		return false
	}

	additionalProperties, ok := schema["additionalProperties"]
	if !ok {
		return false
	}

	switch typed := additionalProperties.(type) {
	case bool:
		return typed
	case map[string]any:
		return true
	default:
		return true
	}
}

func extractChatCompletionsToolCalls(toolCalls []openai.ChatCompletionMessageToolCallUnion) ([]ToolCall, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}

	normalized := make([]ToolCall, 0, len(toolCalls))
	for idx, toolCall := range toolCalls {
		id := strings.TrimSpace(toolCall.ID)
		name := strings.TrimSpace(toolCall.Function.Name)
		if name == "" {
			return nil, errors.New("tool call name is required")
		}
		if id == "" {
			id = getFallbackToolCallID(name, idx)
		}

		normalized = append(normalized, ToolCall{
			ID:    id,
			Name:  name,
			Input: strings.TrimSpace(toolCall.Function.Arguments),
		})
	}

	return normalized, nil
}

func getFallbackToolCallID(name string, index int) string {
	return "functions." + strings.TrimSpace(name) + ":" + strconv.Itoa(index)
}

func logModelClientRequestStarted(req normalizedGenerateRequest, stream bool) {
	log.Debug().
		Str("event", "model client request started").
		Str("target", "openai_compatible_api").
		Str("provider", "openai-compatible").
		Str("mode", req.APIMode).
		Str("model", req.Model).
		Bool("stream", stream).
		Int("message_count", len(req.Messages)).
		Int("tool_count", len(req.Tools)).
		Bool("structured_output", req.StructuredOutput != nil).
		Int64("max_output_tokens", req.MaxOutputTokens).
		Msg("model client request started")
}

func logModelClientRequestCompleted(req normalizedGenerateRequest, stream bool, resp *Response) {
	event := log.Debug().
		Str("event", "model client request completed").
		Str("target", "openai_compatible_api").
		Str("provider", "openai-compatible").
		Str("mode", req.APIMode).
		Str("model", req.Model).
		Bool("stream", stream)
	if resp != nil {
		event = event.
			Str("response_model", resp.Model).
			Int("prompt_tokens", resp.PromptTokens).
			Int("completion_tokens", resp.CompletionTokens).
			Int("total_tokens", resp.TotalTokens).
			Int("tool_call_count", len(resp.ToolCalls)).
			Bool("requires_tool_calls", resp.RequiresToolCalls)
	}
	event.Msg("model client request completed")
}

func logModelClientRequestFailed(req normalizedGenerateRequest, stream bool, err error) {
	log.Debug().
		Str("event", "model client request failed").
		Str("target", "openai_compatible_api").
		Str("provider", "openai-compatible").
		Str("mode", req.APIMode).
		Str("model", req.Model).
		Bool("stream", stream).
		Str("error_kind", getModelClientErrorKind(err)).
		Msg("model client request failed")
}

func getModelClientErrorKind(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}

	value := strings.ToLower(err.Error())
	switch {
	case strings.Contains(value, "response is required"):
		return "missing_response"
	case strings.Contains(value, "failed to accumulate") ||
		strings.Contains(value, "stream failed") ||
		strings.Contains(value, "stream error") ||
		strings.Contains(value, "stream response"):
		return "stream_failed"
	case strings.Contains(value, "tool"):
		return "tool_call_failed"
	case strings.Contains(value, "json"):
		return "decode_failed"
	case strings.Contains(value, "timeout"):
		return "timeout"
	default:
		return "operation_failed"
	}
}

func logRequestDebugMetadata(mode string) {
	log.Debug().
		Str("provider", "openai-compatible").
		Str("mode", mode).
		Msg("model request debug metadata")
}
