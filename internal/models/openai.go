package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/rs/zerolog/log"

	"github.com/wandxy/hand/internal/guardrails"
	handmsg "github.com/wandxy/hand/internal/messages"
)

var jsonMarshal = json.Marshal
var debugRedactor = guardrails.NewRedactor()

type OpenAIClient struct {
	createChatCompletion func(context.Context, openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)
	createResponse       func(context.Context, responses.ResponseNewParams) (*responses.Response, error)
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

type normalizedGenerateRequest struct {
	Model           string
	APIMode         string
	Instructions    string
	Messages        []handmsg.Message
	Tools           []ToolDefinition
	MaxOutputTokens int64
	Temperature     float64
	DebugRequests   bool
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
		createResponse:       newOpenAIResponseCaller(clientOptions...),
	}, nil
}

// Chat sends a chat message to the configured OpenAI-compatible API mode and returns the normalized response.
func (c *OpenAIClient) Chat(ctx context.Context, req Request) (*Response, error) {
	if c == nil {
		return nil, errors.New("model client is required")
	}

	normalized, err := normalizeGenerateRequest(req)
	if err != nil {
		return nil, err
	}

	if normalized.APIMode == APIModeResponses {
		if c.createResponse == nil {
			return nil, errors.New("model client is required")
		}

		params := buildResponsesRequest(normalized)
		if normalized.DebugRequests {
			logRequestDebugDump(normalized.APIMode, params)
		}

		resp, err := c.createResponse(ctx, params)
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return nil, errors.New("model response is required")
		}
		return extractResponsesResponse(resp)
	}

	if c.createChatCompletion == nil {
		return nil, errors.New("model client is required")
	}

	params := buildChatCompletionsRequest(normalized)
	if normalized.DebugRequests {
		logRequestDebugDump(normalized.APIMode, params)
	}

	resp, err := c.createChatCompletion(ctx, params)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("model response is required")
	}
	return extractChatCompletionsResponse(resp)
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
		mode = APIModeChatCompletions
	}
	if mode != APIModeChatCompletions && mode != APIModeResponses {
		return normalizedGenerateRequest{}, errors.New("model api mode must be one of: chat-completions, responses")
	}

	return normalizedGenerateRequest{
		Model:           model,
		APIMode:         mode,
		Instructions:    strings.TrimSpace(req.Instructions),
		Messages:        messages,
		Tools:           tools,
		MaxOutputTokens: req.MaxOutputTokens,
		Temperature:     req.Temperature,
		DebugRequests:   req.DebugRequests,
	}, nil
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
	}, nil
}

func extractResponsesResponse(resp *responses.Response) (*Response, error) {
	var toolCalls []ToolCall
	for _, item := range resp.Output {
		if item.Type != "function_call" {
			continue
		}
		functionCall := item.AsFunctionCall()
		callID := strings.TrimSpace(functionCall.CallID)
		name := strings.TrimSpace(functionCall.Name)
		if callID == "" {
			return nil, errors.New("tool call id is required")
		}
		if name == "" {
			return nil, errors.New("tool call name is required")
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
			Parameters:  shared.FunctionParameters(definition.InputSchema),
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
				Parameters:  definition.InputSchema,
				Strict:      openai.Bool(true),
			},
		})
	}
	return tools
}

func extractChatCompletionsToolCalls(toolCalls []openai.ChatCompletionMessageToolCallUnion) ([]ToolCall, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}

	normalized := make([]ToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		id := strings.TrimSpace(toolCall.ID)
		name := strings.TrimSpace(toolCall.Function.Name)
		if id == "" {
			return nil, errors.New("tool call id is required")
		}
		if name == "" {
			return nil, errors.New("tool call name is required")
		}

		normalized = append(normalized, ToolCall{
			ID:    id,
			Name:  name,
			Input: strings.TrimSpace(toolCall.Function.Arguments),
		})
	}

	return normalized, nil
}

func logRequestDebugDump(mode string, params any) {
	raw, err := jsonMarshal(debugRedactor.Sanitize(params))
	if err != nil {
		log.Debug().Err(err).Msg("Failed to marshal model request debug dump")
		return
	}

	log.Debug().
		Str("provider", "openai-compatible").
		Str("mode", mode).
		RawJSON("request", raw).
		Msg("model request debug dump")
}
