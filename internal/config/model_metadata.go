package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	modelprovider "github.com/wandxy/hand/internal/model/provider"
)

var (
	httpClient       = &http.Client{Timeout: 5 * time.Second}
	modelDocsBaseURL = "https://developers.openai.com/api/docs/models"
	resolveModelMeta = fetchModelMetadataFromProvider
)

var contextWindowPatternOAI = regexp.MustCompile(`([0-9][0-9,]*)(?:\s|<!--[^>]*-->)+context window`)

// modelVerifySlot pairs a config field label (YAML keys) with the slug sent to resolveModelMeta.
type modelVerifySlot struct {
	field string
	slug  string
}

func isValidModelSlug(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}

	owner, name, ok := strings.Cut(value, "/")
	if !ok {
		return false
	}

	owner = strings.TrimSpace(owner)
	name = strings.TrimSpace(name)
	return owner != "" && name != "" && !strings.Contains(name, "/")
}

func applyProviderModelMetadata(ctx context.Context, cfg *Config, requestedContextLength int) {
	if cfg == nil {
		return
	}
	if !cfg.VerifyEnabled() {
		return
	}

	auth, err := cfg.ResolveModelAuth()
	if err != nil {
		return
	}

	meta, err := resolveModelMeta(ctx, cfg, auth)
	if err != nil || !meta.Exists || meta.ContextLength <= 0 {
		return
	}

	if requestedContextLength <= 0 || requestedContextLength > meta.ContextLength {
		cfg.Models.Main.ContextLength = meta.ContextLength
	}
}

func fetchModelMetadataFromProvider(ctx context.Context, cfg *Config, auth ModelAuth) (ModelMetadata, error) {
	if cfg == nil {
		return ModelMetadata{}, nil
	}

	return fetchModelMetadataForSlug(ctx, auth, cfg.Models.Main.Name)
}

func fetchModelMetadataForSlug(ctx context.Context, auth ModelAuth, slug string) (ModelMetadata, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ModelMetadata{}, nil
	}

	switch strings.TrimSpace(strings.ToLower(auth.Provider)) {
	case "openrouter":
		return fetchOpenRouterModelMetadata(ctx, auth.BaseURL, slug, auth.APIKey)
	case "openai":
		return fetchOpenAIModelMetadata(ctx, slug)
	default:
		return ModelMetadata{}, fmt.Errorf("unsupported model provider %q", auth.Provider)
	}
}

func fetchOpenRouterModelMetadata(ctx context.Context, baseURL, model, apiKey string) (ModelMetadata, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return ModelMetadata{}, nil
	}

	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = getDefaultBaseURLForProvider("openrouter", modelprovider.APIOpenAICompletions)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return ModelMetadata{}, err
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return ModelMetadata{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ModelMetadata{}, fmt.Errorf("failed to verify openrouter model %q: "+
			"openrouter models lookup returned %s", model, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ModelMetadata{}, err
	}

	type openRouterModel struct {
		ID            string `json:"id"`
		ContextLength int    `json:"context_length"`
	}

	var wrapped struct {
		Data []openRouterModel `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return ModelMetadata{}, err
	}

	for _, item := range wrapped.Data {
		if strings.TrimSpace(item.ID) == model {
			return ModelMetadata{
				Exists:        true,
				ContextLength: item.ContextLength,
			}, nil
		}
	}

	return ModelMetadata{}, nil
}

func fetchOpenRouterModelEndpoints(ctx context.Context, baseURL, model, apiKey string) (ModelMetadata, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return ModelMetadata{}, nil
	}

	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = getDefaultBaseURLForProvider("openrouter", modelprovider.APIOpenAICompletions)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		baseURL+"/models/"+getOpenRouterModelPath(model)+"/endpoints",
		nil,
	)
	if err != nil {
		return ModelMetadata{}, err
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return ModelMetadata{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ModelMetadata{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return ModelMetadata{}, fmt.Errorf("failed to verify openrouter model %q: "+
			"openrouter model endpoints lookup returned %s", model, resp.Status)
	}

	return ModelMetadata{Exists: true}, nil
}

func getOpenRouterModelPath(model string) string {
	segments := strings.Split(strings.Trim(strings.TrimSpace(model), "/"), "/")
	for idx, segment := range segments {
		segments[idx] = url.PathEscape(segment)
	}

	return strings.Join(segments, "/")
}

func fetchOpenAIModelMetadata(ctx context.Context, model string) (ModelMetadata, error) {
	for _, candidate := range getOpenAIModelDocSlugs(model) {
		meta, err := fetchOpenAIModelMetadataPage(ctx, candidate, true)
		if err != nil {
			return ModelMetadata{}, err
		}
		if meta.Exists {
			return meta, nil
		}
	}

	return ModelMetadata{}, nil
}

func fetchOpenAIModelExists(ctx context.Context, model string) (ModelMetadata, error) {
	for _, candidate := range getOpenAIModelDocSlugs(model) {
		meta, err := fetchOpenAIModelMetadataPage(ctx, candidate, false)
		if err != nil {
			return ModelMetadata{}, err
		}
		if meta.Exists {
			return meta, nil
		}
	}

	return ModelMetadata{}, nil
}

func fetchOpenAIModelMetadataPage(ctx context.Context, model string, requireContextWindow bool) (ModelMetadata, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return ModelMetadata{}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(modelDocsBaseURL, "/")+"/"+model, nil)
	if err != nil {
		return ModelMetadata{}, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return ModelMetadata{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ModelMetadata{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return ModelMetadata{}, fmt.Errorf("failed to verify openai model %q: openai model docs lookup returned %s", model, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ModelMetadata{}, err
	}
	if isOpenAIModelDocsPageNotFound(body) {
		return ModelMetadata{}, nil
	}

	match := contextWindowPatternOAI.FindStringSubmatch(string(body))
	if len(match) != 2 {
		if !requireContextWindow {
			return ModelMetadata{Exists: true}, nil
		}

		return ModelMetadata{}, nil
	}

	contextLength, err := strconv.Atoi(strings.ReplaceAll(match[1], ",", ""))
	if err != nil {
		return ModelMetadata{}, err
	}

	return ModelMetadata{
		Exists:        true,
		ContextLength: contextLength,
	}, nil
}

func isOpenAIModelDocsPageNotFound(body []byte) bool {
	text := string(body)
	return strings.Contains(text, "<title>Page not found | OpenAI API</title>") ||
		strings.Contains(text, `name="title" content="Page not found | OpenAI API"`)
}

func newUnknownModelError(provider, model string) error {
	switch strings.TrimSpace(strings.ToLower(provider)) {
	case "openrouter":
		return fmt.Errorf("model %q is not available on openrouter", model)
	default:
		return fmt.Errorf("model %q is not available on openai", model)
	}
}

func getOpenAIModelDocSlugs(model string) []string {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}

	if prefix, suffix, ok := strings.Cut(model, "/"); ok && strings.EqualFold(prefix, "openai") {
		model = strings.TrimSpace(suffix)
	}

	candidates := []string{model}
	if base := trimOpenAISnapshotSuffix(model); base != model {
		candidates = append(candidates, base)
	}

	return dedupeAndTrim(candidates)
}

func trimOpenAISnapshotSuffix(model string) string {
	parts := strings.Split(strings.TrimSpace(model), "-")
	if len(parts) < 4 {
		return model
	}

	last := len(parts) - 1
	if len(parts[last-2]) != 4 || len(parts[last-1]) != 2 || len(parts[last]) != 2 {
		return model
	}

	if _, err := strconv.Atoi(parts[last-2]); err != nil {
		return model
	}
	if _, err := strconv.Atoi(parts[last-1]); err != nil {
		return model
	}
	if _, err := strconv.Atoi(parts[last]); err != nil {
		return model
	}

	return strings.Join(parts[:last-2], "-")
}
