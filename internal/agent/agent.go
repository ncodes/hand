package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/wandxy/hand/internal/config"
	handctx "github.com/wandxy/hand/internal/context"
	"github.com/wandxy/hand/internal/environment"
	"github.com/wandxy/hand/internal/instruction"
	"github.com/wandxy/hand/internal/models"
	"github.com/wandxy/hand/internal/tools"
)

var jsonMarshal = json.Marshal

type runtimeEnvironment interface {
	Prepare() error
	Context() environment.Context
	Tools() environment.ToolRegistry
	NewIterationBudget() environment.IterationBudget
}

var newRuntimeEnvironment = func(ctx context.Context, cfg *config.Config) runtimeEnvironment {
	return environment.NewEnvironment(ctx, cfg)
}

// Agent is the runtime agent.
// It coordinates runtime state, model calls, and synchronous tool execution.
type Agent struct {
	ctx         context.Context
	cfg         *config.Config
	modelClient models.Client
	env         runtimeEnvironment
}

// NewAgent creates a new agent with the given configuration and model client.
func NewAgent(ctx context.Context, cfg *config.Config, modelClient models.Client) *Agent {
	return &Agent{
		ctx:         ctx,
		cfg:         cfg,
		modelClient: modelClient,
	}
}

// Chat processes a user message and returns a response.
func (c *Agent) Chat(ctx context.Context, msg string) (string, error) {
	if c == nil {
		return "", errors.New("agent is required")
	}
	if c.cfg == nil {
		return "", errors.New("config is required")
	}
	if c.env == nil {
		return "", errors.New("environment has not been initialized")
	}
	if c.modelClient == nil {
		return "", errors.New("model client is required")
	}
	if c.env.Tools() == nil {
		return "", errors.New("tool registry is required")
	}
	if strings.TrimSpace(msg) == "" {
		return "", errors.New("message is required")
	}
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return "", err
	}

	handCtx := c.env.Context()
	if handCtx == nil {
		return "", errors.New("runtime context is required")
	}
	if err := handCtx.AddUserMessage(msg); err != nil {
		return "", err
	}

	budget := c.env.NewIterationBudget()
	for budget.Consume() {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		resp, err := c.modelClient.Chat(ctx, models.GenerateRequest{
			Model:         c.cfg.Model,
			APIMode:       c.cfg.ModelAPIMode,
			Instructions:  handCtx.GetInstructions().String(),
			Messages:      handCtx.GetMessages(),
			Tools:         c.toolDefinitions(),
			DebugRequests: c.cfg.DebugRequests,
		})
		if err != nil {
			return "", err
		}
		if resp == nil {
			return "", errors.New("model response is required")
		}

		if !resp.RequiresToolCalls {
			if err := handCtx.AddAssistantMessage(resp.OutputText); err != nil {
				return "", err
			}
			return resp.OutputText, nil
		}

		if len(resp.ToolCalls) == 0 {
			return "", errors.New("model requested tool execution without tool calls")
		}

		assistantMessage := handctx.Message{
			Role:      handctx.RoleAssistant,
			Content:   strings.TrimSpace(resp.OutputText),
			ToolCalls: toContextToolCalls(resp.ToolCalls),
		}
		if err := handCtx.AddMessage(assistantMessage); err != nil {
			return "", err
		}

		for _, toolCall := range resp.ToolCalls {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if err := handCtx.AddMessage(c.invokeTool(ctx, toolCall)); err != nil {
				return "", err
			}
		}
	}

	return c.summaryFallback(ctx, budget, handCtx)
}

func (c *Agent) Run(context.Context) error {
	if c == nil {
		return errors.New("agent is required")
	}
	if c.cfg == nil {
		return errors.New("config is required")
	}

	c.env = newRuntimeEnvironment(c.ctx, c.cfg)
	if err := c.env.Prepare(); err != nil {
		return err
	}

	return nil
}

func (c *Agent) Conversation() handctx.Conversation {
	if c == nil {
		return handctx.NewConversation()
	}
	if c.env == nil {
		return handctx.NewConversation()
	}

	return c.env.Context().GetConversation()
}

func (c *Agent) toolDefinitions() []models.ToolDefinition {
	if c == nil || c.env == nil || c.env.Tools() == nil {
		return nil
	}

	definitions := c.env.Tools().List()
	toolsList := make([]models.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		toolsList = append(toolsList, models.ToolDefinition{
			Name:        definition.Name,
			Description: definition.Description,
			InputSchema: definition.InputSchema,
		})
	}
	return toolsList
}

func (c *Agent) invokeTool(ctx context.Context, toolCall models.ToolCall) handctx.Message {
	result := map[string]string{
		"name": toolCall.Name,
	}

	toolResult, err := c.env.Tools().Invoke(ctx, tools.Call{
		Name:   toolCall.Name,
		Input:  toolCall.Input,
		Source: "model",
	})
	if err != nil {
		result["error"] = err.Error()
	}
	if strings.TrimSpace(toolResult.Error) != "" {
		result["error"] = strings.TrimSpace(toolResult.Error)
	}
	if strings.TrimSpace(toolResult.Output) != "" {
		result["output"] = strings.TrimSpace(toolResult.Output)
	}

	raw, marshalErr := jsonMarshal(result)
	content := ""
	if marshalErr != nil {
		content = fmt.Sprintf(`{"name":%q,"error":%q}`, toolCall.Name, marshalErr.Error())
	} else {
		content = string(raw)
	}

	return handctx.Message{
		Role:       handctx.RoleTool,
		Name:       toolCall.Name,
		ToolCallID: toolCall.ID,
		Content:    content,
	}
}

func (c *Agent) summaryFallback(
	ctx context.Context,
	budget environment.IterationBudget,
	handCtx environment.Context,
) (string, error) {
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return "", err
	}

	instructions := instruction.
		BuildBase(c.cfg.Name).
		Chain(instruction.BuildSummary(budget.Remaining())...)

	resp, err := c.modelClient.Chat(ctx, models.GenerateRequest{
		Model:         c.cfg.Model,
		APIMode:       c.cfg.ModelAPIMode,
		Instructions:  instructions.String(),
		Messages:      handCtx.GetMessages(),
		Tools:         nil,
		DebugRequests: c.cfg.DebugRequests,
	})
	if err != nil {
		return "", fmt.Errorf("iteration limit reached and summary failed: %w", err)
	}
	if resp == nil {
		return "", errors.New("model response is required")
	}
	if resp.RequiresToolCalls {
		return "", fmt.Errorf("iteration limit reached and summary requested more tools")
	}
	if err := handCtx.AddAssistantMessage(resp.OutputText); err != nil {
		return "", err
	}

	return resp.OutputText, nil
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func toContextToolCalls(toolCalls []models.ToolCall) []handctx.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}

	normalized := make([]handctx.ToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		normalized = append(normalized, handctx.ToolCall{
			ID:    toolCall.ID,
			Name:  toolCall.Name,
			Input: toolCall.Input,
		})
	}
	return normalized
}
