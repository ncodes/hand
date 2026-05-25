package provider_openai

import (
	"context"
	"errors"
	"fmt"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	models "github.com/wandxy/hand/internal/model"
	handmsg "github.com/wandxy/hand/pkg/agent/message"
)

// responsesHandler handles responses requests.
type responsesHandler struct{}

func (responsesHandler) Complete(
	ctx context.Context,
	client *OpenAIClient,
	req normalizedGenerateRequest,
	stream bool,
	onTextDelta func(StreamDelta),
) (*Response, error) {
	params := buildResponsesRequest(req)
	if req.DebugRequests {
		logRequestDebugMetadata(req)
	}

	if stream {
		if client.createResponseStream == nil {
			return nil, errors.New("model client is required")
		}
		return client.completeResponsesStream(ctx, params, onTextDelta)
	}

	if client.createResponse == nil {
		return nil, errors.New("model client is required")
	}
	providerResp, callErr := client.createResponse(ctx, params)
	if callErr != nil {
		return nil, callErr
	}
	if providerResp == nil {
		return nil, errors.New("model response is required")
	}
	return extractResponsesResponse(providerResp)
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
		return StreamDelta{Channel: models.StreamChannelAssistant, Text: event.AsResponseOutputTextDelta().Delta}, nil, nil
	case "response.reasoning_text.delta":
		return StreamDelta{Channel: models.StreamChannelReasoning, Text: event.AsResponseReasoningTextDelta().Delta}, nil, nil
	case "response.reasoning_summary_text.delta":
		return StreamDelta{Channel: models.StreamChannelReasoning, Text: event.AsResponseReasoningSummaryTextDelta().Delta}, nil, nil
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
