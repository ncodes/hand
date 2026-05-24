package host

import (
	"github.com/wandxy/hand/internal/environment"
	"github.com/wandxy/hand/internal/memory"
)

type MemorySource struct {
	env environment.Environment
}

func NewMemorySource(env environment.Environment) MemorySource {
	return MemorySource{env: env}
}

func (s MemorySource) MemoryProvider() memory.Provider {
	if s.env == nil {
		return nil
	}

	return s.env.MemoryProvider()
}
