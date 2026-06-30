package provider_ollama

import (
	"strings"

	"github.com/wandxy/morph/pkg/stringx"
)

func NormalizeModelID(value string) string {
	value = stringx.String(value).Trim()
	if provider, model, ok := strings.Cut(value, "/"); ok &&
		strings.EqualFold(stringx.String(provider).Trim(), "ollama") {
		return stringx.String(model).Trim()
	}

	return value
}

func NormalizeModelIDForComparison(value string) string {
	value = NormalizeModelID(value)
	if value == "" || strings.Contains(value, ":") {
		return value
	}

	return value + ":latest"
}

func ModelIDMatches(installed string, requested string) bool {
	return strings.EqualFold(
		NormalizeModelIDForComparison(installed),
		NormalizeModelIDForComparison(requested),
	)
}
