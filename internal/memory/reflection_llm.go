package memory

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"strings"

	models "github.com/wandxy/morph/internal/model"
	morphmsg "github.com/wandxy/morph/pkg/agent/message"
)

const defaultReflectionMaxOutputTokens int64 = 1600

// LLMReflectionGeneratorOptions wires the reflection generator to the same
// model abstraction used by chat and episodic extraction. DebugRequests is
// forwarded so reflection prompts can be inspected with the rest of model I/O.
type LLMReflectionGeneratorOptions struct {
	Client                 models.Client
	Model                  string
	API                    string
	MaxOutputTokens        int64
	MaxOutputTokensEnabled *bool
	DebugRequests          bool
}

// LLMReflectionGenerator is a proposal generator. It deliberately returns
// MemoryItems without persistence side effects; the provider validates and
// writes them after adding trusted provenance.
type LLMReflectionGenerator struct {
	options LLMReflectionGeneratorOptions
}

// NewLLMReflectionGenerator returns a reflection generator backed by a model client.
func NewLLMReflectionGenerator(options LLMReflectionGeneratorOptions) (*LLMReflectionGenerator, error) {
	if options.Client == nil {
		return nil, errors.New("memory reflection model client is required")
	}
	if strings.TrimSpace(options.Model) == "" {
		return nil, errors.New("memory reflection model is required")
	}
	if options.MaxOutputTokensEnabled != nil && !*options.MaxOutputTokensEnabled {
		options.MaxOutputTokens = 0
	} else if options.MaxOutputTokens <= 0 {
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

	// The model receives compact JSON rather than transcript-style prose so the
	// prompt shape stays stable and structured-output validation can be strict.
	payload, _ := json.Marshal(reflectionGenerationRequestToModelPayload(req))
	resp, err := g.options.Client.Complete(ctx, models.Request{
		Model:            g.options.Model,
		API:              g.options.API,
		Instructions:     getReflectionInstructions(),
		Messages:         []morphmsg.Message{{Role: morphmsg.RoleUser, Content: string(payload)}},
		StructuredOutput: getReflectionStructuredOutput(),
		MaxOutputTokens:  g.options.MaxOutputTokens,
		DebugRequests:    g.options.DebugRequests,
	})
	if err != nil {
		return ReflectionGenerationResult{}, err
	}

	return reflectionModelResponseToGenerationResult(resp)
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
	Procedural reflectionModelProcedural      `json:"procedural"`
}

type reflectionModelMetadataEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type reflectionModelProcedural struct {
	Trigger          string   `json:"trigger"`
	Steps            []string `json:"steps"`
	Constraints      []string `json:"constraints"`
	Examples         []string `json:"examples"`
	ExpectedBehavior string   `json:"expected_behavior"`
}

func reflectionGenerationRequestToModelPayload(req ReflectionGenerationRequest) reflectionModelRequest {
	return reflectionModelRequest{
		SessionID: strings.TrimSpace(req.SessionID),
		Sources:   memoryItemsToReflectionModelMemories(req.Sources),
		Related:   memoryItemsToReflectionModelMemories(req.Related),
		Limit:     req.Limit,
	}
}

func memoryItemsToReflectionModelMemories(items []MemoryItem) []reflectionModelMemory {
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

func reflectionModelResponseToGenerationResult(resp *models.Response) (ReflectionGenerationResult, error) {
	if resp == nil {
		return ReflectionGenerationResult{}, errors.New("memory reflection response is required")
	}

	var parsed reflectionModelResponse
	if err := json.Unmarshal([]byte(normalizeReflectionJSON(resp.OutputText)), &parsed); err != nil {
		return ReflectionGenerationResult{}, err
	}

	items := make([]MemoryItem, 0, len(parsed.Candidates))
	for _, candidate := range parsed.Candidates {
		metadata := reflectionMetadataEntriesToMap(candidate.Metadata)
		kind := memoryKindFromReflectionCandidate(candidate)
		if kind == KindProcedural {
			metadata = setProceduralReflectionMetadata(metadata, candidate.Procedural)
		}

		// Candidate IDs, source links, reflection tags, and provenance metadata are
		// intentionally absent here. The provider reconstructs them from trusted
		// source memories before writing.
		items = append(items, MemoryItem{
			Kind:       kind,
			Status:     StatusCandidate,
			Title:      strings.TrimSpace(candidate.Title),
			Text:       strings.TrimSpace(candidate.Text),
			Tags:       append([]string(nil), candidate.Tags...),
			Metadata:   metadata,
			Confidence: candidate.Confidence,
		})
	}

	return ReflectionGenerationResult{Items: items}, nil
}

func memoryKindFromReflectionCandidate(candidate reflectionModelCandidate) Kind {
	if hasProceduralReflectionFields(candidate.Procedural) {
		return KindProcedural
	}

	return Kind(strings.TrimSpace(candidate.Kind))
}

func hasProceduralReflectionFields(procedural reflectionModelProcedural) bool {
	return strings.TrimSpace(procedural.Trigger) != "" ||
		len(getNonBlankStrings(procedural.Steps)) > 0
}

func normalizeReflectionJSON(raw string) string {
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

// getReflectionInstructions tells the model what to omit as much as what to emit.
// The provider still enforces these requirements, but prompt-level guidance
// reduces noisy candidate proposals before validation.
func getReflectionInstructions() string {
	return strings.Join([]string{
		"Reflect over episodic memory evidence and propose durable memory candidates.",
		"Return JSON only with a candidates array matching the expected schema.",
		"Only emit candidates supported by the provided source memories.",
		"Allowed kinds are semantic, procedural, pinned, and episodic.",
		"Every candidate must remain candidate-only; do not request activation, deletion, or supersession.",
		"Prefer durable preferences, corrections, decisions, recurring procedures, and high-signal continuity facts.",
		"Reject low-importance observations, execution details, raw transcript snippets, or temporary task state by omitting them.",
		"Use semantic for stable user facts, durable preferences, identity details, project facts, and domain knowledge.",
		"When multiple episodic sources corroborate a stable user fact or preference and related memory does not already capture it, emit a semantic candidate.",
		"Use procedural only for reusable workflows, ordered steps, methods, policies, or instructions the assistant should follow.",
		"Use pinned only when the source explicitly requests an always-loaded standing instruction or critical persistent constraint.",
		"Procedural candidates must be written as reusable instructions, not summaries that a process exists.",
		"Procedural text must preserve the trigger, ordered steps, constraints, important examples, and expected behavior when those details are present in the source memories.",
		"Every candidate must include the typed procedural object. For non-procedural candidates, use empty procedural fields.",
		"For non-procedural candidates, procedural.trigger, procedural.expected_behavior, and every procedural array must be empty.",
		"For procedural candidates, fill procedural.trigger and procedural.steps. Also fill procedural.constraints, procedural.examples, and procedural.expected_behavior when present in the source memories.",
		"Tags must be short machine labels, not sentences or descriptive phrases.",
		"Use lowercase kebab-case tags with one to three words, such as daemon-log, review, or workflow.",
		"Do not include tags that repeat the full title or summarize the whole memory.",
		"Use metadata key/value entries for memory_importance and memory_granularity; avoid low importance and execution_detail granularity.",
	}, "\n")
}

// getReflectionStructuredOutput keeps reflection responses machine-checkable. The
// metadata array is used instead of an open object because strict schemas cannot
// allow arbitrary properties.
func getReflectionStructuredOutput() *models.StructuredOutput {
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
							"procedural": map[string]any{
								"type":                 "object",
								"additionalProperties": false,
								"properties": map[string]any{
									"trigger":           map[string]any{"type": "string"},
									"steps":             map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
									"constraints":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
									"examples":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
									"expected_behavior": map[string]any{"type": "string"},
								},
								"required": []string{"trigger", "steps", "constraints", "examples", "expected_behavior"},
							},
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
						"required": []string{"kind", "title", "text", "tags", "confidence", "metadata", "procedural"},
					},
				},
			},
			"required": []string{"candidates"},
		},
	}
}

func setProceduralReflectionMetadata(
	metadata map[string]string,
	procedural reflectionModelProcedural,
) map[string]string {
	values := map[string]string{
		"procedural_trigger":           strings.TrimSpace(procedural.Trigger),
		"procedural_steps":             strings.Join(getNonBlankStrings(procedural.Steps), "; "),
		"procedural_constraints":       strings.Join(getNonBlankStrings(procedural.Constraints), "; "),
		"procedural_examples":          strings.Join(getNonBlankStrings(procedural.Examples), "; "),
		"procedural_expected_behavior": strings.TrimSpace(procedural.ExpectedBehavior),
	}
	for key, value := range values {
		if value == "" {
			continue
		}
		if metadata == nil {
			metadata = make(map[string]string)
		}
		metadata[key] = value
	}

	return metadata
}

func reflectionMetadataEntriesToMap(entries []reflectionModelMetadataEntry) map[string]string {
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

func getNonBlankStrings(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		if value := strings.TrimSpace(value); value != "" {
			normalized = append(normalized, value)
		}
	}

	return normalized
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(metadata))
	maps.Copy(cloned, metadata)
	return cloned
}
