package model

import agentmodel "github.com/wandxy/hand/pkg/agent/model"

const (
	APIOpenAICompletions = agentmodel.APIOpenAICompletions
	APIOpenAIResponses   = agentmodel.APIOpenAIResponses
	APIAnthropicMessages = agentmodel.APIAnthropicMessages
)

type Client = agentmodel.Client
type StreamChannel = agentmodel.StreamChannel

const (
	StreamChannelAssistant = agentmodel.StreamChannelAssistant
	StreamChannelReasoning = agentmodel.StreamChannelReasoning
)

type StreamDelta = agentmodel.StreamDelta
type Request = agentmodel.Request
type StructuredOutput = agentmodel.StructuredOutput
type Response = agentmodel.Response
type ToolDefinition = agentmodel.ToolDefinition
type ToolCall = agentmodel.ToolCall

var ToolCallsToMessageToolCalls = agentmodel.ToolCallsToMessageToolCalls
