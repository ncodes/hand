package memory

import (
	"errors"
	"strings"
)

var ErrUnknownProvider = errors.New("unknown memory provider")

func NewProvider(name string, opts Options) (Provider, error) {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "", ProviderNoop:
		return NewNoopProvider(opts), nil
	case ProviderInMemory:
		return NewInMemoryProvider(opts), nil
	default:
		return nil, ErrUnknownProvider
	}
}
