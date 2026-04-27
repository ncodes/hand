package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultEmbeddingMaxInputsPerBatch = 96
	defaultEmbeddingMaxInputTextBytes = 32 * 1024
	defaultEmbeddingTimeout           = 30 * time.Second
	defaultEmbeddingMaxRetries        = 2
)

type EmbeddingProviderOptions struct {
	HTTPClient        *http.Client
	Provider          string
	APIKey            string
	EndpointURL       string
	MaxInputsPerBatch int
	MaxInputTextBytes int
	Timeout           time.Duration
	MaxRetries        int
}

type EmbeddingProvider struct {
	client            *http.Client
	provider          string
	apiKey            string
	endpointURL       string
	maxInputsPerBatch int
	maxInputTextBytes int
	timeout           time.Duration
	maxRetries        int
}

func NewEmbeddingProvider(opts EmbeddingProviderOptions) (*EmbeddingProvider, error) {
	provider := strings.TrimSpace(strings.ToLower(opts.Provider))
	if provider == "" {
		return nil, errors.New("embedding provider is required")
	}

	endpointURL := strings.TrimRight(strings.TrimSpace(opts.EndpointURL), "/")
	if endpointURL == "" {
		return nil, errors.New("embedding endpoint URL is required")
	}
	if strings.TrimSpace(opts.APIKey) == "" {
		return nil, errors.New("embedding API key is required")
	}

	maxInputs := opts.MaxInputsPerBatch
	if maxInputs <= 0 {
		maxInputs = defaultEmbeddingMaxInputsPerBatch
	}

	maxTextBytes := opts.MaxInputTextBytes
	if maxTextBytes <= 0 {
		maxTextBytes = defaultEmbeddingMaxInputTextBytes
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultEmbeddingTimeout
	}

	maxRetries := opts.MaxRetries
	if maxRetries < 0 {
		return nil, errors.New("embedding max retries must be non-negative")
	}
	if maxRetries == 0 {
		maxRetries = defaultEmbeddingMaxRetries
	}

	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	return &EmbeddingProvider{
		client:            client,
		provider:          provider,
		apiKey:            strings.TrimSpace(opts.APIKey),
		endpointURL:       endpointURL,
		maxInputsPerBatch: maxInputs,
		maxInputTextBytes: maxTextBytes,
		timeout:           timeout,
		maxRetries:        maxRetries,
	}, nil
}

func (p *EmbeddingProvider) Embed(ctx context.Context, req EmbeddingRequest) (EmbeddingResult, error) {
	if p == nil {
		return EmbeddingResult{}, errors.New("embedding provider is required")
	}
	if err := ValidateEmbeddingRequest(req); err != nil {
		return EmbeddingResult{}, err
	}
	if err := p.validateRequestLimits(req); err != nil {
		return EmbeddingResult{}, err
	}

	retrievalLog.Debug().
		Str("event", "embedding request started").
		Str("provider", p.provider).
		Str("embedding_model", strings.TrimSpace(req.Model)).
		Int("input_count", len(req.Inputs)).
		Int("max_inputs_per_batch", p.maxInputsPerBatch).
		Msg("embedding provider request started")

	result := EmbeddingResult{
		Model: strings.TrimSpace(req.Model),
		Items: make([]Embedding, 0, len(req.Inputs)),
	}
	for start := 0; start < len(req.Inputs); start += p.maxInputsPerBatch {
		end := min(start+p.maxInputsPerBatch, len(req.Inputs))
		batchResult, err := p.embedBatch(ctx, req.Model, req.Inputs[start:end])
		if err != nil {
			retrievalLog.Debug().
				Str("event", "embedding request failed").
				Str("error_kind", embeddingProviderErrorKind(err)).
				Str("provider", p.provider).
				Str("embedding_model", strings.TrimSpace(req.Model)).
				Int("input_count", len(req.Inputs)).
				Msg("embedding provider request failed")
			return EmbeddingResult{}, err
		}
		if result.Dimensions == 0 {
			result.Dimensions = batchResult.Dimensions
		} else if result.Dimensions != batchResult.Dimensions {
			err := errors.New("embedding dimensions changed between batches")
			retrievalLog.Debug().
				Str("event", "embedding request failed").
				Str("error_kind", err.Error()).
				Str("provider", p.provider).
				Str("embedding_model", strings.TrimSpace(req.Model)).
				Int("input_count", len(req.Inputs)).
				Msg("embedding provider request failed")
			return EmbeddingResult{}, err
		}
		result.Items = append(result.Items, batchResult.Items...)
	}

	retrievalLog.Debug().
		Str("event", "embedding request completed").
		Str("provider", p.provider).
		Str("embedding_model", result.Model).
		Int("input_count", len(req.Inputs)).
		Int("embedding_count", len(result.Items)).
		Int("dimensions", result.Dimensions).
		Msg("embedding provider request completed")

	return result, nil
}

func (p *EmbeddingProvider) validateRequestLimits(req EmbeddingRequest) error {
	for _, input := range req.Inputs {
		if len([]byte(input.Text)) > p.maxInputTextBytes {
			return fmt.Errorf("embedding input %q exceeds %d bytes", input.ID, p.maxInputTextBytes)
		}
	}

	return nil
}

func (p *EmbeddingProvider) embedBatch(
	ctx context.Context,
	model string,
	inputs []EmbeddingInput,
) (EmbeddingResult, error) {
	var lastErr error
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		retrievalLog.Debug().
			Str("event", "embedding batch started").
			Str("provider", p.provider).
			Str("embedding_model", strings.TrimSpace(model)).
			Int("input_count", len(inputs)).
			Int("attempt", attempt+1).
			Msg("embedding provider batch started")

		result, retry, err := p.embedBatchAttempt(ctx, model, inputs)
		if err == nil {
			retrievalLog.Debug().
				Str("event", "embedding batch completed").
				Str("provider", p.provider).
				Str("embedding_model", strings.TrimSpace(result.Model)).
				Int("input_count", len(inputs)).
				Int("embedding_count", len(result.Items)).
				Int("dimensions", result.Dimensions).
				Int("attempt", attempt+1).
				Msg("embedding provider batch completed")
			return result, nil
		}
		lastErr = err
		retrievalLog.Debug().
			Bool("retry", retry && attempt < p.maxRetries).
			Str("event", "embedding batch failed").
			Str("error_kind", embeddingProviderErrorKind(err)).
			Str("provider", p.provider).
			Str("embedding_model", strings.TrimSpace(model)).
			Int("input_count", len(inputs)).
			Int("attempt", attempt+1).
			Msg("embedding provider batch failed")
		if !retry || attempt == p.maxRetries {
			break
		}
	}

	return EmbeddingResult{}, lastErr
}

func (p *EmbeddingProvider) embedBatchAttempt(
	ctx context.Context,
	model string,
	inputs []EmbeddingInput,
) (EmbeddingResult, bool, error) {
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	payload := embeddingProviderRequest{
		Model:          strings.TrimSpace(model),
		Input:          embeddingTexts(inputs),
		EncodingFormat: "float",
	}
	body, _ := json.Marshal(payload)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpointURL, bytes.NewReader(body))
	if err != nil {
		return EmbeddingResult{}, false, err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return EmbeddingResult{}, true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := providerErrorMessage(resp)
		return EmbeddingResult{}, retryableEmbeddingStatus(resp.StatusCode),
			fmt.Errorf("embedding request failed: %s", message)
	}

	var decoded embeddingProviderResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return EmbeddingResult{}, false, err
	}

	result, err := embeddingResultFromOpenAIResponse(strings.TrimSpace(model), inputs, decoded)
	if err != nil {
		return EmbeddingResult{}, false, err
	}

	return result, false, nil
}

func embeddingResultFromOpenAIResponse(
	model string,
	inputs []EmbeddingInput,
	response embeddingProviderResponse,
) (EmbeddingResult, error) {
	if strings.TrimSpace(response.Model) == "" {
		return EmbeddingResult{}, errors.New("embedding result model is required")
	}
	if strings.TrimSpace(response.Model) != strings.TrimSpace(model) {
		return EmbeddingResult{}, errors.New("embedding result model must match request model")
	}
	if len(response.Data) != len(inputs) {
		return EmbeddingResult{}, errors.New("embedding result count must match input count")
	}

	seen := make(map[int]struct{}, len(response.Data))
	items := make([]Embedding, len(inputs))
	dimensions := 0
	for _, item := range response.Data {
		if item.Index < 0 || item.Index >= len(inputs) {
			return EmbeddingResult{}, fmt.Errorf("embedding response index %d is out of range", item.Index)
		}
		if _, ok := seen[item.Index]; ok {
			return EmbeddingResult{}, fmt.Errorf("embedding response index %d is duplicated", item.Index)
		}
		seen[item.Index] = struct{}{}
		if len(item.Embedding) == 0 {
			return EmbeddingResult{}, errors.New("embedding vector is required")
		}
		if dimensions == 0 {
			dimensions = len(item.Embedding)
		} else if dimensions != len(item.Embedding) {
			return EmbeddingResult{}, errors.New("embedding vector dimensions do not match result dimensions")
		}

		input := inputs[item.Index]
		items[item.Index] = Embedding{
			ID:          input.ID,
			ContentHash: VectorContentHash(input.Text),
			Vector:      append([]float64(nil), item.Embedding...),
		}
	}

	return EmbeddingResult{
		Model:      strings.TrimSpace(response.Model),
		Items:      items,
		Dimensions: dimensions,
	}, nil
}

func embeddingTexts(inputs []EmbeddingInput) []string {
	values := make([]string, 0, len(inputs))
	for _, input := range inputs {
		values = append(values, input.Text)
	}

	return values
}

func providerErrorMessage(resp *http.Response) string {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
	message := strings.TrimSpace(string(data))
	if message == "" {
		message = resp.Status
	}

	return message
}

func retryableEmbeddingStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

func embeddingProviderErrorKind(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}

	value := strings.ToLower(err.Error())
	switch {
	case strings.Contains(value, "embedding request failed"):
		return "provider_request_failed"
	case strings.Contains(value, "json"):
		return "decode_failed"
	case strings.Contains(value, "model"):
		return "model_mismatch"
	case strings.Contains(value, "dimensions"):
		return "dimension_mismatch"
	case strings.Contains(value, "timeout"):
		return "timeout"
	default:
		return "operation_failed"
	}
}

type embeddingProviderRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	EncodingFormat string   `json:"encoding_format,omitempty"`
}

type embeddingProviderResponse struct {
	Model string                          `json:"model"`
	Data  []embeddingProviderResponseData `json:"data"`
}

type embeddingProviderResponseData struct {
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

var _ Embedder = (*EmbeddingProvider)(nil)
