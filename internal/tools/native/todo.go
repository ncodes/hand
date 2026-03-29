package native

import (
	"context"
	"strings"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/tools"
)

func TodoDefinition(dependencies envtypes.Runtime) tools.Definition {
	type input struct {
		Action string              `json:"action"`
		Items  []envtypes.TodoItem `json:"items"`
	}
	return tools.Definition{
		Name:        "todo",
		Description: "Manage the in-memory todo list for the current session.",
		Groups:      []string{"core"},
		InputSchema: map[string]any{"type": "object"},
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var req input
			if result := decodeInput(call, &req); result.Error != "" {
				return result, nil
			}
			switch strings.TrimSpace(req.Action) {
			case "", "list":
				return encodeOutput(map[string]any{"items": dependencies.ListTodos()})
			case "replace":
				return encodeOutput(map[string]any{"items": dependencies.ReplaceTodos(req.Items)})
			case "clear":
				return encodeOutput(map[string]any{"items": dependencies.ClearTodos()})
			default:
				return toolError("invalid_input", "unsupported todo action"), nil
			}
		}),
	}
}
