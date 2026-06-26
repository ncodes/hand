package provider

import (
	"strings"

	"github.com/wandxy/morph/internal/constants"
)

// ModelRef identifies a model by provider and provider-local model id.
type ModelRef struct {
	Provider string
	Model    string
}

// String returns the canonical provider/model reference.
func (r ModelRef) String() string {
	provider := normalizeID(r.Provider)
	model := strings.TrimSpace(r.Model)
	if provider == "" || model == "" {
		return ""
	}

	return provider + "/" + model
}

// ParseLocalModelRef parses refs such as ollama/llama3.1:8b.
func ParseLocalModelRef(value string) (ModelRef, bool) {
	provider, model, ok := strings.Cut(strings.TrimSpace(value), "/")
	if !ok {
		return ModelRef{}, false
	}

	ref := ModelRef{
		Provider: normalizeID(provider),
		Model:    strings.TrimSpace(model),
	}
	if ref.Provider == "" || ref.Model == "" || !IsLocalProviderID(ref.Provider) {
		return ModelRef{}, false
	}

	return ref, true
}

// IsLocalProviderID reports whether provider is reserved for local model runtimes.
func IsLocalProviderID(provider string) bool {
	switch normalizeID(provider) {
	case constants.ModelProviderOllama,
		constants.ModelProviderVLLM,
		constants.ModelProviderSGLang,
		constants.ModelProviderCustomLocal:
		return true
	default:
		return false
	}
}
