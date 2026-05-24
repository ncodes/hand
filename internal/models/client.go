package models

import agentmodel "github.com/wandxy/hand/pkg/agent/model"

type Client = agentmodel.Client

type StreamChannel = agentmodel.StreamChannel

const (
	StreamChannelAssistant = agentmodel.StreamChannelAssistant
	StreamChannelReasoning = agentmodel.StreamChannelReasoning
)

type StreamDelta = agentmodel.StreamDelta

const (
	APIModeCompletions = agentmodel.APIModeCompletions
	APIModeResponses   = agentmodel.APIModeResponses
)

type Request = agentmodel.Request

type StructuredOutput = agentmodel.StructuredOutput

type Response = agentmodel.Response

type ToolDefinition = agentmodel.ToolDefinition

type ToolCall = agentmodel.ToolCall
