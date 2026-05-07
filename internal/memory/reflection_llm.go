package memory

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"strings"

	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
)

const defaultReflectionMaxOutputTokens int64 = 1600

type LLMReflectionGeneratorOptions struct {
	Client          models.Client
	Model           string
	APIMode         string
	MaxOutputTokens int64
	DebugRequests   bool
}

type LLMReflectionGenerator struct {
	options LLMReflectionGeneratorOptions
}

func NewLLMReflectionGenerator(options LLMReflectionGeneratorOptions) (*LLMReflectionGenerator, error) {
	if options.Client == nil {
		return nil, errors.New("memory reflection model client is required")
	}
	if strings.TrimSpace(options.Model) == "" {
		return nil, errors.New("memory reflection model is required")
	}
	if options.MaxOutputTokens <= 0 {
		options.MaxOutputTokens = defaultReflectionMaxOutputTokens
	}
	return &LLMReflectionGenerator{options: options}, nil
}

func (g *LLMReflectionGenerator) GenerateReflectionCandidates(
	ctx context.Context,
	req ReflectionGenerationRequest,
) (ReflectionGenerationResult, error) {
	if g == nil || g.options.Client == nil {
		return ReflectionGenerationResult{}, errors.New("memory reflection model client is required")
	}

	payload, _ := json.Marshal(reflectionModelPayload(req))
	resp, err := g.options.Client.Complete(ctx, models.Request{
		Model:            g.options.Model,
		APIMode:          g.options.APIMode,
		Instructions:     reflectionInstructions(),
		Messages:         []handmsg.Message{{Role: handmsg.RoleUser, Content: string(payload)}},
		StructuredOutput: reflectionStructuredOutput(),
		MaxOutputTokens:  g.options.MaxOutputTokens,
		DebugRequests:    g.options.DebugRequests,
	})
	if err != nil {
		return ReflectionGenerationResult{}, err
	}

	return parseReflectionModelResponse(resp)
}

type reflectionModelMemory struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	Status     string            `json:"status"`
	Title      string            `json:"title,omitempty"`
	Text       string            `json:"text,omitempty"`
	Tags       []string          `json:"tags,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Confidence float64           `json:"confidence,omitempty"`
}

type reflectionModelRequest struct {
	SessionID string                  `json:"session_id"`
	Sources   []reflectionModelMemory `json:"sources"`
	Related   []reflectionModelMemory `json:"related,omitempty"`
	Limit     int                     `json:"limit"`
}

type reflectionModelResponse struct {
	Candidates []reflectionModelCandidate `json:"candidates"`
}

type reflectionModelCandidate struct {
	Kind       string                         `json:"kind"`
	Title      string                         `json:"title"`
	Text       string                         `json:"text"`
	Tags       []string                       `json:"tags"`
	Confidence float64                        `json:"confidence"`
	Metadata   []reflectionModelMetadataEntry `json:"metadata"`
}

type reflectionModelMetadataEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func reflectionModelPayload(req ReflectionGenerationRequest) reflectionModelRequest {
	return reflectionModelRequest{
		SessionID: strings.TrimSpace(req.SessionID),
		Sources:   reflectionModelMemories(req.Sources),
		Related:   reflectionModelMemories(req.Related),
		Limit:     req.Limit,
	}
}

func reflectionModelMemories(items []MemoryItem) []reflectionModelMemory {
	memories := make([]reflectionModelMemory, 0, len(items))
	for _, item := range items {
		memories = append(memories, reflectionModelMemory{
			ID:         strings.TrimSpace(item.ID),
			Kind:       strings.TrimSpace(string(item.Kind)),
			Status:     strings.TrimSpace(string(item.Status)),
			Title:      strings.TrimSpace(item.Title),
			Text:       strings.TrimSpace(item.Text),
			Tags:       append([]string(nil), item.Tags...),
			Metadata:   cloneMetadata(item.Metadata),
			Confidence: item.Confidence,
		})
	}
	return memories
}

func parseReflectionModelResponse(resp *models.Response) (ReflectionGenerationResult, error) {
	if resp == nil {
		return ReflectionGenerationResult{}, errors.New("memory reflection response is required")
	}

	var parsed reflectionModelResponse
	if err := json.Unmarshal([]byte(normalizedReflectionJSON(resp.OutputText)), &parsed); err != nil {
		return ReflectionGenerationResult{}, err
	}

	items := make([]MemoryItem, 0, len(parsed.Candidates))
	for _, candidate := range parsed.Candidates {
		items = append(items, MemoryItem{
			Kind:       Kind(strings.TrimSpace(candidate.Kind)),
			Status:     StatusCandidate,
			Title:      strings.TrimSpace(candidate.Title),
			Text:       strings.TrimSpace(candidate.Text),
			Tags:       append([]string(nil), candidate.Tags...),
			Metadata:   reflectionMetadataEntries(candidate.Metadata),
			Confidence: candidate.Confidence,
		})
	}

	return ReflectionGenerationResult{Items: items}, nil
}

func normalizedReflectionJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "```") {
		return raw
	}

	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```JSON")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
	return strings.TrimSpace(raw)
}

func reflectionInstructions() string {
	return strings.Join([]string{
		"Reflect over episodic memory evidence and propose durable memory candidates.",
		"Only emit candidates supported by the provided source memories.",
		"Allowed kinds are semantic, procedural, pinned, and episodic.",
		"Every candidate must remain candidate-only; do not request activation, deletion, or supersession.",
		"Prefer durable preferences, corrections, decisions, recurring procedures, and high-signal continuity facts.",
		"Reject low-importance observations, execution details, raw transcript snippets, or temporary task state by omitting them.",
		"Use metadata key/value entries for memory_importance and memory_granularity; avoid low importance and execution_detail granularity.",
	}, "\n")
}

func reflectionStructuredOutput() *models.StructuredOutput {
	return &models.StructuredOutput{
		Name:        "memory_reflection_candidates",
		Description: "Durable memory candidates derived from episodic evidence",
		Strict:      true,
		Schema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"candidates": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]any{
							"kind":       map[string]any{"type": "string", "enum": []string{"semantic", "procedural", "pinned", "episodic"}},
							"title":      map[string]any{"type": "string"},
							"text":       map[string]any{"type": "string"},
							"tags":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
							"confidence": map[string]any{"type": "number"},
							"metadata": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type":                 "object",
									"additionalProperties": false,
									"properties": map[string]any{
										"key":   map[string]any{"type": "string"},
										"value": map[string]any{"type": "string"},
									},
									"required": []string{"key", "value"},
								},
							},
						},
						"required": []string{"kind", "title", "text", "tags", "confidence", "metadata"},
					},
				},
			},
			"required": []string{"candidates"},
		},
	}
}

func reflectionMetadataEntries(entries []reflectionModelMetadataEntry) map[string]string {
	if len(entries) == 0 {
		return nil
	}

	metadata := make(map[string]string, len(entries))
	for _, entry := range entries {
		key := strings.TrimSpace(entry.Key)
		if key != "" {
			metadata[key] = strings.TrimSpace(entry.Value)
		}
	}
	if len(metadata) == 0 {
		return nil
	}

	return metadata
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(metadata))
	maps.Copy(cloned, metadata)
	return cloned
}
