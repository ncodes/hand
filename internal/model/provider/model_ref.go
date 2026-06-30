package provider

import (
	"strings"

	"github.com/wandxy/morph/internal/constants"
	"github.com/wandxy/morph/pkg/stringx"
)

// ModelRef identifies a model by provider and provider-local model id.
type ModelRef struct {
	Provider string
	Model    string
}

// String returns the canonical provider/model reference.
func (r ModelRef) String() string {
	provider := normalizeID(r.Provider)
	model := stringx.String(r.Model).Trim()
	if provider == "" || model == "" {
		return ""
	}

	return provider + "/" + model
}

// ParseLocalModelRef parses refs such as ollama/llama3.1:8b.
func ParseLocalModelRef(value string) (ModelRef, bool) {
	provider, model, ok := strings.Cut(stringx.String(value).Trim(), "/")
	if !ok {
		return ModelRef{}, false
	}

	ref := ModelRef{
		Provider: normalizeID(provider),
		Model:    stringx.String(model).Trim(),
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
