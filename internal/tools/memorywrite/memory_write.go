package memorywrite

import (
	"context"
	"errors"
	"strings"

	envtypes "github.com/wandxy/hand/internal/environment/types"
	"github.com/wandxy/hand/internal/instructions"
	"github.com/wandxy/hand/internal/memory"
	"github.com/wandxy/hand/internal/tools"
	"github.com/wandxy/hand/internal/tools/common"
)

type sourceLinkInput struct {
	SessionID     string `json:"session_id,omitempty"`
	MessageIDs    []uint `json:"message_ids,omitempty"`
	Offsets       []int  `json:"offsets,omitempty"`
	CreatedBy     string `json:"created_by,omitempty"`
	CreatedReason string `json:"created_reason,omitempty"`
}

type addInput struct {
	Kind            string            `json:"kind"`
	Title           string            `json:"title,omitempty"`
	Text            string            `json:"text,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	Confidence      *float64          `json:"confidence,omitempty"`
	SourceSessionID string            `json:"source_session_id,omitempty"`
	SourceLinks     []sourceLinkInput `json:"source_links,omitempty"`
	Reason          string            `json:"reason,omitempty"`
}

type updateInput struct {
	ID          string   `json:"id"`
	Reason      string   `json:"reason,omitempty"`
	Replacement addInput `json:"replacement"`
}

type deleteInput struct {
	ID     string `json:"id"`
	Reason string `json:"reason,omitempty"`
}

type addOutput struct {
	Candidate memory.MemoryItem        `json:"candidate"`
	Memory    memory.MemoryItem        `json:"memory"`
	Decision  memory.PromotionDecision `json:"decision"`
}

type updateOutput struct {
	Previous    memory.MemoryItem        `json:"previous"`
	Replacement memory.MemoryItem        `json:"replacement"`
	Decision    memory.PromotionDecision `json:"decision"`
}

type deleteOutput struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Deleted bool   `json:"deleted"`
}

func AddDefinition(runtime envtypes.Runtime) tools.Definition {
	return tools.Definition{
		Name:             "memory_add",
		Description:      "Create a source-linked semantic or procedural memory candidate and run it through promotion.",
		Groups:           []string{"core"},
		Requires:         tools.Capabilities{Memory: true},
		UsageInstruction: instructions.BuildMemoryAddGuidance(),
		InputSchema:      addInputSchema(),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var input addInput
			if result := common.DecodeInput(call, &input); result.Error != "" {
				return result, nil
			}
			if runtime == nil {
				return common.ToolError("tool_error", "memory write is not configured"), nil
			}

			item, err := memoryItemFromAddInput(input)
			if err != nil {
				return common.ToolError("invalid_input", err.Error()), nil
			}

			var candidate memory.MemoryItem
			switch item.Kind {
			case memory.KindSemantic:
				candidate, err = runtime.RecordSemanticMemory(ctx, memory.SemanticRecord{Item: item})
			case memory.KindProcedural:
				candidate, err = runtime.RecordProceduralMemory(ctx, memory.ProceduralRecord{Item: item})
			}
			if err != nil {
				return common.ToolError("tool_error", err.Error()), nil
			}

			lifecycle, err := runtime.PromoteMemoryCandidate(ctx, memory.PromotionRequest{
				ID:     candidate.ID,
				Reason: getReason(input.Reason, "tool_memory_add"),
			})
			if err != nil {
				return common.ToolError("tool_error", err.Error()), nil
			}

			return common.EncodeOutput(addOutput{
				Candidate: candidate,
				Memory:    lifecycle.Item,
				Decision:  lifecycle.Decision,
			})
		}),
	}
}

func UpdateDefinition(runtime envtypes.Runtime) tools.Definition {
	return tools.Definition{
		Name:             "memory_update",
		Description:      "Replace an active semantic or procedural memory with a source-linked candidate through lifecycle promotion.",
		Groups:           []string{"core"},
		Requires:         tools.Capabilities{Memory: true},
		UsageInstruction: instructions.BuildMemoryUpdateGuidance(),
		InputSchema:      updateInputSchema(),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var input updateInput
			if result := common.DecodeInput(call, &input); result.Error != "" {
				return result, nil
			}
			if runtime == nil {
				return common.ToolError("tool_error", "memory write is not configured"), nil
			}
			if strings.TrimSpace(input.ID) == "" {
				return common.ToolError("invalid_input", "memory id is required"), nil
			}

			replacement, err := memoryItemFromAddInput(input.Replacement)
			if err != nil {
				return common.ToolError("invalid_input", err.Error()), nil
			}

			result, err := runtime.UpdateMemory(ctx, memory.UpdateRequest{
				ID:          input.ID,
				Reason:      input.Reason,
				Replacement: replacement,
			})
			if err != nil {
				return common.ToolError("tool_error", err.Error()), nil
			}

			return common.EncodeOutput(updateOutput{
				Previous:    result.Previous,
				Replacement: result.Replacement,
				Decision:    result.Lifecycle.Decision,
			})
		}),
	}
}

func DeleteDefinition(runtime envtypes.Runtime) tools.Definition {
	return tools.Definition{
		Name:             "memory_delete",
		Description:      "Delete a durable memory through the memory lifecycle.",
		Groups:           []string{"core"},
		Requires:         tools.Capabilities{Memory: true},
		UsageInstruction: instructions.BuildMemoryDeleteGuidance(),
		InputSchema: common.ObjectSchema(map[string]any{
			"id":     common.StringSchema("Memory id to delete."),
			"reason": common.StringSchema("Concise user-grounded reason for deletion."),
		}, "id"),
		Handler: tools.HandlerFunc(func(ctx context.Context, call tools.Call) (tools.Result, error) {
			var input deleteInput
			if result := common.DecodeInput(call, &input); result.Error != "" {
				return result, nil
			}
			if runtime == nil {
				return common.ToolError("tool_error", "memory write is not configured"), nil
			}
			id := strings.TrimSpace(input.ID)
			if id == "" {
				return common.ToolError("invalid_input", "memory id is required"), nil
			}

			if err := runtime.DeleteMemory(ctx, memory.DeleteRequest{ID: id, Reason: input.Reason}); err != nil {
				return common.ToolError("tool_error", err.Error()), nil
			}

			return common.EncodeOutput(deleteOutput{ID: id, Status: string(memory.StatusDeleted), Deleted: true})
		}),
	}
}

func memoryItemFromAddInput(input addInput) (memory.MemoryItem, error) {
	kind, err := parseKind(input.Kind)
	if err != nil {
		return memory.MemoryItem{}, err
	}

	confidence, err := getConfidence(input.Confidence)
	if err != nil {
		return memory.MemoryItem{}, err
	}

	item := memory.MemoryItem{
		Kind:        kind,
		Title:       strings.TrimSpace(input.Title),
		Text:        strings.TrimSpace(input.Text),
		Tags:        trimStrings(input.Tags),
		Metadata:    cloneMetadata(input.Metadata),
		SourceLinks: sourceLinksFromInput(input.SourceLinks),
		Confidence:  confidence,
	}
	if item.Metadata == nil {
		item.Metadata = make(map[string]string)
	}
	if sessionID := strings.TrimSpace(input.SourceSessionID); sessionID != "" {
		item.Metadata["source_session_id"] = sessionID
	}
	if item.Title == "" && item.Text == "" {
		return memory.MemoryItem{}, errors.New("memory title or text is required")
	}
	if !hasProvenance(item) {
		return memory.MemoryItem{}, errors.New("memory source provenance is required")
	}

	return item, nil
}

func parseKind(value string) (memory.Kind, error) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case string(memory.KindSemantic):
		return memory.KindSemantic, nil
	case string(memory.KindProcedural):
		return memory.KindProcedural, nil
	default:
		return "", errors.New("memory kind must be semantic or procedural")
	}
}

func sourceLinksFromInput(inputs []sourceLinkInput) []memory.SourceLink {
	links := make([]memory.SourceLink, 0, len(inputs))
	for _, input := range inputs {
		link := memory.SourceLink{
			SessionID:     strings.TrimSpace(input.SessionID),
			MessageIDs:    append([]uint(nil), input.MessageIDs...),
			Offsets:       append([]int(nil), input.Offsets...),
			CreatedBy:     strings.TrimSpace(input.CreatedBy),
			CreatedReason: strings.TrimSpace(input.CreatedReason),
		}
		if link.SessionID == "" &&
			len(link.MessageIDs) == 0 &&
			len(link.Offsets) == 0 {
			continue
		}
		links = append(links, link)
	}

	return links
}

func hasProvenance(item memory.MemoryItem) bool {
	if strings.TrimSpace(item.Metadata["source_session_id"]) != "" {
		return true
	}
	for _, link := range item.SourceLinks {
		if strings.TrimSpace(link.SessionID) != "" ||
			len(link.MessageIDs) > 0 ||
			len(link.Offsets) > 0 {
			return true
		}
	}

	return false
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		if key = strings.TrimSpace(key); key != "" {
			cloned[key] = strings.TrimSpace(value)
		}
	}

	return cloned
}

func trimStrings(values []string) []string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			trimmed = append(trimmed, value)
		}
	}

	return trimmed
}

func getReason(value string, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}

	return fallback
}

func getConfidence(value *float64) (float64, error) {
	if value == nil {
		return 1, nil
	}
	if *value < 0 || *value > 1 {
		return 0, errors.New("memory confidence must be between 0 and 1")
	}

	return *value, nil
}

func addInputSchema() map[string]any {
	return common.ObjectSchema(map[string]any{
		"kind":              enumSchema("Memory kind to create.", "semantic", "procedural"),
		"title":             common.StringSchema("Short memory title."),
		"text":              common.StringSchema("Durable memory text."),
		"tags":              stringArraySchema("Optional concise tags."),
		"metadata":          stringMapSchema("Optional string metadata used by admission and provenance."),
		"confidence":        numberSchema("Confidence from 0 to 1. Defaults to 1 for explicit user-directed writes."),
		"source_session_id": common.StringSchema("Source session id when source_links are unavailable."),
		"source_links":      sourceLinksSchema(),
		"reason":            common.StringSchema("Concise user-grounded reason for the write."),
	}, "kind")
}

func updateInputSchema() map[string]any {
	return common.ObjectSchema(map[string]any{
		"id":          common.StringSchema("Existing active memory id to replace."),
		"reason":      common.StringSchema("Concise user-grounded reason for the update."),
		"replacement": addInputSchema(),
	}, "id", "replacement")
}

func sourceLinksSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "Optional source links proving where the write came from.",
		"items": common.ObjectSchema(map[string]any{
			"session_id":     common.StringSchema("Source session id."),
			"message_ids":    integerArraySchema("Source message ids."),
			"offsets":        integerArraySchema("Source message offsets."),
			"created_by":     common.StringSchema("Provenance creator."),
			"created_reason": common.StringSchema("Provenance reason."),
		}),
	}
}

func enumSchema(description string, values ...string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
		"enum":        values,
	}
}

func stringArraySchema(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items":       map[string]any{"type": "string"},
	}
}

func integerArraySchema(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items":       map[string]any{"type": "integer"},
	}
}

func numberSchema(description string) map[string]any {
	return map[string]any{
		"type":        "number",
		"description": description,
	}
}

func stringMapSchema(description string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"description":          description,
		"additionalProperties": map[string]any{"type": "string"},
	}
}
