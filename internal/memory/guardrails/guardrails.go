package guardrails

import (
	"context"
	"errors"
	"strings"

	coreguardrails "github.com/wandxy/hand/internal/guardrails"
	"github.com/wandxy/hand/internal/memory"
)

type Guardrails struct {
	redactor coreguardrails.Redactor
}

func New(redactor coreguardrails.Redactor) memory.Guardrails {
	if redactor == nil {
		redactor = coreguardrails.NewRedactor()
	}
	return Guardrails{redactor: redactor}
}

func (g Guardrails) ValidateSearch(context.Context, memory.SearchQuery) error {
	return nil
}

func (g Guardrails) ValidateWrite(context.Context, memory.MemoryItem) error {
	return nil
}

func (g Guardrails) ValidateDelete(context.Context, memory.DeleteRequest) error {
	return nil
}

func (g Guardrails) SafetyScan(_ context.Context, item memory.MemoryItem) error {
	scanned := coreguardrails.SafetyScan(
		strings.Join([]string{item.Title, item.Text}, "\n"),
		safetyScanSource(item),
	)
	if scanned.Blocked {
		return errors.New("memory item failed safety scan")
	}
	return nil
}

func (g Guardrails) Redact(_ context.Context, item memory.MemoryItem) (memory.MemoryItem, error) {
	item.Title = sanitizedString(g.redactor, item.Title)
	item.Text = sanitizedString(g.redactor, item.Text)
	item.Tags = sanitizedStrings(g.redactor, item.Tags)
	if len(item.Metadata) > 0 {
		metadata := make(map[string]string, len(item.Metadata))
		for key, value := range item.Metadata {
			metadata[key] = sanitizedString(g.redactor, value)
		}
		item.Metadata = metadata
	}
	return item, nil
}

func sanitizedString(redactor coreguardrails.Redactor, value string) string {
	if redactor == nil {
		redactor = coreguardrails.NewRedactor()
	}
	sanitized, ok := redactor.Sanitize(value).(string)
	if !ok {
		return value
	}
	return sanitized
}

func safetyScanSource(item memory.MemoryItem) string {
	if id := strings.TrimSpace(item.ID); id != "" {
		return "memory:" + id
	}
	return "memory"
}

func sanitizedStrings(redactor coreguardrails.Redactor, values []string) []string {
	if len(values) == 0 {
		return nil
	}
	sanitized := make([]string, 0, len(values))
	for _, value := range values {
		sanitized = append(sanitized, sanitizedString(redactor, value))
	}
	return sanitized
}
