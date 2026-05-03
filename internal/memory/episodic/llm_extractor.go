package episodic

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/wandxy/hand/internal/instructions"
	handmsg "github.com/wandxy/hand/internal/messages"
	"github.com/wandxy/hand/internal/models"
)

const defaultLLMExtractorMaxOutputTokens int64 = 1600

func NewLLMExtractor(options LLMExtractorOptions) (*LLMExtractor, error) {
	if options.Client == nil {
		return nil, errors.New("memory episode extractor model client is required")
	}
	if strings.TrimSpace(options.Model) == "" {
		return nil, errors.New("memory episode extractor model is required")
	}
	if options.MaxOutputTokens <= 0 {
		options.MaxOutputTokens = defaultLLMExtractorMaxOutputTokens
	}
	return &LLMExtractor{options: options}, nil
}

func (e *LLMExtractor) ExtractCandidates(ctx context.Context, req CandidateRequest) (CandidateResult, error) {
	if e == nil || e.options.Client == nil {
		return CandidateResult{}, errors.New("memory episode extractor model client is required")
	}
	payload, _ := json.Marshal(req)
	resp, err := e.options.Client.Complete(ctx, models.Request{
		Model:            e.options.Model,
		APIMode:          e.options.APIMode,
		Instructions:     instructions.BuildEpisodicExtractionInstructions(),
		Messages:         []handmsg.Message{{Role: handmsg.RoleUser, Content: string(payload)}},
		StructuredOutput: llmExtractorStructuredOutput(),
		MaxOutputTokens:  e.options.MaxOutputTokens,
		DebugRequests:    e.options.DebugRequests,
	})
	if err != nil {
		return CandidateResult{}, err
	}
	return parseLLMExtractorResponse(resp)
}

type llmExtractorResponse struct {
	Candidates []llmExtractorCandidate `json:"candidates"`
	Rejections []candidateRejection    `json:"rejections"`
}
type llmExtractorCandidate struct {
	Kind       string            `json:"kind"`
	Title      string            `json:"title"`
	Text       string            `json:"text"`
	Confidence float64           `json:"confidence"`
	Metadata   map[string]string `json:"metadata"`
}

func parseLLMExtractorResponse(resp *models.Response) (CandidateResult, error) {
	if resp == nil {
		return CandidateResult{}, errors.New("memory episode extractor response is required")
	}

	var parsed llmExtractorResponse
	if err := json.Unmarshal([]byte(normalizedLLMExtractorJSON(resp.OutputText)), &parsed); err != nil {
		return CandidateResult{}, err
	}

	result := CandidateResult{Rejections: parsed.Rejections}
	for _, candidate := range parsed.Candidates {
		result.Candidates = append(result.Candidates, episodeCandidate{
			Kind:       strings.TrimSpace(candidate.Kind),
			Title:      strings.TrimSpace(candidate.Title),
			Text:       strings.TrimSpace(candidate.Text),
			Confidence: candidate.Confidence,
			Metadata:   candidate.Metadata,
		})
	}

	return result, nil
}

func normalizedLLMExtractorJSON(raw string) string {
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

func llmExtractorStructuredOutput() *models.StructuredOutput {
	return &models.StructuredOutput{
		Name:        "episodic_memory_candidates",
		Description: "Curated episodic memory candidates",
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
							"kind": map[string]any{
								"type": "string",
								"enum": []string{
									episodeKindDecision,
									episodeKindOutcome,
									episodeKindToolEvent,
									episodeKindBlocker,
									episodeKindUserCorrection,
								},
							},
							"title":      map[string]any{"type": "string"},
							"text":       map[string]any{"type": "string"},
							"confidence": map[string]any{"type": "number"},
							"metadata": map[string]any{
								"type":                 "object",
								"additionalProperties": map[string]any{"type": "string"},
							},
						},
						"required": []string{"kind", "title", "text", "confidence", "metadata"},
					},
				},
				"rejections": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]any{
							"kind":   map[string]any{"type": "string"},
							"reason": map[string]any{"type": "string"},
						},
						"required": []string{"kind", "reason"},
					},
				},
			},
			"required": []string{"candidates", "rejections"},
		},
	}
}
